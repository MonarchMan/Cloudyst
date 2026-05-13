package workflows

import (
	"api/external/data/userdata"
	"common/hashid"
	"common/util"
	"context"
	"encoding/json"
	"file/ent"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/queue"
	"file/internal/data"
	"file/internal/data/rpc"
	"fmt"
	mqueue "queue"
	"strconv"
)

type (
	RebuildIndexTask struct {
		*queue.DBTask

		state  *RebuildIndexTaskState
		client *rpc.KnowledgeClient
	}
	RebuildIndexTaskPhase string
	RebuildIndexTaskState struct {
		Phase                 RebuildIndexTaskPhase `json:"phase"`
		Total                 int                   `json:"total"`
		Indexed               int                   `json:"indexed"`
		LastFileID            int                   `json:"last_file_id"`
		Failed                int                   `json:"failed"`
		FilteredStoragePolicy []int                 `json:"filtered_storage_policy"`
	}
)

const (
	RebuildIndexBatchSize = 1000

	ProgressTypeRebuildIndex = "rebuild_index"
)

func init() {
	mqueue.RegisterResumableTaskFactory(queue.FullTextRebuildTaskType, NewRebuildIndexTaskFromModel)
}

func NewRebuildIndexTask(ctx context.Context, u *userdata.User, filteredStoragePolicy []int) (mqueue.Task, error) {
	state := &RebuildIndexTaskState{
		FilteredStoragePolicy: filteredStoragePolicy,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return &RebuildIndexTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:         queue.FullTextRebuildTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &mqueue.TaskPublicState{},
			},
			DirectOwner: u,
		},
	}, nil
}

func NewRebuildIndexTaskFromModel(task mqueue.TaskRecord) mqueue.Task {
	wrapped, ok := task.(*data.TaskModel)
	if !ok {
		return nil
	}
	return &ImportTask{
		DBTask: &queue.DBTask{
			Task: wrapped.Task,
		},
	}
}

func (t *RebuildIndexTask) Do(ctx context.Context) (mqueue.TaskStatus, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	h := dep.Logger().WithContext(ctx)

	state := &RebuildIndexTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to unmarshal state: %s (%w)", err, mqueue.CriticalErr)
	}
	t.state = state

	// List indexable files with metadata
	ctx = context.WithValue(ctx, data.LoadFilePublicMetadata{}, true)
	files, err := dep.FileClient().ListIndexableFiles(ctx, t.state.LastFileID, RebuildIndexBatchSize)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to list indexable files after ID %d: %w", t.state.LastFileID, err)
	}

	if len(files) == 0 {
		h.Infof("Rebuild complete. %d files indexed, %d failed.", t.state.Indexed-t.state.Failed, t.state.Failed)
		return mqueue.StatusCompleted, nil
	}

	// Reindex files for those have been indexed and create index for the left
	reindexDocIDs := make([]string, len(files))
	filesToIndex := make([]rpc.CreateDocumentRequest, 0, len(files))
	fm := manager.NewFileManager(dep, dbfsDep, t.Owner())
	for _, file := range files {
		if file.Edges.Metadata != nil {
			for _, meta := range file.Edges.Metadata {
				if meta.Name == dbfs.FullTextIndexKey {
					reindexDocIDs = append(reindexDocIDs, meta.Value)
					break
				}
			}
		} else {
			traverseFile, err := fm.TraverseFile(ctx, file.ID)
			if err != nil {
				h.Errorf("failed to traverse file %d: %v", file.ID, err)
				continue
			}
			uri := traverseFile.Uri(true).String()
			filesToIndex = append(filesToIndex, rpc.CreateDocumentRequest{
				DocumentName: file.Name,
				Uri:          uri,
				Version:      strconv.Itoa(file.PrimaryEntity),
			})
		}

	}
	if len(reindexDocIDs) > 0 {
		res, err := t.client.BatchReindex(ctx, reindexDocIDs...)
		if err != nil {
			h.Errorf("failed to reindex files: %v", err)
		}
		t.state.Failed += len(reindexDocIDs) - int(res.Total)
	}

	if len(filesToIndex) > 0 {
		knowledge, err := t.client.GetMasterKnowledge(ctx, t.OwnerID())
		if err != nil {
			h.Errorf("failed to get master knowledge: %v", err)
		}
		for _, f := range filesToIndex {
			f.KnowledgeId = knowledge.Id
		}
		h.Infof("Rebuild complete. %d files indexed, %d failed.", t.state.Indexed-t.state.Failed, t.state.Failed)
		_, total, err := t.client.BatchCreateDocuments(ctx, filesToIndex)
		if err != nil {
			h.Errorf("failed to rebuild documents: %v", err)
		}
		t.state.Failed += len(filesToIndex) - int(total)
	}
	t.state.LastFileID = files[len(files)-1].ID

	newStateStr, marshalErr := json.Marshal(t.state)
	if marshalErr != nil {
		return mqueue.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}

	t.Lock()
	t.Task.PrivateState = string(newStateStr)
	t.Unlock()

	t.ResumeAfter(0)
	return mqueue.StatusSuspending, nil
}

func (t *RebuildIndexTask) Summarize(hasher hashid.Encoder) *mqueue.Summary {
	if t.state == nil {
		if err := json.Unmarshal([]byte(t.State()), &t.state); err != nil {
			return nil
		}
	}

	return &mqueue.Summary{
		Phase: string(t.state.Phase),
		Props: map[string]any{
			SummaryKeyFailed: t.state.Failed,
			SummaryKeyTotal:  t.state.Total,
		},
	}
}
