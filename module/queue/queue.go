package queue

import (
	"context"
	"errors"
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
		// CancelTasks cancels the tasks with the given IDs.
		CancelTasks(ctx context.Context, taskIDs ...int)
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
		// cancelledIDs stores the IDs of the tasks that should be canceled.
		cancelledIDs map[int]struct{}
		// runningCancels stores the IDs and cancelFuncs of the tasks that are running.
		runningCancels map[int]context.CancelFunc

		// Dependencies
		logger     *log.Helper
		taskClient TaskClient
		registry   TaskRegistry

		// Options
		*options
	}
)

var (
	CriticalErr = errors.New("non-retryable error")
)

func New(l *log.Helper, taskClient TaskClient, registry TaskRegistry, ctx context.Context, opts ...Option) Queue {
	o := newDefaultOptions()
	for _, opt := range opts {
		opt.apply(o)
	}

	ctx, cancel := context.WithCancel(ctx)

	return &queue{
		routineGroup:   newRoutineGroup(),
		scheduler:      NewFifoScheduler(0, l),
		quit:           make(chan struct{}),
		ready:          make(chan struct{}, 1),
		metric:         &metric{},
		options:        o,
		logger:         l,
		registry:       registry,
		taskClient:     taskClient,
		rootCtx:        ctx,
		cancel:         cancel,
		cancelledIDs:   map[int]struct{}{},
		runningCancels: map[int]context.CancelFunc{},
	}
}

// Start to enable all worker
func (q *queue) Start() {
	q.routineGroup.Run(func() {
		// Resume tasks in DB
		if len(q.options.resumeTaskType) > 0 && q.taskClient != nil {

			ctx := context.TODO()
			tasks, err := q.taskClient.GetPendingTasks(ctx, q.resumeTaskType...)
			if err != nil {
				q.logger.Warnf("Failed to get pending tasks from DB for given type %v: %s", q.resumeTaskType, err)
			}

			resumed := 0
			for _, t := range tasks {
				resumedTask, err := NewTaskFromModel(t)
				if err != nil {
					q.logger.Warnf("Failed to resume task %d: %s", t.ID(), err)
					continue
				}

				if resumedTask.Status() == StatusSuspending {
					q.metric.IncSuspendingTask()
					q.metric.IncSubmittedTask()
				}

				if err := q.QueueTask(ctx, resumedTask); err != nil {
					q.logger.Warnf("Failed to resume task %d: %s", t.ID(), err)
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

// SuccessTasks returns the numbers of success tasks.
func (q *queue) SuccessTasks() int32 {
	return int32(q.metric.SuccessTasks())
}

// FailureTasks returns the numbers of failure tasks.
func (q *queue) FailureTasks() int32 {
	return int32(q.metric.FailureTasks())
}

// SubmittedTasks returns the numbers of submitted tasks.
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

	if t.Status() != StatusSuspending {
		q.metric.IncSubmittedTask()
		if err := q.transitStatus(ctx, t, StatusQueued); err != nil {
			return err
		}
	} else {
		q.consumeCancelled(t.ID())
	}

	if err := q.scheduler.Queue(t); err != nil {
		return err
	}
	owner := t.OwnerID()
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
	//ctx := trans.WithValue(q.rootCtx, t.OwnerID())
	//ctx = context.WithValue(ctx, filemanager.ManagerDepCtx{}, q.managerDep)
	//ctx = context.WithValue(ctx, filemanager.DbfsDepCtx{}, q.dbfsDep)
	//ctx = context.WithValue(ctx, filemanager.NodePoolCtx{}, q.nodePool)
	return q.rootCtx
}

func (q *queue) work(t Task) {
	ctx := q.newContext(t)
	timeIterationStart := time.Now()
	running := false
	var err error
	// to handle panic cases from inside the worker
	// in such case, we start a new goroutine
	defer func() {
		q.metric.DecBusyWorker()
		e := recover()
		if e != nil {
			q.logger.WithContext(ctx).Errorf("Panic error in queue %q: %v", q.name, e)
			t.OnError(fmt.Errorf("panic error: %v", e), time.Since(timeIterationStart))

			_ = q.transitStatus(ctx, t, StatusError)
		}
		if running {
			q.Lock()
			delete(q.runningCancels, t.ID())
			q.Unlock()
		}
		q.schedule()
	}()

	q.Lock()
	_, preMarked := q.cancelledIDs[t.ID()]
	if preMarked {
		// 任务在入队后、执行前被取消
		delete(q.cancelledIDs, t.ID())
		q.Unlock()

		// 明确设置 status 为 Canceled，让状态机走 Cleanup 路径
		if err := q.transitStatus(ctx, t, StatusCanceled); err != nil {
			q.logger.Warnf("Failed to cancel task %d: %s", t.ID(), err)
		}
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	q.runningCancels[t.ID()] = cancel
	running = true
	q.Unlock()

	err = q.transitStatus(ctx, t, StatusProcessing)
	if err != nil {
		q.logger.WithContext(ctx).Error("failed to transit task %d to processing: %s", t.ID(), err.Error())
		panic(err)
	}

	for {
		timeIterationStart = time.Now()
		var next TaskStatus
		next, err = q.run(ctx, t)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				q.consumeCancelled(t.ID())
				_ = q.transitStatus(ctx, t, StatusCanceled)
				return
			}
			t.OnError(err, time.Since(timeIterationStart))
			q.logger.WithContext(ctx).Error("runtime error in queue %q: %s", q.name, err.Error())

			_ = q.transitStatus(ctx, t, StatusError)
			break
		}

		// iteration completes
		if q.consumeCancelled(t.ID()) {
			_ = q.transitStatus(ctx, t, StatusCanceled)
			return
		}
		t.OnIterationComplete(time.Since(timeIterationStart))
		_ = q.transitStatus(ctx, t, next)
		if next != StatusProcessing {
			break
		}
	}
}

func (q *queue) run(ctx context.Context, t Task) (TaskStatus, error) {
	// create channel with buffer size 1 to avoid goroutine leak
	done := make(chan struct {
		err  error
		next TaskStatus
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
			next = StatusSuspending
		}

		done <- struct {
			err  error
			next TaskStatus
		}{err: err, next: next}
	}()

	select {
	case p := <-panicChan:
		panic(p)
	case <-ctx.Done(): // timeout reached
		return StatusError, ctx.Err()
	case <-q.quit: // shutdown service
		// cancel job
		cancel()

		leftTime := q.maxTaskExecution - t.Executed() - time.Since(startTime)
		// wait job
		select {
		case <-time.After(leftTime):
			return StatusError, context.DeadlineExceeded
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
func (q *queue) transitStatus(ctx context.Context, t Task, to TaskStatus) (err error) {
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

func (q *queue) CancelTasks(ctx context.Context, taskIDs ...int) {
	if len(taskIDs) == 0 {
		return
	}
	q.scheduler.Cancel(taskIDs...)
	for _, taskID := range taskIDs {
		q.Lock()
		q.cancelledIDs[taskID] = struct{}{}
		cancel, running := q.runningCancels[taskID]
		if !running {
			q.Unlock()
		} else {
			q.Unlock()
			cancel()
		}
	}
}

func (q *queue) consumeCancelled(taskID int) bool {
	q.Lock()
	defer q.Unlock()
	if _, ok := q.cancelledIDs[taskID]; !ok {
		return false
	}
	delete(q.cancelledIDs, taskID)
	return true
}
