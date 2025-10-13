package replicated

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	notifierapi "github.com/desain-gratis/common/example/message-broker/src/log-api"

	"github.com/lni/dragonboat/v4/statemachine"
	sm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

type topicSM struct {
	shardID   uint64
	replicaID uint64

	leader       bool
	appliedIndex uint64
	conn         driver.Conn
	topic        notifierapi.Topic
}

// NewSM specify message topic implementation
func NewSMF(topic notifierapi.Topic) statemachine.CreateStateMachineFunc {
	return func(shardID, replicaID uint64) sm.IStateMachine {
		return &topicSM{
			shardID:   shardID,
			replicaID: replicaID,
			topic:     topic,
		}
	}
}

// Lookup performs local lookup on the ExampleStateMachine instance. In this example,
// we always return the Count value as a little endian binary encoded byte
// slice.
func (s *topicSM) Lookup(query interface{}) (interface{}, error) {
	if query == nil {
		return nil, fmt.Errorf("empty query")
	}

	_, ok := query.(QuerySubscribe)
	if !ok {
		return nil, fmt.Errorf("invalid query")
	}

	subs, err := s.topic.Subscribe()
	if err != nil {
		return nil, err
	}

	log.Info().Msgf("created local subscriber: %v", subs.ID())

	return subs, nil
}

// Update updates the object using the specified committed raft entry.
func (s *topicSM) Update(e sm.Entry) (sm.Result, error) {
	var cmd UpdateRequest
	err := json.Unmarshal(e.Cmd, &cmd)
	if err != nil {
		return sm.Result{Value: uint64(len(e.Cmd)), Data: []byte("failed marshal")}, nil
	}

	log.Info().Msgf("applying update: %v", string(e.Cmd))

	switch cmd.CmdName {
	case Command_UpdateLeader:
		return sm.Result{Value: uint64(len(e.Cmd)), Data: []byte("yes")}, nil
	case Command_StartSubscription:
		return s.startSubscription(cmd.Data)
	case Command_PublishMessage:
		return s.publishMessage(EventName_Echo, cmd.Data)
	case Command_NotifyOnline:
		return s.publishMessage(EventName_NotifyOnline, json.RawMessage(cmd.Data))
	case Command_NotifyOffline:
		return s.publishMessage(EventName_NotifyOffline, json.RawMessage(cmd.Data))
	}

	log.Info().Msgf("unknown command: %v", cmd.CmdName)

	resp, _ := json.Marshal(UpdateResponse{
		Error: fmt.Errorf("unknown command: %v", cmd.CmdName),
	})

	return sm.Result{Value: s.appliedIndex, Data: resp}, nil
}

func (s *topicSM) PrepareSnapshot() (interface{}, error) {
	return nil, nil
}

// SaveSnapshot saves the current IStateMachine state into a snapshot using the
// specified io.Writer object.
func (s *topicSM) SaveSnapshot(w io.Writer,
	fc sm.ISnapshotFileCollection, done <-chan struct{}) error {

	log.Info().Msgf("save snapshot triggered")

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
func (s *topicSM) RecoverFromSnapshot(r io.Reader,
	files []sm.SnapshotFile,
	done <-chan struct{}) error {
	// restore the Count variable, that is the only state we maintain in this
	// example, the input files is expected to be empty

	// Just query the log clickhouse log, check the latest applied index

	log.Info().Msgf("recover from snapshot triggered")

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
func (s *topicSM) Close() error { return nil }

func (s *topicSM) startSubscription(rawData json.RawMessage) (sm.Result, error) {
	var data StartSubscriptionData
	err := json.Unmarshal(rawData, &data)
	if err != nil {
		resp, _ := json.Marshal(AddSubscriptionResponse{
			Error: err,
		})
		return sm.Result{Value: uint64(0), Data: resp}, nil
	}

	if data.ReplicaID != s.replicaID {
		log.Info().Msgf("different replica triggered")
		resp, _ := json.Marshal(UpdateResponse{
			Message: "should not happen on different replica",
		})

		// listener only created on the requested replica.
		return sm.Result{Value: uint64(0), Data: resp}, nil
	}

	subs, err := s.topic.GetSubscription(data.SubscriptionID)
	if err != nil {
		resp, _ := json.Marshal(UpdateResponse{
			Message: "skip (no listener)",
		})
		log.Info().Msgf("skip triggered")
		return sm.Result{Value: uint64(0), Data: resp}, nil
	}

	log.Info().Msgf("starting local subscriber: %v", subs.ID())
	subs.Start()

	resp, _ := json.Marshal(UpdateResponse{
		Message: "success",
	})

	return sm.Result{Value: s.appliedIndex, Data: resp}, nil
}

func (s *topicSM) publishMessage(name EventName, rawData json.RawMessage) (sm.Result, error) {
	// since we can publish without using the same connection, identity cannot be determined
	s.topic.Broadcast(context.Background(), Event{
		EvtName: name,
		EvtVer:  0,
		EvtID:   s.appliedIndex,
		Data:    rawData,
	})
	s.appliedIndex++
	return sm.Result{Value: s.appliedIndex, Data: []byte("success!")}, nil
}
