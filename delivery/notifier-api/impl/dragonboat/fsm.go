package dragonboat

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	notifierapi "github.com/desain-gratis/common/delivery/notifier-api"
	"github.com/lni/dragonboat/v4/raftio"
	"github.com/lni/dragonboat/v4/statemachine"
	sm "github.com/lni/dragonboat/v4/statemachine"
)

type Command struct {
	CmdName string          `json:"cmd_name"`
	CmdVer  uint64          `json:"cmd_version"`
	Data    json.RawMessage `json:"data"`
}

type Event struct {
	EvtName string          `json:"evt_name"`
	EvtVer  uint64          `json:"evt_version"`
	EvtID   uint64          `json:"evt_id"` // offset
	Data    json.RawMessage `json:"data"`
}

type messageBroker struct {
	shardID   uint64
	replicaID uint64
	notifier  notifierapi.Notifier

	leader     bool
	eventIndex uint64
	conn       driver.Conn

	listener map[uint32]*subscriber
}

func New(notifier notifierapi.Notifier) statemachine.CreateStateMachineFunc {
	return func(shardID, replicaID uint64) sm.IStateMachine {
		return &messageBroker{
			shardID:   shardID,
			replicaID: replicaID,
			notifier:  notifier,
		}
	}
}

// Lookup performs local lookup on the ExampleStateMachine instance. In this example,
// we always return the Count value as a little endian binary encoded byte
// slice.
func (s *messageBroker) Lookup(query interface{}) (interface{}, error) {
	if query == nil {
		return nil, fmt.Errorf("empty query")
	}

	// get latest snapshot table name / location & their latest apply index
	// for the consumer to query outside of FSM.

	// also get the current message index; for the consumer to query the log [apply, current]
	// outside the FSM.

	// The implementation needs to "guarantee" message received after current is forwarded afterwards

	// the subscriber can just add in the map WITHOUT LOCK, just use this FSM :"""
	// Might need to assess how to cleanly close this listener if no longer used. (eg if context is finished, remove also the map)

	id := rand.Uint32()
	s.listener[id] = &subscriber{
		lastApplyIdx:    0,
		currentEventIdx: s.eventIndex,
		exitMessage:     "byee hehe 👋🏼🥰",
	}

	return s.listener[id], nil
}

type subscriber struct {
	closed      bool
	ch          chan any
	exitMessage any

	lastApplyIdx    uint64
	currentEventIdx uint64
}

var _ notifierapi.Notifier = &subscriber{}

// in case of error, need to delete the entry in the FSM listener map
func (c *subscriber) Publish(_ context.Context, msg any) error {
	if c.closed {
		return fmt.Errorf("closed")
	}

	if c.ch == nil {
		return fmt.Errorf("no listener")
	}

	go func(msg any) {
		c.ch <- msg
	}(msg)

	return nil
}

func (c *subscriber) Listen(ctx context.Context) <-chan any {
	subscribeChan := make(chan any)
	c.ch = subscribeChan

	go func() {
		defer close(subscribeChan)

		<-ctx.Done()
		c.closed = true
		if c.exitMessage != nil {
			subscribeChan <- c.exitMessage
		}
	}()

	return c.ch
}

// Update updates the object using the specified committed raft entry.
func (s *messageBroker) Update(e sm.Entry) (sm.Result, error) {

	// switch cmd:
	// 1. Create snapshot (eg by cron or number of entry in leader node; or triggered by (admin) API call manually..)
	// 2. Write app command (eg. upsert & delete)

	var cmd Command
	err := json.Unmarshal(e.Cmd, &cmd)
	if err != nil {
		return sm.Result{Value: uint64(len(e.Cmd))}, err
	}

	s.eventIndex++

	// err = s.conn.Exec(context.Background(), `insert into ... dt `, "delete", "upsert", "etc as needed..")
	// if err != nil {
	// 	return sm.Result{Value: uint64(len(e.Cmd))}, err
	// }

	event := Event{
		EvtName: "ggwp",
		EvtVer:  1,
		EvtID:   s.eventIndex,
		Data:    cmd.Data,
	}

	ctx := context.Background()

	// s.notifier.Publish(ctx, event)

	for key, listener := range s.listener {
		err := listener.Publish(ctx, event)
		if err != nil {
			delete(s.listener, key)
		}
	}

	// write write write write to di log command.

	return sm.Result{Value: s.eventIndex}, nil
}

// SaveSnapshot saves the current IStateMachine state into a snapshot using the
// specified io.Writer object.
func (s *messageBroker) SaveSnapshot(w io.Writer,
	fc sm.ISnapshotFileCollection, done <-chan struct{}) error {
	// as shown above, the only state that can be saved is the Count variable
	// there is no external file in this IStateMachine example, we thus leave
	// the fc untouched
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, s.eventIndex)
	_, err := w.Write(data)

	// create new table using (all time, already there) + (last apply ,all commmand until current apply index)
	//
	// the table contain ALL data from the beginning of time (yes, a snapshot..)
	// the snapshot frequency can be high it's OK. and the cummulative data can be large yes & contain multiple duplicate yes.
	//

	return err
}

// RecoverFromSnapshot recovers the state using the provided snapshot.
func (s *messageBroker) RecoverFromSnapshot(r io.Reader,
	files []sm.SnapshotFile,
	done <-chan struct{}) error {
	// restore the Count variable, that is the only state we maintain in this
	// example, the input files is expected to be empty

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	v := binary.LittleEndian.Uint64(data)
	s.eventIndex = v

	return nil
}

// Close closes the IStateMachine instance. There is nothing for us to cleanup
// or release as this is a pure in memory data store. Note that the Close
// method is not guaranteed to be called as node can crash at any time.
func (s *messageBroker) Close() error { return nil }

func (s *messageBroker) leadershipChange(_ uint64, msg json.RawMessage) (sm.Result, error) {
	v, err := UnmarshalAs[raftio.LeaderInfo](msg)
	if err != nil {
		return sm.Result{}, err
	}

	s.leader = v.LeaderID == s.replicaID

	return sm.Result{Value: 1}, nil
}

func UnmarshalAs[T any](msg json.RawMessage) (T, error) {
	var t T
	err := json.Unmarshal(msg, &t)
	return t, err
}
