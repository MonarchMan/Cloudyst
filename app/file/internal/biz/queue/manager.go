package queue

import (
	"context"
	"errors"
	"file/internal/biz/setting"
	"file/internal/data"
	"queue"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	QueueManager struct {
		mediaMetaQueue      atomic.Value
		thumbQueue          atomic.Value
		entityRecycleQueue  atomic.Value
		ioIntenseQueue      atomic.Value
		remoteDownloadQueue atomic.Value
		slaveQueue          atomic.Value

		taskClient data.TaskClient
		registry   queue.TaskRegistry
		logger     *log.Helper
		settings   setting.Provider
	}
)

var (
	CriticalErr = errors.New("non-retryable error")
)

func NewQueueManager(taskClient data.TaskClient, registry queue.TaskRegistry, logger log.Logger, settings setting.Provider) (*QueueManager, func()) {
	qm := &QueueManager{
		taskClient: taskClient,
		registry:   registry,
		logger:     log.NewHelper(logger, log.WithMessageKey("biz-queueManager")),
		settings:   settings,
	}
	cleanup := func() {
		wg := sync.WaitGroup{}
		if qm.GetMediaMetaQueue() != nil {
			wg.Add(1)
			go func() {
				qm.GetMediaMetaQueue().Shutdown()
				defer wg.Done()
			}()
		}
		if qm.GetThumbQueue() != nil {
			wg.Add(1)
			go func() {
				qm.GetThumbQueue().Shutdown()
				defer wg.Done()
			}()
		}
		if qm.GetEntityRecycleQueue() != nil {
			wg.Add(1)
			go func() {
				qm.GetEntityRecycleQueue().Shutdown()
				defer wg.Done()
			}()
		}
		if qm.GetIoIntenseQueue() != nil {
			wg.Add(1)
			go func() {
				qm.GetIoIntenseQueue().Shutdown()
				defer wg.Done()
			}()
		}
		if qm.GetRemoteDownloadQueue() != nil {
			wg.Add(1)
			go func() {
				qm.GetRemoteDownloadQueue().Shutdown()
				defer wg.Done()
			}()
		}
		if qm.GetSlaveQueue() != nil {
			wg.Add(1)
			go func() {
				qm.GetSlaveQueue().Shutdown()
				defer wg.Done()
			}()
		}
		wg.Wait()
	}
	return qm, cleanup
}

func (m *QueueManager) Reload(ctx context.Context) error {
	var errs []error
	errs = append(errs, m.ReloadMediaMetaQueue(ctx))
	errs = append(errs, m.ReloadThumbQueue(ctx))
	errs = append(errs, m.ReloadEntityRecycleQueue(ctx))
	errs = append(errs, m.ReloadIoIntenseQueue(ctx))
	errs = append(errs, m.ReloadRemoteDownloadQueue(ctx))
	errs = append(errs, m.ReloadSlaveQueue(ctx))

	var notNilErrs []error
	for _, err := range errs {
		if err != nil {
			notNilErrs = append(notNilErrs, err)
		}
	}

	return errors.Join(notNilErrs...)
}

func (m *QueueManager) ReloadMediaMetaQueue(ctx context.Context) error {
	old := m.GetMediaMetaQueue()
	if old != nil {
		old.Shutdown()
	}
	settings := m.settings
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeMediaMeta)
	newQueue := queue.New(m.logger, m.taskClient, nil, ctx,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("MediaMetadataQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
		queue.WithResumeTaskType(MediaMetaTaskType),
	)
	m.mediaMetaQueue.Store(newQueue)
	return nil
}

func (m *QueueManager) ReloadThumbQueue(ctx context.Context) error {
	old := m.GetThumbQueue()
	if old != nil {
		old.Shutdown()
	}
	settings := m.settings
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeThumb)
	newQueue := queue.New(m.logger, m.taskClient, nil, ctx,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("ThumbQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
	)
	m.thumbQueue.Store(newQueue)
	return nil
}

func (m *QueueManager) ReloadEntityRecycleQueue(ctx context.Context) error {
	old := m.GetEntityRecycleQueue()
	if old != nil {
		old.Shutdown()
	}
	settings := m.settings
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeEntityRecycle)
	newQueue := queue.New(m.logger, m.taskClient, nil, ctx,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("EntityRecycleQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
		queue.WithResumeTaskType(EntityRecycleRoutineTaskType, ExplicitEntityRecycleTaskType, UploadSentinelCheckTaskType),
		queue.WithTaskPullInterval(10*time.Second),
	)
	m.entityRecycleQueue.Store(newQueue)
	return nil
}

func (m *QueueManager) ReloadIoIntenseQueue(ctx context.Context) error {
	old := m.GetIoIntenseQueue()
	if old != nil {
		old.Shutdown()
	}
	settings := m.settings
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeIOIntense)
	newQueue := queue.New(m.logger, m.taskClient, m.registry, ctx,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("IoIntenseQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
		queue.WithResumeTaskType(CreateArchiveTaskType, ExtractArchiveTaskType, RelocateTaskType, ImportTaskType),
		queue.WithTaskPullInterval(10*time.Second),
	)
	m.ioIntenseQueue.Store(newQueue)
	return nil
}

func (m *QueueManager) ReloadRemoteDownloadQueue(ctx context.Context) error {
	old := m.GetRemoteDownloadQueue()
	if old != nil {
		old.Shutdown()
	}
	settings := m.settings
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeRemoteDownload)
	newQueue := queue.New(m.logger, m.taskClient, m.registry, ctx,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("RemoteDownloadQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
		queue.WithResumeTaskType(RemoteDownloadTaskType),
		queue.WithTaskPullInterval(10*time.Second),
	)
	m.remoteDownloadQueue.Store(newQueue)
	return nil
}

func (m *QueueManager) ReloadSlaveQueue(ctx context.Context) error {
	old := m.GetSlaveQueue()
	if old != nil {
		old.Shutdown()
	}
	settings := m.settings
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeSlave)
	newQueue := queue.New(m.logger, nil, nil, ctx,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("SlaveQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
	)
	m.slaveQueue.Store(newQueue)
	return nil
}

func (m *QueueManager) GetMediaMetaQueue() queue.Queue {
	if v, ok := m.mediaMetaQueue.Load().(queue.Queue); ok {
		return v
	}
	return nil
}

func (m *QueueManager) GetThumbQueue() queue.Queue {
	if v, ok := m.thumbQueue.Load().(queue.Queue); ok {
		return v
	}
	return nil
}
func (m *QueueManager) GetEntityRecycleQueue() queue.Queue {
	if v, ok := m.entityRecycleQueue.Load().(queue.Queue); ok {
		return v
	}
	return nil
}
func (m *QueueManager) GetIoIntenseQueue() queue.Queue {
	if v, ok := m.ioIntenseQueue.Load().(queue.Queue); ok {
		return v
	}
	return nil
}
func (m *QueueManager) GetRemoteDownloadQueue() queue.Queue {
	if v, ok := m.remoteDownloadQueue.Load().(queue.Queue); ok {
		return v
	}
	return nil
}

func (m *QueueManager) GetSlaveQueue() queue.Queue {
	if v, ok := m.slaveQueue.Load().(queue.Queue); ok {
		return v
	}
	return nil
}
