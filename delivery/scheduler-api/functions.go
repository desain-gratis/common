package schedulerapi

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/desain-gratis/common/delivery/scheduler-api/statemachine"
	entity "github.com/desain-gratis/common/types/entity/scheduler"
)

// restricted view
type taskExecutionRuntimeView struct {
	base *TaskExecutionRuntime
}

// Return read-only copy
func (u *taskExecutionRuntimeView) TaskDefinition() *entity.TaskExecution {
	return u.base.TaskExecution.Copy()
}

// Cancel mean user cancelled the task
func (u *taskExecutionRuntimeView) Cancel(reason string) {
	if u.base.CancelReason == nil {
		u.base.ExecutionState = &entity.ExecutionState{}
	}
	u.base.Status = entity.ExecutionStatusCancelled
	u.base.CancelReason = &reason
	u.base.cancel(fmt.Errorf("%w: %v", ErrTaskExecutionCancelled, reason)) // todo: proper context handling
}

// can do other allowed mutation as well

const teKey = "task-execs"

// TODO: maybe in their own module to be more user friendly
func GetTaskExecutionView(ctx context.Context) *taskExecutionRuntimeView {
	base := ctx.Value(teKey).(*TaskExecutionRuntime)
	return &taskExecutionRuntimeView{base}
}

func WithTaskExecution(ctx context.Context, rc *TaskExecutionRuntime) context.Context {
	return context.WithValue(ctx, teKey, rc)
}

// Wrap any "stateless" function with state reporting to raft
func taskFunctionHarness(client StateMachineClient, fn Executor) func(taskExec *TaskExecutionRuntime) {
	return func(taskExec *TaskExecutionRuntime) {
		runAt := time.Now().In(taskExec.UserTime.Location())
		taskExec.Status = entity.ExecutionStatusInProgress
		taskExec.ExecutionState = &entity.ExecutionState{
			ActualRunAt: &runAt,
		}

		// TODO: handle ctx more properly
		ctx := taskExec.ctx

		ctx = WithTaskExecution(ctx, taskExec)

		// update before
		_, err := client.Propose(ctx, statemachine.UpdateTaskExecution(taskExec.TaskExecution))
		if err != nil {
			log.Printf("executor: failed to update task execution to %v: %v (%v) %v\n", taskExec.Status, taskExec.Id, taskExec.QueueID, err)
			return
		}

		// TODO: limit mutation (later..)
		out, err := fn(ctx, taskExec.Input)
		if err != nil {
			taskExec.Status = entity.ExecutionStatusFailed
		} else {
			if taskExec.Status == "" {
				taskExec.Status = entity.ExecutionStatusSucceed
			}
		}

		tComplete := time.Now().In(taskExec.UserTime.Location())
		taskExec.ExecutionState.CompletedAt = &tComplete
		taskExec.ExecutionState.Output = out

		// update after
		// TODO: maybe make outside the raft.
		_, err = client.Propose(ctx, statemachine.UpdateTaskExecution(taskExec.TaskExecution))
		if err != nil {
			// failed to inform cluster back. Leader's timeout mechanism will kick in.
			log.Printf("executor: failed to update task execution to %v: %v (%v) %v\n", taskExec.Status, taskExec.Id, taskExec.QueueID, err)
			return
		}
	}

}
