package schedulerapi

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/desain-gratis/common/delivery/scheduler-api/statemachine"
	"github.com/desain-gratis/common/lib/notifier"
	entity "github.com/desain-gratis/common/types/entity/scheduler"
	"go.etcd.io/raft/v3"
)

type leader struct {
	*worker

	client     StateMachineClient
	timersLock *sync.RWMutex
	timers     map[string]*time.Timer
	id         uint64
	term       uint64

	node   map[uint64]*entity.Node
	ticker *time.Ticker

	// config
	healthyDurationWithin time.Duration

	raftNode     raft.Node
	subscription notifier.Subscription

	workers map[uint64]*workerStatus

	// preferredWorker to assign task to, taken from list of healthy worker
	preferredWorker uint64
}

type workerStatus struct {
	uid        uint64
	notHealthy bool
	lastSeen   time.Time
}

// utility to retrieve task from task-ish alias
func getTask(taskish any) *entity.RecurringTask {
	switch task := taskish.(type) {
	case statemachine.EventRecurringTaskCreated:
		return (*entity.RecurringTask)(task)
	case statemachine.EventRecurringTaskUpdated:
		return (*entity.RecurringTask)(task)
	case statemachine.EventRecurringTaskGenerated:
		return (*entity.RecurringTask)(task)
	}

	return nil
}

// Main loop for leader
func (ls *leader) Handler(ctx context.Context, term, leaderID uint64) {
	_ = ls.periodicCheck(ctx, time.Now())

	// todo: refactor
	t := time.Now()
	// todo pt in function
	allRecurringTasks, err := ls.taskDefinitionStoreRO.Get(ctx, ls.namespace, nil, "")
	if err != nil {
		return
	}
	for _, task := range allRecurringTasks {
		nextGenerate := t // immediate for new task
		if task.State != nil {
			nextGenerate = task.State.LastGenerateEnd
		}

		time.AfterFunc(time.Until(nextGenerate)/2, func() {
			err := ls.triggerGenerate(ctx, task, t)
			if err != nil {
				log.Printf("leader: err creating scheduled task execution instances %v", err)
			}
		})
	}

	for {
		select {
		case t := <-ls.ticker.C:
			// check tasks that has empty assignment / unhealthy node
			err := ls.periodicCheck(ctx, t)
			if err != nil {
				log.Printf("leader: error during periodic check: %v", err)
			}

		case msg := <-ls.subscription.Listen():
			t := time.Now()

			switch evt := msg.(type) {
			case statemachine.EventRecurringTaskCreated:
				task := getTask(evt)
				if task == nil {
					// TODO: error inside the raft app, can check which causes empty event
					log.Printf("leader: lah task nya nil")
					continue
				}
				log.Printf("leader: a new recurring task has come. Triggering generate %v", task.Id)
				err = ls.triggerGenerate(ctx, task, t)
				if err != nil {
					log.Printf("leader: failed to trigger generate %v", err)
				}
			case statemachine.EventRecurringTaskGenerated:
				task := getTask(evt)
				if task == nil {
					// TODO: error inside the raft app, can check which causes empty event
					log.Printf("leader: lah task nya nil")
					continue
				}

				nextGenerate := t // immediate for new task
				if task.State != nil {
					start := task.State.LastGenerateStart
					end := task.State.LastGenerateEnd

					nextGenerate = start.Add(end.Sub(start) / 2)
				}

				log.Printf("leader: recurring task %v has been generated, scheduling next trigger at %v", task.Id, nextGenerate)

				time.AfterFunc(time.Until(nextGenerate), func() {
					t := time.Now()
					log.Printf("leader: duar! %v", t)

					err := ls.triggerGenerate(ctx, task, t)
					if err != nil {
						log.Printf("leader: err creating scheduled task execution instances %v", err)
					}
				})
			case statemachine.EventTaskExecutionCreated:
				taskExec := (*entity.TaskExecution)(evt)
				if ls.preferredWorker == 0 {
					// should be fatal, since it's should be assigned during the first periodicCheck()
					log.Printf("leader: no healthy UID during task assignment")
					continue
				}
				ls.assignPendingTask(ctx, ls.preferredWorker, taskExec, t)
			default:
				// log.Printf("Unknown job evt")
			}
		case <-ctx.Done():
			return
		}
	}
}

func (ls *leader) triggerGenerate(ctx context.Context, task *entity.RecurringTask, t time.Time) error {
	_, err := ls.client.Propose(ctx, statemachine.GenerateRecurringTaskExecution{
		Task:       task,
		ServerTime: t,
	})
	if err != nil {
		return err
	}

	return nil
}

// check for unhealthy and if found, re-assign task execution, notify worker
func (ls *leader) periodicCheck(ctx context.Context, t0 time.Time) error {
	healthyCount := 0

	// only available in leader
	for nodeID, progress := range ls.raftNode.Status().Progress {
		if _, ok := ls.workers[nodeID]; !ok {
			ls.workers[nodeID] = &workerStatus{}
		}

		ls.workers[nodeID].uid = nodeID

		if progress.RecentActive {
			ls.workers[nodeID].notHealthy = false
			ls.workers[nodeID].lastSeen = t0
			healthyCount++
		} else {
			ls.workers[nodeID].notHealthy = true
		}
	}

	if healthyCount == 0 {
		return fmt.Errorf("leader: no healthy node")
	}

	// assign preferred worker from healthy node
	// sticky: only change the preferred worker at initialization & if we detected that the (currrent) preffered worker is not healthy
	//
	// todo: task can have node/worker preferences in which they may execute in
	preffered, ok := ls.workers[ls.preferredWorker]
	if ls.preferredWorker == 0 || (ok && preffered.notHealthy) {
		for workerUID, aWorker := range ls.workers {
			if !aWorker.notHealthy {
				ls.preferredWorker = workerUID
				break
			}
		}
	}

	_ = ls.assignPendingTasks(ctx, ls.preferredWorker, t0)
	_ = ls.reassignTasksFromUnhealthyNode(ctx, ls.preferredWorker, t0)

	return nil
}

// Assign pending tasks to a worker.
// Move pending task to another worker if detected unhealthy.
func (ls *leader) assignPendingTasks(ctx context.Context, anyHealthyUID uint64, t0 time.Time) error {
	// This is totally separate with worker code; we can use different value
	// todo: replace the hardcoded value
	before := t0.Add(materializeExecutionPeriod)
	notAssigned := entity.AssignmentStatusPending
	filter := statemachine.TaskExecutionFilter{
		Before:           &before,
		ExecutionStatus:  []entity.ExecutionStatus{entity.ExecutionStatusPending},
		AssignmentStatus: &notAssigned,
	}

	taskCh, errCh := ls.teStorageV2.GetAllWithFilter(ctx, filter, ls.namespace)
	for taskCh != nil || errCh != nil {
		select {
		case task, ok := <-taskCh:
			if !ok {
				taskCh = nil
				continue
			}
			ls.assignPendingTask(ctx, anyHealthyUID, task, t0)
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

	return nil
}

func (ls *leader) reassignTasksFromUnhealthyNode(ctx context.Context, prefferedUID uint64, t0 time.Time) error {
	unhealthyUIDs := make([]uint64, 0)
	if len(unhealthyUIDs) == 0 {
		// no error
		return nil
	}

	// This is totally separate with worker code. we can use different value
	// todo: replace the hardcoded value
	before := t0.Add(materializeExecutionPeriod)
	assigned := entity.AssignmentStatusAssigned
	filter := statemachine.TaskExecutionFilter{
		Before:           &before,
		ExecutionStatus:  []entity.ExecutionStatus{entity.ExecutionStatusPending}, // Maybe check also the one in progress
		AssignmentStatus: &assigned,
		AssignedNode:     unhealthyUIDs,
	}

	taskCh, errCh := ls.teStorageV2.GetAllWithFilter(ctx, filter, ls.namespace)
	for taskCh != nil || errCh != nil {
		select {
		case task, ok := <-taskCh:
			if !ok {
				taskCh = nil
				continue
			}

			if task.AssignmentState == nil {
				// should not happen
				continue
			}

			assignee := ls.workers[task.AssignmentState.AssignedTo]
			if !assignee.notHealthy {
				// healthy, no need reassign
				continue
			}

			if task.ScheduledRunAt.Before(assignee.lastSeen.Add(gracePeriod)) {
				// grace period. we hope (optimistic) that the asignee worker will be back within 10 minute
				// and execute the said task
				continue
			}

			ls.assignPendingTask(ctx, prefferedUID, task, t0)
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

	return nil
}

// assignPendingTask
// it can be called immediately during live request, or polled
func (ls *leader) assignPendingTask(ctx context.Context, anyHealthyUID uint64, taskExec *entity.TaskExecution, t time.Time) {
	// safeguard
	// todo: maybe responsibility of the caller
	if taskExec.Status != entity.ExecutionStatusPending {
		return
	}

	// if it's not immediate, we can assign to worker later
	if taskExec.ExecutionMode != entity.TaskExecutionModeImmediate && taskExec.ScheduledRunAt.After(t.Add(3*time.Hour)) {
		return
	}

	// in case of re-assignment, it's possible taskExec already have previous assignment
	if taskExec.AssignmentState == nil {
		taskExec.AssignmentState = &entity.AssignmentState{}
	}

	taskExec.AssignmentState.AssignedTo = anyHealthyUID
	taskExec.AssignmentState.AssignedBy = ls.client.ID()
	taskExec.AssignmentState.AssignmentTime = t.In(taskExec.UserTime.Location())
	taskExec.AssignmentState.AssignmentCount++
	taskExec.AssignmentStatus = entity.AssignmentStatusAssigned

	result, err := ls.client.Propose(ctx, statemachine.UpdateTaskExecution(taskExec))
	if err != nil {
		// determien error, maybe panic or maybe just fail
		log.Printf("leader: failed to (re)assign work: %v", err)
	}

	_ = result
}
