package chatlogwriter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	notifierapi "github.com/desain-gratis/common/example/message-broker/src/log-api"
	"github.com/desain-gratis/common/example/message-broker/src/log-api/impl/statemachine"
	"github.com/desain-gratis/common/lib/logwriter"
	sm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

const (
	appName = "chat_app"
)

var _ logwriter.Happy = &happySM{}

type happyState struct {
	ChatIndex *uint64 `json:"chat_index"`
}

// happySM to isolate all business logic from the state machine technicality
// because this is an OLAP usecase,  writing to DB, choosing the appropriate DB & indexes are tightly coupled.
// will not try to abstract away
type happySM struct {
	conn      driver.Conn
	state     *happyState
	topic     notifierapi.Topic
	replicaID uint64
	shardID   uint64
}

func NewHappy(topic notifierapi.Topic, shardID, replicaID uint64) *happySM {
	return &happySM{
		topic:     topic,
		shardID:   shardID,
		replicaID: replicaID,
	}
}

func (s *happySM) Lookup(ctx context.Context, query interface{}) (interface{}, error) {
	if query == nil {
		return nil, fmt.Errorf("empty query")
	}

	switch q := query.(type) {
	case SubscribeLog:
		// subscribe to real time event for log update
		log.Info().Msgf("I WANT TO SUBSCRIBE: %T %+v", q, q)
		return s.getSubscription()
	case QueryLog:
		// query historical log
		return s.queryLog(ctx, q)
	}

	return nil, errors.New("unsupported query")
}

type Log struct {
	TableID         string    `json:"table_id"`
	EventID         uint64    `json:"event_id"`
	ServerTimestamp time.Time `json:"server_timestamp"`
	Data            []byte    `json:"data"`
}

func (s *happySM) Init(ctx context.Context) error {
	conn := statemachine.GetClickhouseConnection(ctx)

	// prepare chat log table
	if err := conn.Exec(ctx, DDLChatLog); err != nil {
		return err
	}

	// get metadata
	meta, err := statemachine.GetMetadata(ctx, appName)
	if err != nil {
		return err
	}

	_ = json.Unmarshal(meta, s.state)

	if s.state == nil {
		s.state = &happyState{}
	}

	if s.state.ChatIndex == nil {
		var idx uint64
		s.state.ChatIndex = &idx
	}

	s.conn = conn

	return nil
}

// PrepareUpdate prepare the resources for upcoming message
func (s *happySM) PrepareUpdate(ctx context.Context) (context.Context, error) {
	conn := statemachine.GetClickhouseConnection(ctx)

	batch, err := conn.PrepareBatch(context.Background(), DMLWriteChat)
	if err != nil {
		return ctx, err
	}

	return context.WithValue(ctx, chatTableKey, batch), nil
}

// OnUpdate updates the object using the specified committed raft entry.
func (s *happySM) OnUpdate(ctx context.Context, e sm.Entry) logwriter.OnAfterApply {
	var cmd UpdateRequest
	err := json.Unmarshal(e.Cmd, &cmd)
	if err != nil {
		resp, _ := json.Marshal(UpdateResponse{
			Error: fmt.Errorf("failed to parse request: %v", err),
		})
		return func() (sm.Result, error) {
			return sm.Result{Data: resp}, nil
		}
	}

	chatIdx := *s.state.ChatIndex

	switch cmd.CmdName {
	case Command_UpdateLeader:
		return func() (sm.Result, error) {
			return sm.Result{
				Value: uint64(len(e.Cmd)),
				Data:  []byte("yes"),
			}, nil
		}
	case Command_StartSubscription:
		return func() (sm.Result, error) {
			return s.startSubscription(e, cmd.Data, chatIdx)
		}
	case Command_NotifyOnline, Command_NotifyOffline:
		return func() (sm.Result, error) {
			err := s.topic.Broadcast(context.Background(), Event{
				EvtName: EventName(cmd.CmdName),
				EvtID:   chatIdx, // latest chat index;
				Data:    cmd.Data,
			})
			if err != nil {
				log.Err(err).Msgf("error brodkest: %v", cmd.CmdName)
			}

			return sm.Result{Value: chatIdx, Data: []byte("success!")}, nil
		}
	case Command_PublishMessage:
		// persist in batch
		chatBatch, ok := ctx.Value(chatTableKey).(driver.Batch)
		if !ok {
			return func() (sm.Result, error) {
				return sm.Result{Data: []byte("error")}, nil
			}
		}

		// what we store is what we publish
		serverTimestamp := time.Now()
		chat := Event{
			EvtTable:        "chat_log",
			EvtName:         EventName_Echo,
			EvtID:           chatIdx,
			ServerTimestamp: serverTimestamp,

			// user defined. the actual log.
			Data: cmd.Data,
		}

		// we store "only" the user defined data. the actual log.
		chatBatch.Append("default", chatIdx, serverTimestamp, string(cmd.Data))

		// increment our index
		*s.state.ChatIndex++

		return func() (sm.Result, error) {
			// we publish wrapped data
			err := s.topic.Broadcast(context.Background(), chat)
			if err != nil {
				log.Err(err).Msgf("error kirim data %T", chat)
			}

			return sm.Result{Value: chat.EvtID, Data: []byte("success!")}, nil
		}
	}

	log.Info().Msgf("unknown command: %v", cmd.CmdName)
	resp, _ := json.Marshal(UpdateResponse{
		Error: fmt.Errorf("unknown command: %v", cmd.CmdName),
	})

	return func() (sm.Result, error) {
		return sm.Result{Data: resp}, nil
	}
}

// Apply or "Sync". The core of dragonboat's state machine "Update" function.
func (s *happySM) Apply(ctx context.Context) error {
	chatBatch, ok := ctx.Value(chatTableKey).(driver.Batch)
	if !ok {
		log.Panic().Msgf("tidak semestinya")
	}

	err := chatBatch.Send()
	if err != nil {
		return err
	}

	err = chatBatch.Close()
	if err != nil {
		return err
	}

	// save metadata
	payload, _ := json.Marshal(s.state)
	err = statemachine.SetMetadata(ctx, appName, payload)
	if err != nil {
		return err
	}

	return nil
}

func (s *happySM) startSubscription(ent sm.Entry, rawData json.RawMessage, startIdx uint64) (sm.Result, error) {
	var data StartSubscriptionData
	err := json.Unmarshal(rawData, &data)
	if err != nil {
		log.Err(err).Msgf("err while start subscription unmarshal %v", err)
		resp, _ := json.Marshal(AddSubscriptionResponse{
			Error: err,
		})
		return sm.Result{Value: uint64(0), Data: resp}, nil
	}

	if data.ReplicaID != s.replicaID {
		resp, _ := json.Marshal(UpdateResponse{
			Message: "should not happen on different replica",
		})

		// listener only created on the requested replica.
		return sm.Result{Value: uint64(0), Data: resp}, nil
	}

	subs, err := s.topic.GetSubscription(data.SubscriptionID)
	if err != nil {
		log.Err(err).Msgf("err get subs %v: %v %v", err, data.SubscriptionID, string(rawData))
		resp, _ := json.Marshal(UpdateResponse{
			Message: "skip (no listener)",
		})
		return sm.Result{Value: uint64(0), Data: resp}, nil
	}

	log.Info().Msgf("starting local subscriber: %v %v", subs.ID(), string(rawData))
	subs.Start()

	resp, _ := json.Marshal(UpdateResponse{
		Message: strconv.FormatUint(startIdx, 10),
	})

	return sm.Result{Value: ent.Index, Data: resp}, nil
}

func (s *happySM) getSubscription() (notifierapi.Subscription, error) {
	subs, err := s.topic.Subscribe()
	if err != nil {
		log.Err(err).Msgf("LOH PAK BU")
		return nil, err
	}

	log.Info().Msgf("created local subscriber: %v", subs.ID())

	return subs, nil
}

func (s *happySM) queryLog(ctx context.Context, q QueryLog) (chan Event, error) {
	var rows driver.Rows
	var err error

	if q.FromDatetime != nil {
		rows, err = s.conn.Query(ctx, DQLReadAll, q.ToOffset, *q.FromDatetime)
	} else {
		rows, err = s.conn.Query(ctx, DQLReadAll, q.ToOffset)
	}

	if err != nil {
		log.Err(err).Msgf("helo failed to qeuery clickhouse")
		return nil, err
	}

	result := make(chan Event)
	go func() {
		defer close(result)
		defer rows.Close()
		defer func() {
			log.Info().Msgf("query finished")
		}()

		for rows.Next() {
			var evt Event

			evt.EvtTable = "chat_log"
			evt.EvtName = EventName_Echo // todo: maybe move it somewher

			var namespace string
			var data string
			err := rows.Scan(&namespace, &evt.EvtID, &evt.ServerTimestamp, &data)
			if err != nil {
				log.Err(err).Msgf("error scaning row")
				return
			}
			evt.Data = []byte(data)
			result <- evt
		}
	}()

	return result, nil
}
