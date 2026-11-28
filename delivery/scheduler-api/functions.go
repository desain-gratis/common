package schedulerapi

import (
	"context"
	"fmt"
	"log/slog"
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

const (
	teKey     = "task-execs"
	loggerKey = "log"
)

// TODO: maybe in their own module to be more user friendly
func GetTaskExecutionView(ctx context.Context) *taskExecutionRuntimeView {
	base := ctx.Value(teKey).(*TaskExecutionRuntime)
	return &taskExecutionRuntimeView{base}
}

func WithTaskExecution(ctx context.Context, rc *TaskExecutionRuntime) context.Context {
	return context.WithValue(ctx, teKey, rc)
}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func GetLogger(ctx context.Context) *slog.Logger {
	logger, _ := ctx.Value(loggerKey).(*slog.Logger)
	return logger
}

// Wrap any "stateless" function with state reporting to raft
func taskFunctionHarness(ctx context.Context, client StateMachineClient, fn Executor, log *slog.Logger) func(taskExec *TaskExecutionRuntime) {
	return func(taskExec *TaskExecutionRuntime) {
		log := log.With(
			slog.String("task_id", taskExec.Id),
			slog.String("queue_id", taskExec.QueueID),
			slog.Uint64("executor_id", taskExec.AssignmentState.AssignedTo),
		)
		if taskExec.ParentId != nil {
			log = log.With(slog.String("task_parent_id", *taskExec.ParentId))
		}

		runAt := time.Now().In(taskExec.UserTime.Location())
		taskExec.Status = entity.ExecutionStatusInProgress
		taskExec.ExecutionState = &entity.ExecutionState{
			ActualRunAt: &runAt,
		}

		// update before
		_, err := client.Propose(ctx, statemachine.UpdateTaskExecution(taskExec.TaskExecution))
		if err != nil {
			log.Error("executor: failed to update task execution", slog.String("err", err.Error()))
			return
		}

		// TODO: limit mutation (later..)

		// task context
		tctx := taskExec.ctx
		tctx = WithTaskExecution(tctx, taskExec)
		tctx = WithLogger(tctx, log)
		out, err := fn(tctx, taskExec.Input)
		if err != nil {
			taskExec.Status = entity.ExecutionStatusFailed
		} else {
			switch taskExec.Status {
			case entity.ExecutionStatusCancelled, entity.ExecutionStatusInterrupted:
			default:
				taskExec.Status = entity.ExecutionStatusSucceed
			}
		}

		// in case of top-down error (from raft), worker does not need to send anything back.
		// we don't use select statement here because fn should be responsible in managing handling the cancellation event
		tErr := tctx.Err()
		if tErr != nil {
			return
		}

		tComplete := time.Now().In(taskExec.UserTime.Location())
		taskExec.ExecutionState.CompletedAt = &tComplete
		taskExec.ExecutionState.Output = out

		// update after
		// TODO: maybe make outside the raft.
		_, err = client.Propose(ctx, statemachine.UpdateTaskExecution(taskExec.TaskExecution))
		if err != nil {
			// failed to inform cluster back. Leader's timeout mechanism will kick in.
			log.Error("executor: failed to update task execution", slog.String("err", err.Error()))
			return
		}
	}

}
