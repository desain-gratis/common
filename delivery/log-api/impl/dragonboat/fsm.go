package dragonboat

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	notifierapi "github.com/desain-gratis/common/delivery/log-api"
	"github.com/lni/dragonboat/v4/statemachine"
	sm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

type messagetopic struct {
	shardID   uint64
	replicaID uint64

	leader       bool
	appliedIndex uint64
	conn         driver.Conn
	topic        notifierapi.Topic
}

// Specify message topic implementation
func New(topic notifierapi.Topic) statemachine.CreateStateMachineFunc {
	return func(shardID, replicaID uint64) sm.IStateMachine {
		return &messagetopic{
			shardID:   shardID,
			replicaID: replicaID,
			topic:     topic,
		}
	}
}

// Lookup performs local lookup on the ExampleStateMachine instance. In this example,
// we always return the Count value as a little endian binary encoded byte
// slice.
func (s *messagetopic) Lookup(query interface{}) (interface{}, error) {
	if query == nil {
		return nil, fmt.Errorf("empty query")
	}

	q, ok := query.(SubscribeRequest)
	if !ok {
		return nil, fmt.Errorf("invalid query")
	}

	subs, err := s.topic.GetSubscription(string(q))
	if err != nil {
		return nil, err
	}

	return subs, nil
}

// Update updates the object using the specified committed raft entry.
func (s *messagetopic) Update(e sm.Entry) (sm.Result, error) {
	var cmd Command
	err := json.Unmarshal(e.Cmd, &cmd)
	if err != nil {
		return sm.Result{Value: uint64(len(e.Cmd)), Data: []byte("Fail miserably!")}, nil
	}

	// todo: sync write log entry to clickhouse, this one is immutable;
	// not a key value based.
	// based on:
	// e.Index

	if cmd.CmdName == "add-subscription" {
		var replicaID uint64
		err = json.Unmarshal(cmd.Data, &replicaID)
		if err != nil {
			return sm.Result{Value: uint64(len(e.Cmd)), Data: []byte("Invalid replicaID data miserably!")}, nil
		}

		if replicaID != s.replicaID {
			// listener only created on the requested replica.
			return sm.Result{Value: uint64(len(e.Cmd)), Data: []byte(
				fmt.Sprintf("not the expected replica. expected: %v, got: %v", s.replicaID, replicaID))}, nil
		}

		log.Info().Msgf("Adding subcription for replica ID: %v", replicaID)

		subsID, subs := s.topic.Subscribe()
		subs.Publish(context.Background(), fmt.Sprintf("Welcome!👋🏼 starting to listen at: %v", s.appliedIndex))

		return sm.Result{Value: s.appliedIndex, Data: []byte(subsID)}, nil
	}

	s.appliedIndex = e.Index
	event := Event{
		EvtName: "tail",
		EvtVer:  1,
		EvtID:   s.appliedIndex,
		Data:    cmd.Data,
	}

	// strictly log processing (not key value)
	// but user can extend the fsm later, we should provide hook to handle the message

	// broadcast to listener upon success write log
	s.topic.Broadcast(context.Background(), event)

	// write write write write to di log command.

	return sm.Result{Value: s.appliedIndex}, nil
}

// SaveSnapshot saves the current IStateMachine state into a snapshot using the
// specified io.Writer object.
func (s *messagetopic) SaveSnapshot(w io.Writer,
	fc sm.ISnapshotFileCollection, done <-chan struct{}) error {
	// should be not need to do anything since we store them all in clickhouse log
	// instead, we can have administrative update command to configure log retention / data trimming
	// so this basically making this a log server.

	// as shown above, the only state that can be saved is the Count variable
	// there is no external file in this IStateMachine example, we thus leave
	// the fc untouched
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, s.appliedIndex)
	_, err := w.Write(data)

	// create new table using (all time, already there) + (last apply ,all commmand until current apply index)
	//
	// the table contain ALL data from the beginning of time (yes, a snapshot..)
	// the snapshot frequency can be high it's OK. and the cummulative data can be large yes & contain multiple duplicate yes.
	//

	return err
}

// RecoverFromSnapshot recovers the state using the provided snapshot.
func (s *messagetopic) RecoverFromSnapshot(r io.Reader,
	files []sm.SnapshotFile,
	done <-chan struct{}) error {
	// restore the Count variable, that is the only state we maintain in this
	// example, the input files is expected to be empty

	// Just query the log clickhouse log, check the latest applied index

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	v := binary.LittleEndian.Uint64(data)
	s.appliedIndex = v

	return nil
}

// Close closes the IStateMachine instance. There is nothing for us to cleanup
// or release as this is a pure in memory data store. Note that the Close
// method is not guaranteed to be called as node can crash at any time.
func (s *messagetopic) Close() error { return nil }
