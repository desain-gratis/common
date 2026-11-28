package schedulerapi

import (
	"context"
	"database/sql"
	"strconv"
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

	// List of worker / node
	WorkerUsecase mycontent.Usecase[*entity.Worker]

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

	config *Config
}

type Config struct {
	Host string `json:"host"`

	// Log URL to allow access to the log
	LogUrl string `json:"log_url"`

	// Log URL to allow access to the log tail / live tail
	LogTailUrl string `json:"log_tail_url"`

	// Other config such as region.. etc..
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
func NewReplicatedScheduler(ctx context.Context, namespace string, conn *sql.DB, config *Config) (*replicatedScheduler, error) {
	// worker registry
	workerStorage, err := content_chdb.New(ctx, conn, statemachine.TableWorker, 0)
	if err != nil {
		return nil, err
	}

	// base recurring task storage
	recurringTaskStorage, err := content_chdb.New(ctx, conn, statemachine.TableRecurringTask, 0)
	if err != nil {
		return nil, err
	}

	// for easy access in the code
	workerStorageUsecase := mycontent_base.New[*entity.Worker](workerStorage)
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
		workerStorageUsecase,
		recurringTaskStorageUsecase,
		taskExecStorage,
		executorCh,
	)

	topic := notifier_impl.NewStandardTopic()

	// the replicated state machine
	// any storage mutations happened here
	storageStateMachine := statemachine.NewRaftApp(
		namespace,
		topic,
		workerStorageUsecase,
		recurringTaskStorageUsecase,
		taskExecStorage,
		executorCh,
	)

	// todo: make storage read only for user
	return &replicatedScheduler{
		namespace:            namespace,
		Worker:               worker,
		WorkerUsecase:        workerStorageUsecase,
		RecurringTaskUsecase: recurringTaskStorageUsecase,
		TaskExecutionUsecase: taskExecStorage.DGCompat(),
		taskExecutionUsecase: taskExecStorage,
		StateMachine:         storageStateMachine,
		StateMachineEvent:    topic,
		config:               config,
	}, nil
}

func (w *replicatedScheduler) StartWorker(ctx context.Context, raftWrapper *runneretcd.RaftWrapper) error {
	// Load all local task in to memory and start the worker goroutine

	// Can also put the client inside worker; but now it's more clear like this first
	client := statemachine.NewClient(raftWrapper)
	err := w.Worker.Start(ctx, client.ID())
	if err != nil {
		return err
	}
	return nil
}

func (w *replicatedScheduler) UpdateAvailability(ctx context.Context, raftWrapper *runneretcd.RaftWrapper) error {
	client := statemachine.NewClient(raftWrapper)
	wdata := &entity.Worker{
		Ns:          w.namespace,
		Id:          strconv.FormatUint(client.ID(), 10),
		IdInt:       client.ID(),
		Queue:       w.Worker.queue.GetAll(),
		PublishedAt: time.Now(),
	}
	if w.config != nil {
		wdata.LogUrl = w.config.LogUrl
		wdata.Host = w.config.Host
	}

	// Also tells that this worker is ready to receive task
	_, err := client.Propose(ctx, statemachine.UpdateAvailability(wdata))
	if err != nil {
		return err
	}

	return nil
}

// HttpHandler pre-made HTTPHandler to interact with the scheduler
func (w *replicatedScheduler) HttpHandler(raftWrapper *runneretcd.RaftWrapper) *httpAPI {
	client := statemachine.NewClient(raftWrapper)
	return &httpAPI{raftClient: client, tes: w.taskExecutionUsecase, worker: w.Worker}
}

// todo: provide good default
func (w *replicatedScheduler) Queue(raftWrapper *runneretcd.RaftWrapper, logDir string) *queueManager {
	client := statemachine.NewClient(raftWrapper)
	w.Worker.queue = &queueManager{
		raftClient:    client,
		queueByID:     make(map[string]chan *TaskExecutionRuntime),
		queueByIDLock: &sync.RWMutex{},
		logDir:        logDir,
		queueData:     make(map[string]*queueState),
		queueDataLock: &sync.RWMutex{},
	}

	return w.Worker.queue
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

		workers:         make(map[uint64]*workerStatus),
		preferredWorker: 0,
	}

	go func(term, leaderID uint64) {
		defer ticker.Stop()
		ls.Handler(ctx, term, leaderID)
	}(term, leaderID)

	return nil
}
