package queue

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

var (
	// ErrQueueShutdown the queue is released and closed.
	ErrQueueShutdown = errors.New("queue has been closed and released")
	// ErrMaxCapacity Maximum size limit reached
	ErrMaxCapacity = errors.New("golang-queue: maximum size limit reached")
	// ErrNoTaskInQueue there is nothing in the queue
	ErrNoTaskInQueue = errors.New("golang-queue: no Task in queue")
)

type (
	Scheduler interface {
		// Queue add a new Task into the queue
		Queue(task Task) error
		// Cancel removes tasks that have not been picked by a worker yet.
		Cancel(taskIDs ...int) int
		// Request get a new Task from the queue
		Request() (Task, error)
		// Shutdown stop all worker
		Shutdown() error
	}
	fifoScheduler struct {
		sync.Mutex
		taskQueue taskHeap
		capacity  int
		count     int
		exit      chan struct{}
		logger    *log.Helper
		stopOnce  sync.Once
		stopFlag  int32
	}
	taskHeap []Task
)

// Queue send Task to the buffer channel
func (s *fifoScheduler) Queue(task Task) error {
	if atomic.LoadInt32(&s.stopFlag) == 1 {
		return ErrQueueShutdown
	}

	s.Lock()
	removed := s.cancelLocked(task.ID())
	if s.capacity > 0 && s.count-removed >= s.capacity {
		s.Unlock()
		return ErrMaxCapacity
	}
	s.count -= removed
	s.taskQueue.Push(task)
	s.count++
	s.Unlock()

	return nil
}

func (s *fifoScheduler) Cancel(taskIDs ...int) int {
	if len(taskIDs) == 0 {
		return 0
	}
	s.Lock()
	defer s.Unlock()

	cancelled := s.cancelLocked(taskIDs...)
	s.count -= cancelled
	if s.count < 0 {
		s.count = 0
	}
	return cancelled
}

func (s *fifoScheduler) cancelLocked(taskIDs ...int) int {
	targets := make(map[int]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		targets[taskID] = struct{}{}
	}

	cancelled := 0
	kept := s.taskQueue[:0]
	for _, queuedTask := range s.taskQueue {
		if queuedTask == nil {
			kept = append(kept, queuedTask)
			continue
		}
		if _, ok := targets[queuedTask.ID()]; ok {
			cancelled++
			continue
		}
		kept = append(kept, queuedTask)
	}
	s.taskQueue = kept
	return cancelled
}

// Request a new Task from channel
func (s *fifoScheduler) Request() (Task, error) {
	if atomic.LoadInt32(&s.stopFlag) == 1 {
		return nil, ErrQueueShutdown
	}

	if s.count == 0 {
		return nil, ErrNoTaskInQueue
	}
	s.Lock()
	if s.taskQueue[s.taskQueue.Len()-1].ResumeTime() > time.Now().Unix() {
		s.Unlock()
		return nil, ErrNoTaskInQueue
	}

	data := s.taskQueue.Pop()
	s.count--
	s.Unlock()

	return data.(Task), nil
}

// Shutdown the worker
func (s *fifoScheduler) Shutdown() error {
	if !atomic.CompareAndSwapInt32(&s.stopFlag, 0, 1) {
		return ErrQueueShutdown
	}

	return nil
}

// NewFifoScheduler for create new Scheduler instance
func NewFifoScheduler(queueSize int, logger *log.Helper) Scheduler {
	w := &fifoScheduler{
		taskQueue: make([]Task, 2),
		capacity:  queueSize,
		logger:    logger,
	}

	return w
}

// Implement heap.Interface
func (h taskHeap) Len() int {
	return len(h)
}

func (h taskHeap) Less(i, j int) bool {
	return h[i].ResumeTime() < h[j].ResumeTime()
}

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(Task))
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
