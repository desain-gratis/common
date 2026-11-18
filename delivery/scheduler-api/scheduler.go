package schedulerapi

import (
	"context"
	"database/sql"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	mycontent_base "github.com/desain-gratis/common/delivery/mycontent-api/mycontent/base"
	content_chdb "github.com/desain-gratis/common/delivery/mycontent-api/storage/content/chdb"
	"github.com/desain-gratis/common/delivery/scheduler-api/storage"
	"github.com/desain-gratis/common/lib/notifier"
	notifier_impl "github.com/desain-gratis/common/lib/notifier/impl"
	entity "github.com/desain-gratis/common/types/entity/scheduler"
)

type StateMachineClient interface {
	ID() uint64
	Propose(ctx context.Context, request any) (any, error)
	Subscribe(ctx context.Context) (notifier.Subscription, error)
}

type wrapp struct {
	namespace string
	// Recurring task definition store
	RecurringTaskUsecase mycontent.Usecase[*entity.RecurringTask]

	// Task execution store
	TaskExecutionUsecase storage.TaskExecutionUsecase

	// Integrations
	Http   *httpAPI
	Worker *worker
	Raft   *storage.RaftStorage

	Topic notifier.Topic

	taskExec chan *entity.TaskExecution

	// TODO: make common
	Client StateMachineClient
}

func New(ctx context.Context, conn *sql.DB, replicated bool) (*wrapp, error) {
	namespace := "common"
	// base recurring task storage
	recurringTaskStorage, err := content_chdb.New(ctx, conn, storage.TableRecurringTask, 0)
	if err != nil {
		return nil, err
	}

	// wrapper for easy access
	recurringTaskStorageUsecase := mycontent_base.New[*entity.RecurringTask](recurringTaskStorage)

	// custom task execution storage
	// and execute initialize
	taskExecStorage := storage.NewClickhouseTaskExecutionStorage(conn)
	err = taskExecStorage.Init(ctx)
	if err != nil {
		return nil, err
	}

	// topic for events related to scheduler
	topic := notifier_impl.NewStandardTopic()

	executorCh := make(chan *entity.TaskExecution)

	stub := &stubSMClient{}

	worker := NewWorker(
		namespace,
		topic,
		stub,
		recurringTaskStorageUsecase,
		taskExecStorage,
		executorCh,
	)

	httpAPI := NewHttpAPI(topic, stub, taskExecStorage)

	raftStorage := storage.NewRaftApp(
		namespace,
		topic,
		recurringTaskStorageUsecase,
		taskExecStorage,
		executorCh,
	)

	return &wrapp{
		namespace: namespace,

		Http:   httpAPI,
		Worker: worker,
		Topic:  topic,

		Raft: raftStorage,

		Client: stub,
	}, nil
}

func (w *wrapp) StartWorker(ctx context.Context, client StateMachineClient) error {
	// TODO: refactor
	w.Client = client
	w.Http.rc = client
	w.Worker.client = client

	// Load all local task in to memory and start the worker goroutine
	err := w.Worker.Init(ctx)
	if err != nil {
		return err
	}

	return nil
}
