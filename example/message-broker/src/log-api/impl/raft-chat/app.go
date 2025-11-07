package raftchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/desain-gratis/common/example/message-broker/src/log-api/impl/statemachine"
	"github.com/desain-gratis/common/lib/notifier"
	"github.com/desain-gratis/common/lib/notifier/impl"
	"github.com/desain-gratis/common/lib/raft"
	sm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

const (
	appName = "chat_app"
)

var _ raft.Application = &chatWriterApp{}

// happySM to isolate all business logic from the state machine technicality
// because this is an OLAP usecase,  writing to DB, choosing the appropriate DB & indexes are tightly coupled.
// will not try to abstract away
type chatWriterApp struct {
	conn      driver.Conn
	state     *state
	topic     notifier.Topic
	replicaID uint64
	shardID   uint64
}

func New(topic notifier.Topic, shardID, replicaID uint64) *chatWriterApp {
	return &chatWriterApp{
		topic:     topic,
		shardID:   shardID,
		replicaID: replicaID,
	}
}

func (s *chatWriterApp) Lookup(ctx context.Context, query interface{}) (interface{}, error) {
	if query == nil {
		return nil, fmt.Errorf("empty query")
	}

	switch q := query.(type) {
	case Subscribe:
		// subscribe to real time event for log update
		log.Info().Msgf("I WANT TO SUBSCRIBE: %T %+v", q, q)
		return s.topic, nil
	case QueryLog:
		// query historical log
		return s.queryLog(ctx, q)
	}

	return nil, errors.New("unsupported query")
}

func (s *chatWriterApp) Init(ctx context.Context) error {
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
		s.state = &state{}
	}

	if s.state.ChatIndex == nil {
		var idx uint64
		s.state.ChatIndex = &idx
	}

	s.conn = conn

	return nil
}

// PrepareUpdate prepare the resources for upcoming message
func (s *chatWriterApp) PrepareUpdate(ctx context.Context) (context.Context, error) {
	// conn := statemachine.GetClickhouseConnection(ctx)

	// batch, err := conn.PrepareBatch(ctx, DMLWriteChat)
	// if err != nil {
	// 	return ctx, err
	// }

	// return context.WithValue(ctx, chatTableKey, batch), nil
	return ctx, nil
}

// OnUpdate updates the object using the specified committed raft entry.
func (s *chatWriterApp) OnUpdate(ctx context.Context, e sm.Entry) raft.OnAfterApply {
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

		// persist in batch
		// chatBatch, ok := ctx.Value(chatTableKey).(driver.Batch)
		// if !ok {
		// 	return func() (sm.Result, error) {
		// 		return sm.Result{Data: []byte("error")}, nil
		// 	}
		// }

		// we store "only" the user defined data. the actual log.
		// chatBatch.Append("default", chatIdx, serverTimestamp, string(cmd.Data))

		// try to use async insert instead of batch
		conn := statemachine.GetClickhouseConnection(ctx)
		err = conn.AsyncInsert(ctx, DMLWriteChat, true, "default", chatIdx, serverTimestamp, string(cmd.Data))
		if err != nil {
			log.Panic().Msgf("err async insert: %v", err)
		}

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
func (s *chatWriterApp) Apply(ctx context.Context) error {
	// chatBatch, ok := ctx.Value(chatTableKey).(driver.Batch)
	// if !ok {
	// 	log.Panic().Msgf("tidak semestinya")
	// }

	// err := chatBatch.Send()
	// if err != nil {
	// 	return err
	// }

	// err = chatBatch.Close()
	// if err != nil {
	// 	return err
	// }

	// save metadata
	payload, _ := json.Marshal(s.state)
	err := statemachine.SetMetadata(ctx, appName, payload)
	if err != nil {
		return err
	}

	return nil
}

func (s *chatWriterApp) startSubscription(ent sm.Entry, rawData json.RawMessage, startIdx uint64) (sm.Result, error) {
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
		if !errors.Is(err, impl.ErrNotFound) {
			log.Err(err).Msgf("err get subs %v: %v %v", err, data.SubscriptionID, string(rawData))
		}
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

func (s *chatWriterApp) queryLog(ctx context.Context, q QueryLog) (chan Event, error) {
	var rows driver.Rows
	var err error

	if q.FromDatetime != nil {
		rows, err = s.conn.Query(q.Ctx, DQLReadAll, q.ToOffset, *q.FromDatetime)
	} else {
		rows, err = s.conn.Query(q.Ctx, DQLReadAll, q.ToOffset)
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
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
			if q.Ctx.Err() != nil {
				return
			}

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
