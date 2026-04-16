package manager

import (
	pbknowledge "api/api/ai/knowledge/v1"
	filepb "api/api/file/common/v1"
	pbfile "api/api/file/files/v1"
	userpb "api/api/user/common/v1"
	"api/external/trans"
	"common/constants"
	"common/hashid"
	"common/util"
	"context"
	"encoding/json"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/queue"
	"file/internal/data/rpc"
	"file/internal/data/types"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
)

type (
	FullTextIndexTask struct {
		*queue.DBTask
		client *rpc.KnowledgeClient
	}

	FullTextIndexTaskState struct {
		Uri      *fs.URI `json:"uri"`
		EntityID int     `json:"entity_id"`
		FileID   int     `json:"file_id"`
		OwnerID  int     `json:"owner_id"`
	}

	ftsFileInfo struct {
		FileID   int
		OwnerID  int
		EntityID int
		FileName string
	}
)

func (m *manager) SearchFullText(ctx context.Context, query string, offset int) (*FullTextSearchResults, error) {
	results, total, err := m.aikc.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search full text: %w", err)
	}

	if len(results) == 0 {
		return &FullTextSearchResults{}, nil
	}

	// Traverse each file in result
	files := lo.FilterMap(results, func(result *pbknowledge.SearchResult, index int) (FullTextSearchResult, bool) {
		uri, err := fs.NewUriFromString(result.DocUri)
		if err != nil {
			return FullTextSearchResult{}, false
		}
		entityID, err := m.hasher.Decode(result.DocVersion, hashid.EntityID)
		if err != nil {
			return FullTextSearchResult{}, false
		}
		file, err := m.Get(ctx, uri)
		if err != nil || file.PrimaryEntityID() != entityID {
			m.l.Debugf("Failed to get file %s for full text search: %s, skipping.", result.DocUri, err)
			return FullTextSearchResult{}, false
		}
		return FullTextSearchResult{
			File:    file,
			Content: result.Content,
		}, true
	})

	if len(files) == 0 {
		// No valid files, run next offset
		return m.SearchFullText(ctx, query, offset+len(results))
	}

	return &FullTextSearchResults{
		Hits:  files,
		Total: total,
	}, nil
}

func init() {
	queue.RegisterResumableTaskFactory(queue.FullTextIndexTaskType, NewFullTextIndexTaskFromModel)
	queue.RegisterResumableTaskFactory(queue.FullTextCopyTaskType, NewFullTextCopyTaskFromModel)
	queue.RegisterResumableTaskFactory(queue.FullTextChangeOwnerTaskType, NewFullTextChangeOwnerTaskFromModel)
	queue.RegisterResumableTaskFactory(queue.FullTextDeleteTaskType, NewFullTextDeleteTaskFromModel)
}

func NewFullTextIndexTask(ctx context.Context, uri *fs.URI, entityID, fileID int, ownerID int, creator *userpb.User) (*FullTextIndexTask, error) {
	state := &FullTextIndexTaskState{
		Uri:      uri,
		FileID:   fileID,
		OwnerID:  ownerID,
		EntityID: entityID,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return &FullTextIndexTask{
		DBTask: &queue.DBTask{
			DirectOwner: creator,
			Task: &ent.Task{
				Type:         queue.FullTextIndexTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &filepb.TaskPublicState{},
			},
		},
	}, nil
}

func NewFullTextIndexTaskFromModel(t *ent.Task) queue.Task {
	return &FullTextIndexTask{
		DBTask: &queue.DBTask{
			Task: t,
		},
	}
}

func (t *FullTextIndexTask) Do(ctx context.Context) (task.Status, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	l := dep.Logger()
	fm := NewFileManager(dep, dbfsDep, trans.FromContext(ctx)).(*manager)

	if fm.settings.FTSEnabled(ctx) {
		l.Debug("FTS disabled, skipping full text index task.")
		return task.StatusCompleted, nil
	}

	// Unmarshal state
	var state FullTextIndexTaskState
	if err := json.Unmarshal([]byte(t.State()), &state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %s (%w)", err, queue.CriticalErr)
	}

	// Get fresh file to make sure task is not stale
	file, err := fm.Get(ctx, state.Uri, dbfs.WithFilePublicMetadata())
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get latest file: %w", err)
	}

	if file.PrimaryEntityID() != state.EntityID {
		l.Debug("File %d is not the latest version, skipping indexing.", state.FileID)
		return task.StatusCompleted, nil
	}

	docID, _ := file.Metadata()[dbfs.FullTextIndexKey]

	return performIndexing(ctx, fm, state.Uri, state.EntityID, state.FileID, state.OwnerID, state.Uri.Name(), docID, t.client, l)
}

func performIndexing(ctx context.Context, fm *manager, uri *fs.URI, entityID, fileID, ownerID int, fileName string,
	docID string, client *rpc.KnowledgeClient, l *log.Helper) (task.Status, error) {
	// Delete old chunks
	if docID != "" {
		err := client.DeleteDocuments(ctx, docID)
		if err != nil {
			l.Warnf("Failed to delete old index chunks for file %d: %s", fileID, err)
		}
	}

	kid := hashid.EncodeID(fm.hasher, constants.SystemKnowledgeID, hashid.KnowledgeID)
	// Index
	document, err := client.CreateDocument(ctx, &rpc.CreateDocumentRequest{
		KnowledgeId:  kid,
		DocumentName: fileName,
		Uri:          uri.String(),
		Version:      hashid.EncodeID(fm.hasher, entityID, hashid.EntityID),
	})
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to create document to index: %w", err)
	}

	// Upsert metadata
	if err := fm.fs.PatchMetadata(ctx, []*fs.URI{uri}, &pbfile.MetadataPatch{
		Key:   dbfs.FullTextIndexKey,
		Value: document.Id,
	}); err != nil {
		return task.StatusError, fmt.Errorf("failed to patch metadata: %w", err)
	}

	l.Debugf("Successfully indexed file %d for owner %d.", fileID, ownerID)
	return task.StatusCompleted, nil
}

type (
	FullTextCopyTask struct {
		*queue.DBTask
		client *rpc.KnowledgeClient
	}

	FullTextCopyTaskState struct {
		Uri            *fs.URI `json:"uri"`
		OriginalFileID int     `json:"original_file_id"`
		FileID         int     `json:"file_id"`
		OwnerID        int     `json:"owner_id"`
		EntityID       int     `json:"entity_id"`
	}
)

func NewFullTextCopyTask(ctx context.Context, uri *fs.URI, originalFileID, fileID, ownerID, entityID int, creator *userpb.User,
	client *rpc.KnowledgeClient) (*FullTextCopyTask, error) {
	state := &FullTextCopyTaskState{
		Uri:            uri,
		OriginalFileID: originalFileID,
		FileID:         fileID,
		OwnerID:        ownerID,
		EntityID:       entityID,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return &FullTextCopyTask{
		DBTask: &queue.DBTask{
			DirectOwner: creator,
			Task: &ent.Task{
				Type:         queue.FullTextCopyTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &filepb.TaskPublicState{},
			},
		},
		client: client,
	}, nil
}

func NewFullTextCopyTaskFromModel(t *ent.Task) queue.Task {
	return &FullTextCopyTask{
		DBTask: &queue.DBTask{
			Task: t,
		},
	}
}

func (t *FullTextCopyTask) Do(ctx context.Context) (task.Status, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	fm := NewFileManager(dep, dbfsDep, trans.FromContext(ctx)).(*manager)
	l := dep.Logger()

	if !fm.settings.FTSEnabled(ctx) {
		l.Debug("FTS disabled, skipping full text copy task.")
		return task.StatusCompleted, nil
	}

	var state FullTextCopyTaskState
	if err := json.Unmarshal([]byte(t.State()), &state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %s (%w)", err, queue.CriticalErr)
	}

	// Get fresh file to make sure task is not stale
	file, err := fm.Get(ctx, state.Uri, dbfs.WithFilePublicMetadata())
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get latest file: %w", err)
	}

	if file.PrimaryEntityID() != state.EntityID {
		l.Debugf("File %d entity changed, skipping copy index.", state.FileID)
		return task.StatusCompleted, nil
	}

	entityID := hashid.EncodeID(fm.hasher, state.EntityID, hashid.EntityID)
	docID, _ := file.Metadata()[dbfs.FullTextIndexKey]
	if docID == "" {
		return task.StatusError, fmt.Errorf("failed to find index for file %d", state.FileID)
	}
	doc, err := t.client.CopyDocument(ctx, docID, entityID)
	if err != nil {
		l.Warnf("Failed to copy index from file %d to %d, falling back to full indexing: %s", state.OriginalFileID, state.FileID, err)
		return task.StatusError, fmt.Errorf("failed to copy document: %w", err)
	}

	// Patch metadata to mark file as indexed
	if err := fm.fs.PatchMetadata(ctx, []*fs.URI{state.Uri}, &pbfile.MetadataPatch{
		Key:   dbfs.FullTextIndexKey,
		Value: doc.Id,
	}); err != nil {
		return task.StatusError, fmt.Errorf("failed to patch metadata: %w", err)
	}

	l.Debugf("Successfully copied index from file %d to %d.", state.OriginalFileID, state.FileID)
	return task.StatusCompleted, nil
}

type (
	FullTextChangeOwnerTask struct {
		*queue.DBTask
		client *rpc.KnowledgeClient
	}

	FullTextChangeOwnerTaskState struct {
		Uri             *fs.URI `json:"uri"`
		EntityID        int     `json:"entity_id"`
		FileID          int     `json:"file_id"`
		OriginalOwnerID int     `json:"original_owner_id"`
		NewOwnerID      int     `json:"new_owner_id"`
	}
)

func NewFullTextChangeOwnerTask(ctx context.Context, uri *fs.URI, entityID, fileID, originalOwnerID, newOwnerID int, creator *userpb.User,
	client *rpc.KnowledgeClient) (*FullTextChangeOwnerTask, error) {
	state := &FullTextChangeOwnerTaskState{
		Uri:             uri,
		EntityID:        entityID,
		FileID:          fileID,
		OriginalOwnerID: originalOwnerID,
		NewOwnerID:      newOwnerID,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return &FullTextChangeOwnerTask{
		DBTask: &queue.DBTask{
			DirectOwner: creator,
			Task: &ent.Task{
				Type:         queue.FullTextChangeOwnerTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &filepb.TaskPublicState{},
			},
		},
		client: client,
	}, nil
}

func NewFullTextChangeOwnerTaskFromModel(t *ent.Task) queue.Task {
	return &FullTextChangeOwnerTask{
		DBTask: &queue.DBTask{
			Task: t,
		},
	}
}

func (t *FullTextChangeOwnerTask) Do(ctx context.Context) (task.Status, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	fm := NewFileManager(dep, dbfsDep, trans.FromContext(ctx)).(*manager)
	l := dep.Logger()

	if !fm.settings.FTSEnabled(ctx) {
		l.Debug("FTS disabled, skipping full text change owner task.")
		return task.StatusCompleted, nil
	}

	var state FullTextChangeOwnerTaskState
	if err := json.Unmarshal([]byte(t.State()), &state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %s (%w)", err, queue.CriticalErr)
	}

	// Get fresh file to make sure task is not stale
	file, err := fm.Get(ctx, state.Uri, dbfs.WithFilePublicMetadata())
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get latest file: %w", err)
	}

	if file.PrimaryEntityID() != state.EntityID {
		l.Debugf("File %d entity changed, skipping copy index.", state.FileID)
		return task.StatusCompleted, nil
	}

	docID, _ := file.Metadata()[dbfs.FullTextIndexKey]
	if docID == "" {
		return task.StatusError, fmt.Errorf("failed to find index for file %d", state.FileID)
	}
	if _, err := t.client.ChangeDocumentOwner(ctx, docID, state.OriginalOwnerID, state.NewOwnerID); err != nil {
		return task.StatusError, fmt.Errorf("failed to change document owner for file %d: %w", state.FileID, err)
	}

	l.Debugf("Successfully changed index owner for file %d from %d to %d.", state.FileID, state.OriginalOwnerID, state.NewOwnerID)
	return task.StatusCompleted, nil
}

type (
	FullTextDeleteTask struct {
		*queue.DBTask
		client *rpc.KnowledgeClient
	}

	FullTextDeleteTaskState struct {
		DocIDs []int `json:"doc_ids"`
	}
)

func NewFullTextDeleteTask(ctx context.Context, docIDs []int, creator *userpb.User, client *rpc.KnowledgeClient) (*FullTextDeleteTask, error) {
	state := &FullTextDeleteTaskState{
		DocIDs: docIDs,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return &FullTextDeleteTask{
		DBTask: &queue.DBTask{
			DirectOwner: creator,
			Task: &ent.Task{
				Type:         queue.FullTextDeleteTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &filepb.TaskPublicState{},
			},
		},
		client: client,
	}, nil
}

func NewFullTextDeleteTaskFromModel(t *ent.Task) queue.Task {
	return &FullTextDeleteTask{
		DBTask: &queue.DBTask{
			Task: t,
		},
	}
}

func (t *FullTextDeleteTask) Do(ctx context.Context) (task.Status, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	l := dep.Logger()

	// Unmarshal state
	var state FullTextDeleteTaskState
	if err := json.Unmarshal([]byte(t.State()), &state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %s (%w)", err, queue.CriticalErr)
	}

	docIDs := lo.Map(state.DocIDs, func(id int, _ int) string {
		return hashid.EncodeID(dep.Encoder(), id, hashid.KnowledgeDocumentID)
	})
	if err := t.client.DeleteDocuments(ctx, docIDs...); err != nil {
		return task.StatusError, fmt.Errorf("failed to delete index for %d file(s): %w", len(state.DocIDs), err)
	}

	l.Debugf("Successfully deleted index for %d file(s).", len(state.DocIDs))
	return task.StatusCompleted, nil
}

// ShouldExtractText checks if a file is eligible for text extraction based on
// the extractor's supported extensions and max file size. This is exported for
// use by the rebuild index workflow.
func ShouldExtractText(client *rpc.KnowledgeClient, fileName string, size int64) bool {
	ts, maxFileSize, err := client.GetSupportTextParseTypes(context.Background())
	if err != nil {
		return false
	}
	return util.IsInExtensionList(ts, fileName) && maxFileSize > size
}

// shouldIndexFullText checks if a file should be indexed for full-text search.
func (m *manager) shouldIndexFullText(ctx context.Context, fileName string, size int64) bool {
	if !m.settings.FTSEnabled(ctx) {
		return false
	}

	return ShouldExtractText(m.aikc, fileName, size)
}

// fullTextIndexForNewEntity creates and queues a full text index task for a newly uploaded entity.
func (m *manager) fullTextIndexForNewEntity(ctx context.Context, session *fs.UploadSession, owner int) {
	if session.Props.EntityType != nil && *session.Props.EntityType != types.EntityTypeVersion {
		return
	}

	if !m.shouldIndexFullText(ctx, session.Props.Uri.Name(), session.Props.Size) {
		return
	}

	t, err := NewFullTextIndexTask(ctx, session.Props.Uri, session.EntityID, session.FileID, owner, m.user)
	if err != nil {
		m.l.Warnf("Failed to create full text index task: %s", err)
		return
	}
	if err := m.qm.GetMediaMetaQueue().QueueTask(ctx, t); err != nil {
		m.l.Warnf("Failed to queue full text index task: %s", err)
	}
}

func (m *manager) processIndexDiff(ctx context.Context, diff *fs.IndexDiff) {
	if diff == nil {
		return
	}

	for _, update := range diff.IndexToUpdate {
		t, err := NewFullTextIndexTask(ctx, &update.Uri, update.EntityID, update.FileID, update.OwnerID, m.user)
		if err != nil {
			m.l.Warnf("Failed to create full text update task: %s", err)
			continue
		}
		if err := m.qm.GetMediaMetaQueue().QueueTask(ctx, t); err != nil {
			m.l.Warnf("Failed to queue full text update task: %s", err)
		}
	}

	for _, cp := range diff.IndexToCopy {
		t, err := NewFullTextCopyTask(ctx, &cp.Uri, cp.OriginalFileID, cp.FileID, cp.OwnerID, cp.EntityID, m.user, m.aikc)
		if err != nil {
			m.l.Warnf("Failed to create full text copy task: %s", err)
			continue
		}
		if err := m.qm.GetMediaMetaQueue().QueueTask(ctx, t); err != nil {
			m.l.Warnf("Failed to queue full text copy task: %s", err)
		}
	}

	for _, change := range diff.IndexToChangeOwner {
		t, err := NewFullTextChangeOwnerTask(ctx, &change.Uri, change.EntityID, change.FileID, change.OriginalOwnerID, change.NewOwnerID, m.user, m.aikc)
		if err != nil {
			m.l.Warnf("Failed to create full text change owner task: %s", err)
			continue
		}
		if err := m.qm.GetMediaMetaQueue().QueueTask(ctx, t); err != nil {
			m.l.Warnf("Failed to queue full text change owner task: %s", err)
		}
	}

	if len(diff.IndexToDelete) > 0 && m.settings.FTSEnabled(ctx) {
		t, err := NewFullTextDeleteTask(ctx, diff.IndexToDelete, m.user, m.aikc)
		if err != nil {
			m.l.Warnf("Failed to create full text delete task: %s", err)
			return
		}
		if err := m.qm.GetMediaMetaQueue().QueueTask(ctx, t); err != nil {
			m.l.Warnf("Failed to queue full text delete task: %s", err)
		}
	}

	// 由于ai模块的知识库里的文档和文件名不强制一致，所以这里不处理重命名
	//ctx = context.WithoutCancel(ctx)
	//go func() {
	//	for _, rename := range diff.IndexToRename {
	//		if err := m.aikc.Rename(ctx, rename.FileID, rename.EntityID, rename.Uri.Name()); err != nil {
	//			m.l.Warnf("Failed to rename index for file %d: %s", rename.FileID, err)
	//		}
	//	}
	//}()
}
