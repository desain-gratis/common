package schedulerapi

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	mycontent_base "github.com/desain-gratis/common/delivery/mycontent-api/mycontent/base"
	content_chdb "github.com/desain-gratis/common/delivery/mycontent-api/storage/content/chdb"
	"github.com/desain-gratis/common/delivery/scheduler-api/statemachine"
	"github.com/desain-gratis/common/lib/notifier"
	notifier_impl "github.com/desain-gratis/common/lib/notifier/impl"
	runneretcd "github.com/desain-gratis/common/lib/raft/runner-etcd"
	entity "github.com/desain-gratis/common/types/entity/scheduler"
)

type StateMachineClient interface {
	ID() uint64
	Propose(ctx context.Context, request any) (any, error)
	Subscribe(ctx context.Context) (notifier.Subscription, error)
}

type replicatedScheduler struct {
	namespace string
	// Recurring task definition store
	RecurringTaskUsecase mycontent.Usecase[*entity.RecurringTask]

	// Task execution store
	TaskExecutionUsecase mycontent.Usecase[*entity.TaskExecution]
	taskExecutionUsecase statemachine.TaskExecutionUsecase // base

	// Integrations
	StateMachine      *statemachine.RaftStorage
	StateMachineEvent notifier.Topic

	// Worker handles node specific job
	Worker *worker
}

// NewReplicatedScheduler creates new replicated scheduler that run on top of etcd-io/raft
//
// scheduler := NewReplicatedScheduler(...)
// raftClient, _ ... := runneretcd.InitWithConfig(scheduler.Raft ...)
//
// scheduler.StartWorker(ctx, raftClient)
//
// raftClient.Start(ctx, scheduler.OnBecomingLeader)
//
// handler := scheduler.HttpHandler(raftClient)
//
// ...
func NewReplicatedScheduler(ctx context.Context, namespace string, conn *sql.DB) (*replicatedScheduler, error) {

	// base recurring task storage
	recurringTaskStorage, err := content_chdb.New(ctx, conn, statemachine.TableRecurringTask, 0)
	if err != nil {
		return nil, err
	}

	// replicatedSchedulerer for easy access
	recurringTaskStorageUsecase := mycontent_base.New[*entity.RecurringTask](recurringTaskStorage)

	// custom task execution storage
	// and execute initialize
	taskExecStorage := statemachine.NewClickhouseTaskExecutionStorage(conn)
	err = taskExecStorage.Init(ctx)
	if err != nil {
		return nil, err
	}

	executorCh := make(chan *entity.TaskExecution)

	worker := newWorker(
		namespace,
		recurringTaskStorageUsecase,
		taskExecStorage,
		executorCh,
	)

	topic := notifier_impl.NewStandardTopic()
	storageStateMachine := statemachine.NewRaftApp(
		namespace,
		topic,
		recurringTaskStorageUsecase,
		taskExecStorage,
		executorCh,
	)

	return &replicatedScheduler{
		namespace:            namespace,
		Worker:               worker,
		RecurringTaskUsecase: recurringTaskStorageUsecase,
		TaskExecutionUsecase: taskExecStorage.DGCompat(),
		taskExecutionUsecase: taskExecStorage,
		StateMachine:         storageStateMachine,
		StateMachineEvent:    topic,
	}, nil
}

func (w *replicatedScheduler) StartWorker(ctx context.Context, raftClient *runneretcd.RaftWrapper) error {
	// Load all local task in to memory and start the worker goroutine

	// Can also put the client inside worker; but now it's more clear like this first
	id := statemachine.NewClient(raftClient).ID()
	err := w.Worker.Start(ctx, id)
	if err != nil {
		return err
	}

	return nil
}

// HttpHandler pre-made HTTPHandler to interact with the scheduler
func (w *replicatedScheduler) HttpHandler(raftWrapper *runneretcd.RaftWrapper) *httpAPI {
	client := statemachine.NewClient(raftWrapper)
	return &httpAPI{raftClient: client, tes: w.taskExecutionUsecase}
}

func (w *replicatedScheduler) Queue(raftWrapper *runneretcd.RaftWrapper) *queueManager {
	client := statemachine.NewClient(raftWrapper)
	return &queueManager{raftClient: client, queueByID: make(map[string]chan *TaskExecutionRuntime), queueByIDLock: &sync.RWMutex{}}
}

// Called everytime becoming raft leader
// Context will be cancelled when stepping down from raft leader
func (w *replicatedScheduler) OnBecomingRaftLeader(ctx context.Context, term uint64, leaderID uint64) error {
	ticker := time.NewTicker(5 * time.Second) // todo: maybe separate health tick & task execution tick

	timers := make(map[string]*time.Timer)
	timersLock := &sync.RWMutex{}

	// Guarantees that we can get the latest state, and all incoming message after that

	// Get client from context, instead from init value somewhere
	rc := runneretcd.GetRaftWrapper(ctx)
	client := statemachine.NewClient(rc)

	subscription, err := client.Subscribe(ctx)
	if err != nil {
		return err
	}

	ls := leader{
		worker:     w.Worker,
		client:     client,
		timersLock: timersLock,
		timers:     timers,
		id:         leaderID,
		term:       term,
		raftNode:   rc.Node(),

		// keep track of node health
		node:                  make(map[uint64]*entity.Node),
		healthyDurationWithin: 10 * time.Second,

		ticker:       ticker,
		subscription: subscription,

		healthyUIDs: make(map[uint64]struct{}),
		healthyUID:  0,
	}

	go func(term, leaderID uint64) {
		defer ticker.Stop()
		ls.Handler(ctx, term, leaderID)
	}(term, leaderID)

	return nil
}
