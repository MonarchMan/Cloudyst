package queue

import (
	"ai/internal/data"
	"ai/internal/data/rpc"
	"api/external/data/common"
	"context"
	"fmt"
	"queue"
	"sync"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	QueueTypeIngest  = "IngestQueue"
	QueueTypeReindex = "ReindexQueue"
)

type QueueManager struct {
	ingestQueue   atomic.Value
	retrieveQueue atomic.Value

	taskClient data.TaskClient
	logger     *log.Helper
	settings   rpc.SettingClient
}

func NewQueueManager(taskClient data.TaskClient, logger log.Logger, settings rpc.SettingClient) (*QueueManager, func()) {
	qm := &QueueManager{
		taskClient: taskClient,
		logger:     log.NewHelper(logger, log.WithMessageKey("biz-queueManager")),
		settings:   settings,
	}
	cleanup := func() {
		wg := sync.WaitGroup{}
		if qm.IngestQueue() != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				qm.IngestQueue().Shutdown()
			}()
		}
		if qm.ReindexQueue() != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				qm.ReindexQueue().Shutdown()
			}()
		}
		wg.Wait()
	}
	return qm, cleanup
}

func (m *QueueManager) IngestQueue() queue.Queue {
	if v, ok := m.ingestQueue.Load().(queue.Queue); ok {
		return v
	}
	return nil
}

func (m *QueueManager) ReindexQueue() queue.Queue {
	if v, ok := m.retrieveQueue.Load().(queue.Queue); ok {
		return v
	}
	return nil
}

func (m *QueueManager) ReloadIngestQueue(ctx context.Context) error {
	old := m.IngestQueue()
	if old != nil {
		old.Shutdown()
	}
	queueSettings, err := m.settings.Queue(ctx, QueueTypeIngest)
	if err != nil {
		return err
	}

	newQueue := queue.New(m.logger, m.taskClient, nil, ctx,
		queue.WithBackoffFactor(queueSettings.BackoffFactor),
		queue.WithMaxRetry(queueSettings.MaxRetry),
		queue.WithBackoffMaxDuration(queueSettings.BackoffMaxDuration),
		queue.WithRetryDelay(queueSettings.RetryDelay),
		queue.WithWorkerCount(queueSettings.WorkerNum),
		queue.WithName(QueueTypeIngest),
		queue.WithMaxTaskExecution(queueSettings.MaxExecution),
		queue.WithResumeTaskType(IngestTaskType),
	)
	m.ingestQueue.Store(newQueue)
	return nil
}
func (m *QueueManager) ReloadReindexQueue(ctx context.Context) error {
	old := m.ReindexQueue()
	if old != nil {
		old.Shutdown()
	}
	queueSettings, err := m.settings.Queue(ctx, QueueTypeReindex)
	if err != nil {
		return err
	}
	newQueue := queue.New(m.logger, m.taskClient, nil, ctx,
		queue.WithBackoffFactor(queueSettings.BackoffFactor),
		queue.WithMaxRetry(queueSettings.MaxRetry),
		queue.WithBackoffMaxDuration(queueSettings.BackoffMaxDuration),
		queue.WithRetryDelay(queueSettings.RetryDelay),
		queue.WithWorkerCount(queueSettings.WorkerNum),
		queue.WithName(QueueTypeReindex),
		queue.WithMaxTaskExecution(queueSettings.MaxExecution),
		queue.WithResumeTaskType(ReindexTaskType),
	)
	m.retrieveQueue.Store(newQueue)
	return nil
}

func (m *QueueManager) ResumeTasks(ctx context.Context, ids []int, taskType string) error {
	if len(ids) == 0 {
		return nil
	}
	res, err := m.taskClient.List(ctx, &data.ListTaskArgs{
		PaginationArgs: &common.PaginationArgs{
			PageSize: len(ids),
		},
		IDs:    ids,
		Types:  []string{taskType},
		Status: []queue.TaskStatus{queue.StatusSuspending},
	})
	if err != nil {
		return err
	}

	var target queue.Queue
	switch taskType {
	case IngestTaskType:
		target = m.IngestQueue()
	case ReindexTaskType:
		target = m.ReindexQueue()
	default:
		return fmt.Errorf("invalid task type: %s", taskType) ///
	}
	if target == nil {
		return fmt.Errorf("queue for task type %s is not initialized", taskType)
	}

	for _, model := range res.Tasks {
		t, err := queue.NewTaskFromModel(data.NewTaskModel(model))
		if err != nil {
			return err
		}
		if err := target.QueueTask(ctx, t); err != nil {
			return err
		}
	}
	return nil
}
