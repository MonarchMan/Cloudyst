package task

import (
	"ai/ent"
	"ai/internal/biz/knowledge/rag/ingestion"
	"ai/internal/biz/queue"
	"ai/internal/data"
	"ai/internal/data/rpc"
	"ai/internal/data/vector"
	"api/external/data/userdata"
	"common/util"
	"context"
	"encoding/json"
	"fmt"
	mqueue "queue"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
)

type (
	ReindexTask struct {
		*queue.DBTask
		progress mqueue.Progresses
		state    *ReindexTaskState
		l        *log.Helper
	}

	ReindexTaskPhase string
	ReindexTaskState struct {
		Phase     ReindexTaskPhase `json:"phase"`
		Total     int              `json:"total"`
		Indexed   int              `json:"indexed"`
		LastDocID int              `json:"last_doc_id"`
		Failed    int              `json:"failed"`
		DocIDs    []int            `json:"doc_ids"`
		// CheckPointID and InterruptIDs are persisted when Eino pauses the graph.
		CheckPointID string   `json:"checkpoint_id,omitempty"`
		InterruptIDs []string `json:"interrupt_ids,omitempty"`
	}
)

const (
	ReindexPhaseNuke  ReindexTaskPhase = "nuke"
	ReindexPhaseIndex ReindexTaskPhase = "index"

	ReindexBatchSize = 1000

	ProgressTypeReindexCount  = "reindex_count"
	ProgressTypeReindexSize   = "reindex_size"
	ProgressTypeReindexTokens = "reindex_tokens"
)

func init() {
	mqueue.RegisterResumableTaskFactory(queue.ReindexTaskType, NewReindexTaskFromModel)
}

func NewReindexTask(ctx context.Context, u *userdata.User, docIDs ...int) (*ReindexTask, error) {
	state := &ReindexTaskState{
		Phase:  ReindexPhaseNuke,
		DocIDs: docIDs,
		Total:  len(docIDs),
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}
	return &ReindexTask{
		DBTask: &queue.DBTask{
			DirectOwner: u,
			Task: &ent.Task{
				Type:         queue.ReindexTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &mqueue.TaskPublicState{},
			},
		},
	}, nil
}

func NewReindexTaskFromModel(task mqueue.TaskRecord) mqueue.Task {
	wrapped, ok := task.(*data.TaskModel)
	if !ok {
		return nil
	}
	return &ReindexTask{
		DBTask: &queue.DBTask{
			Task: wrapped.Task,
		},
	}
}

func (t *ReindexTask) Do(ctx context.Context) (mqueue.TaskStatus, error) {
	t.Lock()
	if t.progress == nil {
		t.progress = make(mqueue.Progresses)
	}
	t.progress[ProgressTypeReindexCount] = &mqueue.Progress{}
	t.progress[ProgressTypeReindexSize] = &mqueue.Progress{}
	t.progress[ProgressTypeReindexTokens] = &mqueue.Progress{}
	t.Unlock()

	state := &ReindexTaskState{}
	if err := json.Unmarshal([]byte(t.Task.PrivateState), state); err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to unmarshal state: %s (%w)", err, mqueue.CriticalErr)
	}
	t.state = state

	if t.l != nil {
		t.l.Infof("Found %d indexable files, starting rebuild...", t.state.Total)
	}
	var (
		next = mqueue.StatusCompleted
		err  error
	)
	dep := IngestEngineFromContext(ctx)
	switch t.state.Phase {
	case ReindexPhaseNuke, "":
		next, err = t.nuke(ctx, dep.VectorStore)
	case ReindexPhaseIndex:
		next, err = t.index(ctx, dep.Engine, dep.DocumentClient, dep.FileClient)
	default:
		next, err = mqueue.StatusError, fmt.Errorf("unknown phase %q: %w", t.state.Phase, mqueue.CriticalErr)

	}

	newStateStr, marshalErr := json.Marshal(t.state)
	if marshalErr != nil {
		return mqueue.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}

	t.Lock()
	t.Task.PrivateState = string(newStateStr)
	t.Unlock()
	return next, err
}

func (t *ReindexTask) nuke(ctx context.Context, vs vector.VectorStore) (mqueue.TaskStatus, error) {
	if t.l != nil {
		t.l.Info("Deleting all existing index documents...")
	}

	err := vs.DeleteByDocIDs(ctx, t.state.DocIDs)
	if err != nil {
		return mqueue.StatusError, err
	}

	t.state.Phase = ReindexPhaseIndex
	t.state.LastDocID = 0
	t.state.Indexed = 0
	t.ResumeAfter(0)
	return mqueue.StatusSuspending, nil
}

func (t *ReindexTask) index(ctx context.Context, engine *ingestion.IngestEngine, kdc data.KnowledgeDocumentClient, fc rpc.FileClient) (mqueue.TaskStatus, error) {
	atomic.StoreInt64(&t.progress[ProgressTypeReindexCount].Total, int64(t.state.Total))
	atomic.StoreInt64(&t.progress[ProgressTypeReindexCount].Current, int64(t.state.Indexed))

	// 1. Get documents
	docs, err := kdc.ListIndexable(ctx, t.state.LastDocID, ReindexBatchSize)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to list indexable files after ID %d: %w", t.state.LastDocID, err)
	}

	if len(docs) == 0 {
		t.l.Infof("Rebuild complete. %d documents indexed, %d failed.", t.state.Indexed-t.state.Failed, t.state.Failed)
		return mqueue.StatusCompleted, nil
	}

	// 2. Get file download links
	uris := lo.Map(docs, func(item *ent.AiKnowledgeDocument, index int) string {
		return item.URL
	})
	dUrlResp, err := fc.GetFileUrl(ctx, uris)
	if err != nil {
		return mqueue.StatusError, err
	}
	// 3. Rebuild index of documents
	dUrls := make([]string, len(dUrlResp.Urls))
	infos := make([]ingestion.DocumentInfo, len(docs))
	for i, doc := range docs {
		infos[i] = ingestion.DocumentInfo{
			ID:          doc.ID,
			Name:        doc.Name,
			Version:     doc.Version,
			Url:         doc.URL,
			KnowledgeID: doc.KnowledgeID,
		}
		dUrls[i] = dUrlResp.Urls[i].Url
	}
	progressDocumentFunc := func(doc *ent.AiKnowledgeDocument) {
		if doc == nil {
			return
		}
		t.Lock()
		atomic.AddInt64(&t.progress[ProgressTypeReindexCount].Current, 1)
		atomic.AddInt64(&t.progress[ProgressTypeReindexSize].Current, int64(doc.ContentLength))
		atomic.AddInt64(&t.progress[ProgressTypeReindexTokens].Current, int64(doc.Tokens))
		t.state.Indexed++
		t.state.LastDocID = doc.ID
		t.Unlock()
	}

	if len(docs) == 1 {
		if err := engine.Ingest(ctx, &infos[0], dUrls[0], progressDocumentFunc); err != nil {
			if interrupted, ok := ingestion.IsIngestInterrupted(err); ok {
				return t.suspendForInterrupt(interrupted)
			}
			return mqueue.StatusError, err
		}
	} else {
		batchFailed, err := engine.BatchIngest(ctx, infos, dUrls, progressDocumentFunc)
		t.state.Failed += batchFailed
		if err != nil {
			if interrupted, ok := ingestion.IsIngestInterrupted(err); ok {
				return t.suspendForInterrupt(interrupted)
			}
			return mqueue.StatusError, err
		}
	}

	// Suspend and resume for next batch
	t.ResumeAfter(0)
	return mqueue.StatusSuspending, nil
}

func (t *ReindexTask) suspendForInterrupt(err *ingestion.IngestInterruptedError) (mqueue.TaskStatus, error) {
	t.state.CheckPointID = err.CheckPointID
	t.state.InterruptIDs = err.InterruptIDs()
	return mqueue.StatusSuspending, nil
}

func (t *ReindexTask) RequireManualResume() bool {
	return t.state != nil && len(t.state.InterruptIDs) > 0
}
