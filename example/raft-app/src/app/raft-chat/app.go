package raftchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/desain-gratis/common/lib/notifier"
	"github.com/desain-gratis/common/lib/raft"
	notifierhelper "github.com/desain-gratis/common/lib/raft/notifier-helper"
	raft_runner "github.com/desain-gratis/common/lib/raft/runner"
	sm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

const (
	appName = "chat_app"

	TopicChatLog = "chat_log"
)

var _ raft.Application = &chatWriterApp{}

// happySM to isolate all business logic from the state machine technicality
// because this is an OLAP usecase,  writing to DB, choosing the appropriate DB & indexes are tightly coupled.
// will not try to abstract away
type chatWriterApp struct {
	state     *state
	topicReg  notifierhelper.TopicRegistry
	replicaID uint64
	shardID   uint64
}

func New(topic notifier.Topic, shardID, replicaID uint64) *chatWriterApp {
	nh := notifierhelper.NewTopicRegistry(map[string]notifier.Topic{
		TopicChatLog: topic,
	})
	return &chatWriterApp{
		topicReg:  nh,
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
		return s.topicReg[q.Topic], nil
	case QueryLog:
		// query historical log
		return s.queryLog(ctx, q)
	}

	return nil, errors.New("unsupported query")
}

func (s *chatWriterApp) Init(ctx context.Context) error {
	conn := raft_runner.GetClickhouseConnection(ctx)

	// prepare chat log table
	if err := conn.Exec(ctx, DDLChatLog); err != nil {
		return err
	}

	// get metadata
	meta, err := raft_runner.GetMetadata(ctx, appName)
	if err != nil {
		return err
	}

	s.state = &state{}

	if len(meta) > 0 {
		err = json.Unmarshal(meta, s.state)
		if err != nil {
			return err
		}
	}

	if s.state.ChatIndex == nil {
		var idx uint64
		s.state.ChatIndex = &idx
	}

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
			err := s.startSubscription(e, cmd.Data, chatIdx)
			if err != nil {
				resp, _ := json.Marshal(StartSubscriptionResponse{
					Error: err,
				})
				return sm.Result{Value: uint64(0), Data: resp}, nil
			}
			resp, _ := json.Marshal(UpdateResponse{
				Message: strconv.FormatUint(chatIdx, 10),
			})

			return sm.Result{Value: chatIdx, Data: resp}, nil
		}
	case Command_NotifyOnline, Command_NotifyOffline:
		return func() (sm.Result, error) {
			err := s.topicReg[TopicChatLog].Broadcast(context.Background(), Event{
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
			EvtTable:        TopicChatLog,
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
		conn := raft_runner.GetClickhouseConnection(ctx)
		err = conn.AsyncInsert(ctx, DMLWriteChat, true, "default", chatIdx, serverTimestamp, string(cmd.Data))
		if err != nil {
			log.Panic().Msgf("err async insert: %v", err)
		}

		// increment our index
		*s.state.ChatIndex++

		return func() (sm.Result, error) {
			// we publish wrapped data
			err := s.topicReg[TopicChatLog].Broadcast(context.Background(), chat)
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
	err := raft_runner.SetMetadata(ctx, appName, payload)
	if err != nil {
		return err
	}

	return nil
}

func (s *chatWriterApp) startSubscription(ent sm.Entry, rawData json.RawMessage, startIdx uint64) error {
	var data notifierhelper.StartSubscriptionRequest
	_ = json.Unmarshal(rawData, &data)
	err := s.topicReg.StartSubscription(s.replicaID, startIdx, data)
	if err != nil {
		return err
	}

	return nil
}

func (s *chatWriterApp) queryLog(ctx context.Context, q QueryLog) (chan Event, error) {
	var rows driver.Rows
	var err error

	conn := raft_runner.GetClickhouseConnection(ctx)

	if q.FromDatetime != nil {
		rows, err = conn.Query(q.Ctx, DQLReadAll, q.ToOffset, *q.FromDatetime)
	} else {
		rows, err = conn.Query(q.Ctx, DQLReadAll, q.ToOffset)
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

			evt.EvtTable = TopicChatLog
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
