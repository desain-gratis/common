package writer

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	notifierapi "github.com/desain-gratis/common/example/message-broker/src/log-api"
)

// TableWriterStateMachine
// TWSM
// or just, TableWriter

// EventWriterSM is a state machine that can writes and publishes event.
type ewsm struct {
	purchaseCounter uint64
}

type input struct {
	ID    uint64
	Type  string
	Value json.RawMessage
}

func (t *ewsm) Load(state []byte) {

}

func (t *ewsm) Save() []byte {
	return nil
}

func parseMsg(data []byte) (*input, error) {
	var msg input
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// an EWSM can have multiple tables for different purpose.
// eg. one for append only log, other for aggregated by key, store temporary calculation,
// cache "active" data, etc.
func (t *ewsm) GetTables() map[string]TableDefs {
	return map[string]TableDefs{
		`purchase_event`: {`create table ... ()`, `insert into purchased () values ()`, eventPurchasedInsert},
	}
}

// type BatchFn func(driver.Batch)

type batchPair struct {
	stmt string
	fn   func(driver.Batch, any) error
}

func eventPurchasedInsert(b driver.Batch, evt any) error {
	pevt, ok := evt.(*eventPurchased)
	if !ok {
		return errors.New("wrong type")
	}
	return b.Append(pevt.eventID, pevt.userID, pevt.itemID)
}

type Tables struct {
	applyFns []func(any)
}

func GetTable[T any](tables Tables, name string) Table[T] {
	return &table[T]{}
}

type table[T any] struct {
	name      string
	batch     driver.Batch
	bfn       BatchFn
	notifier  notifierapi.Topic
	callbacks []func(context.Context)
}

// Write to batch, but also notify
func (s *table[T]) Write(data T) {
	s.bfn(s.batch, data)
	s.callbacks = append(s.callbacks, func(ctx context.Context) {
		s.notifier.Broadcast(ctx, data)
	})
}

type Table[T any] interface {
	Write(t T)
}

// an EWSM can handle submitted event from the state machine
func (t *ewsm) Update(tables Tables, data []byte) OnAfterCommit {

	purchaseTable := GetTable[*eventPurchased](tables, "purchase_evt")

	// 2. parse input
	msg, err := parseMsg(data)
	if err != nil {
		// return t.base.Update(out, data)
		return func() ([]byte, error) {
			return nil, err
		}
	}

	// 3. apply business logic
	switch msg.Type {
	case "hello":
		// validate input
		// validate against state
		// publish event
	case "i-want-to-purchase":
		// validate input
		// validate against state

		// publish event: yes, your purchase is valid

		purchaseTable.Write(&eventPurchased{eventID: t.purchaseCounter, userID: 123, itemID: 10001})

		t.purchaseCounter++

		return func() ([]byte, error) {
			return []byte("yes, your purchase is valid"), nil
		}
	}

	// return t.base.Update(out, data) || ORRR; it's already handled by the framework
	// so we have our default "happy" (not the fsm implementation), but this base "ewsm" default.
	return func() ([]byte, error) {
		return []byte("fail"), nil
	}
}

// An EWSM can handle query to the local DB connection for this state machine
func (t *ewsm) Query(conn driver.Conn, query any) (any, error) {
	ctx := context.Background()
	switch q := query.(type) {
	case EventQuery:
		// common event functinoality (can be extracted as library / base (parent) struct: eventQuery handler)
		// or exposed automatically. (it is up to the outer layer for the validation
		// ewsm will always have this); an API are integrated with this
		if _, ok := t.GetTables()[q.Name]; !ok {
			return nil, errors.New("unknown table " + q.Name)
		}
		if q.FromDatetime != nil {
			return conn.Query(ctx, `select a, b, c from `+q.Name+` where `, q.ToOffset, *q.FromDatetime)
		} else {
			return conn.Query(ctx, `select a, b, c from `+q.Name+` where `, q.ToOffset)
		}
	}

	return nil, errors.New("unsupported query")
}

type EventQuery struct {
	Name         string     `json:"name"`
	FromOffset   *uint64    `json:"from_offset,omitempty"`
	FromDatetime *time.Time `json:"from_datetime,omitempty"`
	ToOffset     uint64     `json:"to_offset,omitempty"`
}
