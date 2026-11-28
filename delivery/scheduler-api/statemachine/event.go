package statemachine

import (
	"time"

	entity "github.com/desain-gratis/common/types/entity/scheduler"
)

type Event string

type EventRecurringTaskCreated *entity.RecurringTask
type EventRecurringTaskUpdated *entity.RecurringTask
type EventRecurringTaskGenerated *entity.RecurringTask
type EventWorkerRequestedWork *entity.Worker

type EventTaskExecutionCreated *entity.TaskExecution

type EventTaskExecutionAssignmentUpdated struct{}

type EventWorkCreatedRecurringTask struct {
	ScheduledRunAt time.Time
	Queue          string
	Tasks          []*entity.TaskExecution
}
