package queue

import (
	"api/external/trans"
	"context"
	"errors"
	"file/ent/task"
	"file/internal/biz/setting"
	"file/internal/data"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/jpillora/backoff"
)

type (
	Queue interface {
		// Start resume tasks and starts all workers.
		Start()
		// Shutdown stops all workers.
		Shutdown()
		// QueueTask submits a Task to the queue.
		QueueTask(ctx context.Context, t Task) error
		// BusyWorkers returns the numbers of workers in the running process.
		BusyWorkers() int32
		// SuccessTasks returns the numbers of success tasks.
		SuccessTasks() int32
		// FailureTasks returns the numbers of failure tasks.
		FailureTasks() int32
		// SubmittedTasks returns the numbers of submitted tasks.
		SubmittedTasks() int32
		// SuspendingTasks returns the numbers of suspending tasks.
		SuspendingTasks() int32
	}
	queue struct {
		sync.Mutex
		routineGroup *routineGroup
		metric       *metric
		quit         chan struct{}
		ready        chan struct{}
		scheduler    Scheduler
		stopOnce     sync.Once
		stopFlag     int32
		rootCtx      context.Context
		cancel       context.CancelFunc

		// Dependencies
		logger     *log.Helper
		taskClient data.TaskClient
		registry   TaskRegistry
		settings   setting.Provider

		// Options
		*options
	}

	Dep interface {
		ForkWithLogger(ctx context.Context, l *log.Helper) context.Context
	}

	QueueManager struct {
		mediaMetaQueue      atomic.Value
		thumbQueue          atomic.Value
		entityRecycleQueue  atomic.Value
		ioIntenseQueue      atomic.Value
		remoteDownloadQueue atomic.Value
		slaveQueue          atomic.Value

		taskClient data.TaskClient
		registry   TaskRegistry
		logger     *log.Helper
		settings   setting.Provider
	}
)

var (
	CriticalErr = errors.New("non-retryable error")
)

func New(l *log.Helper, taskClient data.TaskClient, registry TaskRegistry, ctx context.Context, opts ...Option) Queue {
	o := newDefaultOptions()
	for _, opt := range opts {
		opt.apply(o)
	}

	ctx, cancel := context.WithCancel(ctx)

	return &queue{
		routineGroup: newRoutineGroup(),
		scheduler:    NewFifoScheduler(0, l),
		quit:         make(chan struct{}),
		ready:        make(chan struct{}, 1),
		metric:       &metric{},
		options:      o,
		logger:       l,
		registry:     registry,
		taskClient:   taskClient,
		rootCtx:      ctx,
		cancel:       cancel,
	}
}

// Start to enable all worker
func (q *queue) Start() {
	q.routineGroup.Run(func() {
		// Resume tasks in DB
		if len(q.options.resumeTaskType) > 0 && q.taskClient != nil {

			ctx := context.TODO()
			ctx = context.WithValue(ctx, data.LoadTaskUser{}, true)
			tasks, err := q.taskClient.GetPendingTasks(ctx, q.resumeTaskType...)
			if err != nil {
				q.logger.Warnf("Failed to get pending tasks from DB for given type %v: %s", q.resumeTaskType, err)
			}

			resumed := 0
			for _, t := range tasks {
				resumedTask, err := NewTaskFromModel(t)
				if err != nil {
					q.logger.Warnf("Failed to resume task %d: %s", t.ID, err)
					continue
				}

				if resumedTask.Status() == task.StatusSuspending {
					q.metric.IncSuspendingTask()
					q.metric.IncSubmittedTask()
				}

				if err := q.QueueTask(ctx, resumedTask); err != nil {
					q.logger.Warnf("Failed to resume task %d: %s", t.ID, err)
				}
				resumed++
			}

			q.logger.Infof("Resumed %d tasks from DB.", resumed)
		}

		q.start()
	})
	q.logger.Infof("Queue %q started with %d workers.", q.name, q.workerCount)
}

// Shutdown stops all queues.
func (q *queue) Shutdown() {
	q.logger.Infof("Shutting down queue %q...", q.name)
	defer func() {
		q.routineGroup.Wait()
	}()

	if !atomic.CompareAndSwapInt32(&q.stopFlag, 0, 1) {
		return
	}

	q.stopOnce.Do(func() {
		q.cancel()
		if q.metric.BusyWorkers() > 0 {
			q.logger.Infof("shutdown all tasks in queue %q: %d workers", q.name, q.metric.BusyWorkers())
		}

		if err := q.scheduler.Shutdown(); err != nil {
			q.logger.Error("failed to shutdown scheduler in queue %q: %w", q.name, err)
		}
		close(q.quit)
	})

}

// BusyWorkers returns the numbers of workers in the running process.
func (q *queue) BusyWorkers() int32 {
	return int32(q.metric.BusyWorkers())
}

// BusyWorkers returns the numbers of success tasks.
func (q *queue) SuccessTasks() int32 {
	return int32(q.metric.SuccessTasks())
}

// BusyWorkers returns the numbers of failure tasks.
func (q *queue) FailureTasks() int32 {
	return int32(q.metric.FailureTasks())
}

// BusyWorkers returns the numbers of submitted tasks.
func (q *queue) SubmittedTasks() int32 {
	return int32(q.metric.SubmittedTasks())
}

// SuspendingTasks returns the numbers of suspending tasks.
func (q *queue) SuspendingTasks() int32 {
	return int32(q.metric.SuspendingTasks())
}

// QueueTask to queue single Task
func (q *queue) QueueTask(ctx context.Context, t Task) error {
	if atomic.LoadInt32(&q.stopFlag) == 1 {
		return ErrQueueShutdown
	}

	if t.Status() != task.StatusSuspending {
		q.metric.IncSubmittedTask()
		if err := q.transitStatus(ctx, t, task.StatusQueued); err != nil {
			return err
		}
	}

	if err := q.scheduler.Queue(t); err != nil {
		return err
	}
	owner := ""
	if t.Owner() != nil {
		owner = t.Owner().Email
	}
	q.logger.Infof("New Task with type %q submitted to queue %q by %q", t.Type(), q.name, owner)
	if q.registry != nil {
		q.registry.Set(t.ID(), t)
	}

	return nil
}

// newContext creates a new context for a new Task iteration.
func (q *queue) newContext(t Task) context.Context {
	//l := q.logger.CopyWithPrefix(fmt.Sprintf("[Cid: %s TaskID: %d Queue: %s]", t.TraceID(), t.ID(), q.name))
	//ctx := q.dep.ForkWithLogger(q.rootCtx, l)
	ctx := context.WithValue(q.rootCtx, trans.UserCtx{}, t.Owner())
	//ctx = context.WithValue(ctx, filemanager.ManagerDepCtx{}, q.managerDep)
	//ctx = context.WithValue(ctx, filemanager.DbfsDepCtx{}, q.dbfsDep)
	//ctx = context.WithValue(ctx, filemanager.NodePoolCtx{}, q.nodePool)
	return ctx
}

func (q *queue) work(t Task) {
	ctx := q.newContext(t)
	timeIterationStart := time.Now()

	var err error
	// to handle panic cases from inside the worker
	// in such case, we start a new goroutine
	defer func() {
		q.metric.DecBusyWorker()
		e := recover()
		if e != nil {
			q.logger.WithContext(ctx).Errorf("Panic error in queue %q: %v", q.name, e)
			t.OnError(fmt.Errorf("panic error: %v", e), time.Since(timeIterationStart))

			_ = q.transitStatus(ctx, t, task.StatusError)
		}
		q.schedule()
	}()

	err = q.transitStatus(ctx, t, task.StatusProcessing)
	if err != nil {
		q.logger.WithContext(ctx).Error("failed to transit task %d to processing: %s", t.ID(), err.Error())
		panic(err)
	}

	for {
		timeIterationStart = time.Now()
		var next task.Status
		next, err = q.run(ctx, t)
		if err != nil {
			t.OnError(err, time.Since(timeIterationStart))
			q.logger.WithContext(ctx).Error("runtime error in queue %q: %s", q.name, err.Error())

			_ = q.transitStatus(ctx, t, task.StatusError)
			break
		}

		// iteration completes
		t.OnIterationComplete(time.Since(timeIterationStart))
		_ = q.transitStatus(ctx, t, next)
		if next != task.StatusProcessing {
			break
		}
	}
}

func (q *queue) run(ctx context.Context, t Task) (task.Status, error) {
	// create channel with buffer size 1 to avoid goroutine leak
	done := make(chan struct {
		err  error
		next task.Status
	}, 1)
	panicChan := make(chan interface{}, 1)
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(ctx, q.maxTaskExecution-t.Executed())
	defer func() {
		cancel()
	}()

	// run the job
	go func() {
		// handle panic issue
		defer func() {
			if p := recover(); p != nil {
				panicChan <- p
			}
		}()

		q.logger.WithContext(ctx).Debug("Iteration started.")
		next, err := t.Do(ctx)
		q.logger.WithContext(ctx).Debugf("Iteration ended with err=%s", err)
		if err != nil && q.maxRetry-t.Retried() > 0 && !errors.Is(err, CriticalErr) && atomic.LoadInt32(&q.stopFlag) != 1 {
			// Retry needed
			t.OnRetry(err)
			b := &backoff.Backoff{
				Max:    q.backoffMaxDuration,
				Factor: q.backoffFactor,
			}
			delay := q.retryDelay
			if q.retryDelay == 0 {
				delay = b.ForAttempt(float64(t.Retried()))
			}

			// Resume after to retry
			q.logger.WithContext(ctx).Infof("Will be retried in %s", delay)
			t.OnSuspend(time.Now().Add(delay).Unix())
			err = nil
			next = task.StatusSuspending
		}

		done <- struct {
			err  error
			next task.Status
		}{err: err, next: next}
	}()

	select {
	case p := <-panicChan:
		panic(p)
	case <-ctx.Done(): // timeout reached
		return task.StatusError, ctx.Err()
	case <-q.quit: // shutdown service
		// cancel job
		cancel()

		leftTime := q.maxTaskExecution - t.Executed() - time.Since(startTime)
		// wait job
		select {
		case <-time.After(leftTime):
			return task.StatusError, context.DeadlineExceeded
		case r := <-done: // job finish
			return r.next, r.err
		case p := <-panicChan:
			panic(p)
		}
	case r := <-done: // job finish
		return r.next, r.err
	}
}

// beforeTaskStart updates Task status from queued to processing
func (q *queue) transitStatus(ctx context.Context, t Task, to task.Status) (err error) {
	old := t.Status()
	transition, ok := stateTransitions[t.Status()][to]
	if !ok {
		err = fmt.Errorf("invalid state transition from %s to %s", old, to)
	} else {
		if innerErr := transition(ctx, t, to, q); innerErr != nil {
			err = fmt.Errorf("failed to transit Task status from %s to %s: %w", old, to, innerErr)
		}
	}

	if err != nil {
		q.logger.WithContext(ctx).Error(err.Error())
	}

	q.logger.WithContext(ctx).Infof("Task %d status changed from %q to %q.", t.ID(), old, to)
	return
}

// schedule to check worker number
func (q *queue) schedule() {
	q.Lock()
	defer q.Unlock()
	if q.BusyWorkers() >= int32(q.workerCount) {
		return
	}

	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// start to start all worker
func (q *queue) start() {
	tasks := make(chan Task, 1)

	for {
		// check worker number
		q.schedule()

		select {
		// wait worker ready
		case <-q.ready:
		case <-q.quit:
			return
		}

		// request Task from queue in background
		q.routineGroup.Run(func() {
			for {
				t, err := q.scheduler.Request()
				if t == nil || err != nil {
					if err != nil {
						select {
						case <-q.quit:
							if !errors.Is(err, ErrNoTaskInQueue) {
								close(tasks)
								return
							}
						case <-time.After(q.taskPullInterval):
							// sleep to fetch new Task
						}
					}
				}
				if t != nil {
					tasks <- t
					return
				}

				select {
				case <-q.quit:
					if !errors.Is(err, ErrNoTaskInQueue) {
						close(tasks)
						return
					}
				default:
				}
			}
		})

		t, ok := <-tasks
		if !ok {
			return
		}

		// start new Task
		q.metric.IncBusyWorker()
		q.routineGroup.Run(func() {
			q.work(t)
		})
	}
}

func NewQueueManager(taskClient data.TaskClient, registry TaskRegistry, logger log.Logger, settings setting.Provider) (*QueueManager, func()) {
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
	newQueue := New(m.logger, m.taskClient, nil, ctx,
		WithBackoffFactor(queueSetting.BackoffFactor),
		WithMaxRetry(queueSetting.MaxRetry),
		WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		WithRetryDelay(queueSetting.RetryDelay),
		WithWorkerCount(queueSetting.WorkerNum),
		WithName("MediaMetadataQueue"),
		WithMaxTaskExecution(queueSetting.MaxExecution),
		WithResumeTaskType(MediaMetaTaskType),
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
	newQueue := New(m.logger, m.taskClient, nil, ctx,
		WithBackoffFactor(queueSetting.BackoffFactor),
		WithMaxRetry(queueSetting.MaxRetry),
		WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		WithRetryDelay(queueSetting.RetryDelay),
		WithWorkerCount(queueSetting.WorkerNum),
		WithName("ThumbQueue"),
		WithMaxTaskExecution(queueSetting.MaxExecution),
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
	newQueue := New(m.logger, m.taskClient, nil, ctx,
		WithBackoffFactor(queueSetting.BackoffFactor),
		WithMaxRetry(queueSetting.MaxRetry),
		WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		WithRetryDelay(queueSetting.RetryDelay),
		WithWorkerCount(queueSetting.WorkerNum),
		WithName("EntityRecycleQueue"),
		WithMaxTaskExecution(queueSetting.MaxExecution),
		WithResumeTaskType(EntityRecycleRoutineTaskType, ExplicitEntityRecycleTaskType, UploadSentinelCheckTaskType),
		WithTaskPullInterval(10*time.Second),
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
	newQueue := New(m.logger, m.taskClient, m.registry, ctx,
		WithBackoffFactor(queueSetting.BackoffFactor),
		WithMaxRetry(queueSetting.MaxRetry),
		WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		WithRetryDelay(queueSetting.RetryDelay),
		WithWorkerCount(queueSetting.WorkerNum),
		WithName("IoIntenseQueue"),
		WithMaxTaskExecution(queueSetting.MaxExecution),
		WithResumeTaskType(CreateArchiveTaskType, ExtractArchiveTaskType, RelocateTaskType, ImportTaskType),
		WithTaskPullInterval(10*time.Second),
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
	newQueue := New(m.logger, m.taskClient, m.registry, ctx,
		WithBackoffFactor(queueSetting.BackoffFactor),
		WithMaxRetry(queueSetting.MaxRetry),
		WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		WithRetryDelay(queueSetting.RetryDelay),
		WithWorkerCount(queueSetting.WorkerNum),
		WithName("RemoteDownloadQueue"),
		WithMaxTaskExecution(queueSetting.MaxExecution),
		WithResumeTaskType(RemoteDownloadTaskType),
		WithTaskPullInterval(10*time.Second),
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
	newQueue := New(m.logger, nil, nil, ctx,
		WithBackoffFactor(queueSetting.BackoffFactor),
		WithMaxRetry(queueSetting.MaxRetry),
		WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		WithRetryDelay(queueSetting.RetryDelay),
		WithWorkerCount(queueSetting.WorkerNum),
		WithName("SlaveQueue"),
		WithMaxTaskExecution(queueSetting.MaxExecution),
	)
	m.slaveQueue.Store(newQueue)
	return nil
}

func (m *QueueManager) GetMediaMetaQueue() Queue {
	if v, ok := m.mediaMetaQueue.Load().(Queue); ok {
		return v
	}
	return nil
}

func (m *QueueManager) GetThumbQueue() Queue {
	if v, ok := m.thumbQueue.Load().(Queue); ok {
		return v
	}
	return nil
}
func (m *QueueManager) GetEntityRecycleQueue() Queue {
	if v, ok := m.entityRecycleQueue.Load().(Queue); ok {
		return v
	}
	return nil
}
func (m *QueueManager) GetIoIntenseQueue() Queue {
	if v, ok := m.ioIntenseQueue.Load().(Queue); ok {
		return v
	}
	return nil
}
func (m *QueueManager) GetRemoteDownloadQueue() Queue {
	if v, ok := m.remoteDownloadQueue.Load().(Queue); ok {
		return v
	}
	return nil
}

func (m *QueueManager) GetSlaveQueue() Queue {
	if v, ok := m.slaveQueue.Load().(Queue); ok {
		return v
	}
	return nil
}
