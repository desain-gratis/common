package schedulerapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	mycontent_base "github.com/desain-gratis/common/delivery/mycontent-api/mycontent/base"
	"github.com/desain-gratis/common/delivery/scheduler-api/statemachine"
	entity "github.com/desain-gratis/common/types/entity/scheduler"
)

var (
	ErrTaskExecutionCancelled  = errors.New("task execution cancelled")
	ErrTaskExecutionReAssigned = errors.New("task execution reassigned")
)

type worker struct {
	id uint64

	namespace string

	taskDefinitionStoreRO *mycontent_base.Handler[*entity.RecurringTask]
	teStorageV2           statemachine.TaskExecutionUsecase
	// task IDs
	scheduled     map[queueTaskPair]*TaskExecutionRuntime
	scheduledLock *sync.Mutex

	queue *queueManager
	// client StateMachineClient

	timeBucket        map[timeQueue][]*TaskExecutionRuntime
	timeBucketLock    *sync.Mutex
	assignmentUpdated <-chan *entity.TaskExecution
}
type queueTaskPair struct {
	queueID string
	TaskID  string
}

// or background process I guess
// strictly read-only from the raft events from assignmentUpdated channel.
// started before raft is ran.
func newWorker(
	namespace string,
	taskDefinitionStoreRO *mycontent_base.Handler[*entity.RecurringTask],
	teStorageV2 statemachine.TaskExecutionUsecase,
	assignmentUpdated <-chan *entity.TaskExecution,
) *worker {
	return &worker{
		namespace:             namespace,
		taskDefinitionStoreRO: taskDefinitionStoreRO,
		teStorageV2:           teStorageV2,
		assignmentUpdated:     assignmentUpdated,
		scheduled:             make(map[queueTaskPair]*TaskExecutionRuntime),
		scheduledLock:         &sync.Mutex{},
		timeBucket:            make(map[timeQueue][]*TaskExecutionRuntime),
		timeBucketLock:        &sync.Mutex{},
		queue:                 nil,
		id:                    0,
	}
}

// func (w *worker) RegisterQueue(queue map[string]chan *TaskExecutionRuntime) {
// 	w.queue = queue
// }

func (w *worker) loadPendingTask(ctx context.Context, t0 time.Time) {
	// TODO: refactor
	// only load task that will be executed in the next 3 hour
	before := t0.Add(3 * time.Hour)
	assigned := entity.AssignmentStatusAssigned

	filter := statemachine.TaskExecutionFilter{
		Before:           &before,
		ExecutionStatus:  []entity.ExecutionStatus{entity.ExecutionStatusPending},
		AssignmentStatus: &assigned,
		AssignedNode:     []uint64{w.id},
	}

	taskCh, errCh := w.teStorageV2.GetAllWithFilter(ctx, filter, "deployd")
	for taskCh != nil || errCh != nil {
		select {
		case task, ok := <-taskCh:
			if !ok {
				taskCh = nil
				continue
			}

			// Process task.
			w.loadTaskExec(ctx, task, "backfill")

		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}

			if err != nil {
				log.Println("GetAll error:", err)
			}
		}
	}
}

// Start the worker)
func (w *worker) Start(ctx context.Context, id uint64) error {
	w.loadPendingTask(ctx, time.Now())

	w.id = id

	poll := time.NewTicker(30 * time.Second)
	go func() {
		defer poll.Stop()

		for {
			select {
			case <-ctx.Done():
			case t := <-poll.C:
				w.loadPendingTask(ctx, t)
			case taskExec := <-w.assignmentUpdated:
				w.checkCancellation(ctx, taskExec)
				w.loadTaskExec(ctx, taskExec, "live")
			}
		}
	}()

	return nil
}

// TODO: private and public
type TaskExecutionRuntime struct {
	*entity.TaskExecution
	ctx    context.Context
	cancel context.CancelCauseFunc
}

func (t *TaskExecutionRuntime) Cancel(err error) {
	if t == nil {
		return
	}
	t.cancel(err)
}

func (w *worker) cleanUp(taskExec *TaskExecutionRuntime) {
	taskExec.Cancel(fmt.Errorf("%w", ErrTaskExecutionCancelled))

	if taskExec == nil {
		return
	}

	if taskExec.TaskExecution == nil {
		return
	}

	func() {
		defer w.scheduledLock.Unlock()
		w.scheduledLock.Lock()
		delete(w.scheduled, queueTaskPair{taskExec.QueueID, taskExec.Id})
	}()

	func() {
		defer w.timeBucketLock.Unlock()
		w.timeBucketLock.Lock()

		tq := timeQueue{taskScheduledRunAt: taskExec.ScheduledRunAt, queueID: taskExec.QueueID}
		tasks := w.timeBucket[tq]
		for idx, tr := range tasks {
			if tr.QueueID == taskExec.QueueID && tr.Id == taskExec.Id {
				tasks = append(tasks[:idx], tasks[idx+1:]...)
				w.timeBucket[tq] = tasks
				break
			}
		}

		if len(tasks) == 0 {
			delete(w.timeBucket, tq)
		}
	}()

	// maybe cancel the timer as well
}

// for loaded task (in memory), we should check whether it's still relevant or not
func (w *worker) checkCancellation(_ context.Context, taskExec *entity.TaskExecution) {
	taskCoord := queueTaskPair{taskExec.QueueID, taskExec.Id}
	prev, ok := w.scheduled[taskCoord]
	if !ok {
		// no need to listen update, since it's already gone
		return
	}

	// task has been re-queued
	if taskExec.AssignmentState == nil {
		prev.Cancel(ErrTaskExecutionReAssigned)
		return
	}

	// task no longer ours :(
	if taskExec.AssignmentState.AssignedTo != w.id {
		prev.Cancel(ErrTaskExecutionReAssigned)
		return
	}

	// user cancels the task
	// only cancel if the task is still pending or in progress
	if taskExec.Status == entity.ExecutionStatusCancelled {
		switch prev.Status {
		case entity.ExecutionStatusInProgress, entity.ExecutionStatusPending:
			prev.Cancel(ErrTaskExecutionCancelled)
		}
		return
	}
}

// loadTaskExec materialize task execution into an in memory data structure, ready to be executed
func (w *worker) loadTaskExec(ctx context.Context, taskExec *entity.TaskExecution, _ string) {
	// if it's not pending
	if taskExec.Status != entity.ExecutionStatusPending {
		return
	}

	// if it's not assigned
	if taskExec.AssignmentState == nil {
		return
	}

	// if it's not our task
	if taskExec.AssignmentState.AssignedTo != w.id {
		return
	}

	if taskExec.ScheduledRunAt.After(time.Now().Add(3 * time.Hour)) {
		// to soon to load
		// use the polling mechanism later
		log.Printf("the task %v is past 3 hours from now (%v), I'll schedule it later.", taskExec.Id, taskExec.ScheduledRunAt)
		return
	}

	// if the queue no longer found
	var q chan *TaskExecutionRuntime
	var ok bool

	func() {
		if w.queue == nil {
			return
		}
		w.queue.queueByIDLock.Lock()
		defer w.queue.queueByIDLock.Unlock()
		q, ok = w.queue.queueByID[taskExec.QueueID]
	}()
	if !ok {
		// todo: queue validation should happens at creation
		// and additional handling..
		return
	}

	var isLoadedAlready bool
	func() {
		defer w.scheduledLock.Unlock()
		w.scheduledLock.Lock()
		_, isLoadedAlready = w.scheduled[queueTaskPair{taskExec.QueueID, taskExec.Id}]
	}()
	if isLoadedAlready {
		// we already hold the task in memory
		return
	}

	ctx, cancel := context.WithCancelCause(ctx)
	taskExecRuntime := &TaskExecutionRuntime{
		TaskExecution: taskExec, ctx: ctx, cancel: cancel,
	}

	// reference for future
	func() {
		defer w.scheduledLock.Unlock()
		w.scheduledLock.Lock()
		w.scheduled[queueTaskPair{taskExec.QueueID, taskExec.Id}] = taskExecRuntime
	}()

	tq := timeQueue{taskExec.ScheduledRunAt, taskExec.QueueID}
	var newTimeBucket bool

	switch taskExec.ExecutionMode {
	case entity.TaskExecutionModeImmediate:
		// TODO: maybe there is another better approach to queue immediate tasks
		go func() {
			select {
			case q <- taskExecRuntime:
			default:
				log.Printf("worker: queue starting to get full~")
			}
		}()
	case entity.TaskExecutionModeRecurring, entity.TaskExecutionModeScheduled:
		// initialize
		func() {
			defer w.timeBucketLock.Unlock()
			w.timeBucketLock.Lock()

			if _, ok := w.timeBucket[tq]; !ok {
				w.timeBucket[tq] = make([]*TaskExecutionRuntime, 0)
				newTimeBucket = true
			}
			w.timeBucket[tq] = append(w.timeBucket[tq], taskExecRuntime)
		}()

		// log.Printf("worker: got task execution assignment updated event (%v)", mode)

		if newTimeBucket {
			executeAt := time.Until(tq.taskScheduledRunAt)

			// TODO: choose what's best
			if executeAt < -10*time.Second {
				// in hope server already re-assign
				log.Println("worker: warn. stale task will not be executed")
				return
			} else if executeAt < 0 {
				// immediately executed
				executeAt = 0
			}

			time.AfterFunc(executeAt, func() {
				var taskExecs []*TaskExecutionRuntime
				func() {
					defer w.timeBucketLock.Unlock()
					w.timeBucketLock.Lock()
					taskExecs = w.timeBucket[tq]
				}()

				for _, taskExecRuntime := range taskExecs {
					if taskExecRuntime.ctx.Err() != nil {
						// we will no longer execute
						continue
					}

					select {
					case q <- taskExecRuntime:
					default:
						log.Printf("worker: queue starting to get full~")
					}
				}

				// todo: delete the bucket
			})
		}
	}
}

type timeQueue struct {
	taskScheduledRunAt time.Time
	queueID            string
}
