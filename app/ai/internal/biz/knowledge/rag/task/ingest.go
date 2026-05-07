package task

import (
	"ai/ent"
	"ai/internal/biz/knowledge/rag/ingestion"
	"ai/internal/biz/queue"
	"ai/internal/data"
	"ai/internal/data/rpc"
	"api/external/data/userdata"
	"common/hashid"
	"common/util"
	"context"
	"encoding/json"
	"fmt"
	mqueue "queue"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
)

func init() {
	mqueue.RegisterResumableTaskFactory(queue.IngestTaskType, NewIngestTaskFromModel)
}

type (
	IngestTask struct {
		*queue.DBTask
		state    *IngestTaskState
		progress mqueue.Progresses
		l        *log.Helper
	}
	IngestTaskPhase string
	IngestTaskState struct {
		DocIDs    []int           `json:"doc_ids"`
		Phase     IngestTaskPhase `json:"phase"`
		Failed    int             `json:"failed"`
		LastDocID int             `json:"last_doc_id"`
		Indexed   int             `json:"indexed"`
		Total     int             `json:"total"`
		// CheckPointID and InterruptIDs are persisted when Eino pauses the graph.
		CheckPointID string   `json:"checkpoint_id,omitempty"`
		InterruptIDs []string `json:"interrupt_ids,omitempty"`
	}
)

const (
	ProgressTypeIngestCount = "ingest_count"
	ProgressTypeIngestSize  = "ingest_size"
	ProgressTypeTokens      = "tokens"
)

func NewIngestTask(ctx context.Context, creator *userdata.User, l *log.Helper, docIDs ...int) (*IngestTask, error) {
	state := &IngestTaskState{
		DocIDs: docIDs,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}
	return &IngestTask{
		DBTask: &queue.DBTask{
			DirectOwner: creator,
			Task: &ent.Task{
				Type:         queue.IngestTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &mqueue.TaskPublicState{},
			},
		},
		l: l.WithContext(ctx),
	}, nil
}

func NewIngestTaskFromModel(task mqueue.TaskRecord) mqueue.Task {
	wrapped, ok := task.(*data.TaskModel)
	if !ok {
		return nil
	}
	return &IngestTask{
		DBTask: &queue.DBTask{
			Task: wrapped.Task,
		},
	}
}

func (t *IngestTask) Do(ctx context.Context) (mqueue.TaskStatus, error) {
	t.Lock()
	if t.progress == nil {
		t.progress = make(mqueue.Progresses)
	}
	t.progress[ProgressTypeIngestCount] = &mqueue.Progress{}
	t.progress[ProgressTypeIngestSize] = &mqueue.Progress{}
	t.progress[ProgressTypeTokens] = &mqueue.Progress{}
	t.Unlock()

	// 1. Unmarshal state
	state := &IngestTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to unmarshal state: %s (%w)", err, mqueue.CriticalErr)
	}
	t.state = state

	if len(state.DocIDs) == 0 {
		return mqueue.StatusCompleted, nil
	}

	dep := IngestEngineFromContext(ctx)
	status, err := t.performIndexing(ctx, dep.Engine, dep.DocumentClient, dep.FileClient)

	if err := t.saveState(); err != nil {
		return mqueue.StatusError, err
	}
	return status, err
}

func (t *IngestTask) performIndexing(ctx context.Context, engine *ingestion.IngestEngine, kdc data.KnowledgeDocumentClient, fc rpc.FileClient) (mqueue.TaskStatus, error) {
	// 1. Get documents
	docs, err := kdc.ListIndexable(ctx, t.state.LastDocID, ReindexBatchSize)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to list indexable files after ID %d: %w", t.state.LastDocID, err)
	}
	// 2. Get file download links
	uris := lo.Map(docs, func(item *ent.AiKnowledgeDocument, index int) string {
		return item.URL
	})
	dUrlResp, err := fc.GetFileUrl(ctx, uris)
	if err != nil {
		return mqueue.StatusError, err
	}

	// 3. Batch ingest documents
	dUrls := make([]string, len(docs))
	infos := make([]ingestion.DocumentInfo, len(docs))
	for i, doc := range docs {
		infos[i] = ingestion.DocumentInfo{
			KnowledgeID: doc.KnowledgeID,
			Name:        doc.Name,
			Version:     doc.Version,
			Url:         doc.URL,
			ID:          doc.ID,
		}
		dUrls[i] = dUrlResp.Urls[i].Url
	}
	progressDocumentFunc := func(doc *ent.AiKnowledgeDocument) {
		if doc == nil {
			return
		}
		t.Lock()
		atomic.AddInt64(&t.progress[ProgressTypeIngestCount].Current, 1)
		atomic.AddInt64(&t.progress[ProgressTypeIngestSize].Current, int64(doc.ContentLength))
		atomic.AddInt64(&t.progress[ProgressTypeTokens].Current, int64(doc.Tokens))
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
		failed, err := engine.BatchIngest(ctx, infos, dUrls, progressDocumentFunc)
		t.state.Failed += failed
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

func (t *IngestTask) suspendForInterrupt(err *ingestion.IngestInterruptedError) (mqueue.TaskStatus, error) {
	t.state.CheckPointID = err.CheckPointID
	t.state.InterruptIDs = err.InterruptIDs()
	return mqueue.StatusSuspending, nil
}

func (t *IngestTask) RequireManualResume() bool {
	return t.state != nil && len(t.state.InterruptIDs) > 0
}

func (t *IngestTask) saveState() error {
	stateBytes, err := json.Marshal(t.state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	t.Lock()
	t.Task.PrivateState = string(stateBytes)
	t.Unlock()
	return nil
}

func (t *IngestTask) Summarize(hasher hashid.Encoder) *mqueue.Summary {
	// Unmarshal state
	if t.state == nil {
		if err := json.Unmarshal([]byte(t.State()), &t.state); err != nil {
			return nil
		}
	}

	return &mqueue.Summary{
		NodeID: 0,
		Phase:  string(t.state.Phase),
		Props: map[string]any{
			"doc_ids": t.state.DocIDs,
			"failed":  t.state.Failed,
		},
	}
}

func (t *IngestTask) Progress(ctx context.Context) mqueue.Progresses {
	t.Lock()
	defer t.Unlock()
	return t.progress
}
