package schedulerapi

import (
	"context"
	"sync"
)

type queueManager struct {
	raftClient StateMachineClient

	queueByID     map[string]chan *TaskExecutionRuntime
	queueByIDLock *sync.RWMutex
}

// Add let's user specify how the worker will execute the task
func (q *queueManager) Add(ctx context.Context, id string, queue chan<- *taskExecutionRuntimeView) *queueManager {
	queue2 := make(chan *TaskExecutionRuntime)
	go func() {
		defer close(queue2)
		for task := range queue2 {
			t := &taskExecutionRuntimeView{base: task}
			queue <- t
		}
	}()

	func() {
		q.queueByIDLock.Lock()
		defer q.queueByIDLock.Unlock()
		q.queueByID[id] = queue2
	}()

	return q
}

type Executor func(ctx context.Context, input any) (any, error)

// AddFunction is a convenience function
func (q *queueManager) AddFunction(ctx context.Context, id string, ngoroutine int, fn Executor) *queueManager {
	if ngoroutine <= 0 {
		ngoroutine = 1
	}

	queue := make(chan *TaskExecutionRuntime)

	fnWithHarness := taskFunctionHarness(q.raftClient, fn)

	for range ngoroutine {
		go func() {
			defer close(queue)

			for task := range queue {
				// todo: more proper context handlling & app shutdown handling
				if ctx.Err() != nil {
					break
				}

				fnWithHarness(task)
			}
		}()
	}

	func() {
		q.queueByIDLock.Lock()
		defer q.queueByIDLock.Unlock()
		q.queueByID[id] = queue
	}()

	return q
}
