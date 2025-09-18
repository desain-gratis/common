package dragonboat

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	notifierapi "github.com/desain-gratis/common/delivery/notifier-api"
	"github.com/lni/dragonboat/v4/statemachine"
	sm "github.com/lni/dragonboat/v4/statemachine"
)

type messageBroker struct {
	shardID   uint64
	replicaID uint64

	leader     bool
	eventIndex uint64
	conn       driver.Conn
	broker     notifierapi.Broker
}

// Specify message broker implementation
func New(broker notifierapi.Broker) statemachine.CreateStateMachineFunc {
	return func(shardID, replicaID uint64) sm.IStateMachine {
		return &messageBroker{
			shardID:   shardID,
			replicaID: replicaID,
			broker:    broker,
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

	q, ok := query.(Query)
	if !ok {
		return nil, fmt.Errorf("invalid query")
	}

	if q == Query_Subscribe {
		subs := s.broker.Subscribe()
		return subs, nil
	}

	return nil, fmt.Errorf("unsupported query")
}

// Update updates the object using the specified committed raft entry.
func (s *messageBroker) Update(e sm.Entry) (sm.Result, error) {
	var cmd Command
	err := json.Unmarshal(e.Cmd, &cmd)
	if err != nil {
		return sm.Result{Value: uint64(len(e.Cmd)), Data: []byte("Fail miserably!")}, nil
	}

	s.eventIndex++

	event := Event{
		EvtName: "ggwp",
		EvtVer:  1,
		EvtID:   s.eventIndex,
		Data:    cmd.Data,
	}

	s.broker.Broadcast(context.Background(), event)

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
