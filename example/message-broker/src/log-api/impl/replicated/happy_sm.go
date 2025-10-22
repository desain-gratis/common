package replicated

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	notifierapi "github.com/desain-gratis/common/example/message-broker/src/log-api"
	sm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

var _ Happy = &happySM{}

type happyMeta struct {
	ChatIndex *uint64 `json:"chat_index"`
}

// happySM to isolate all business logic from the state machine technicality
// because this is an OLAP usecase,  writing to DB, choosing the appropriate DB & indexes are tightly coupled.
// will not try to abstract away
type happySM struct {
	topic     notifierapi.Topic
	replicaID uint64
	shardID   uint64
}

func NewHappy(topic notifierapi.Topic, clickhouseAddr string) func(shardID uint64, replicaID uint64) sm.IOnDiskStateMachine {
	return func(shardID uint64, replicaID uint64) sm.IOnDiskStateMachine {
		database := fmt.Sprintf("%v_%v_%v", "chat_app", shardID, replicaID)
		return &baseDiskSM{
			database:       database,
			clickhouseAddr: clickhouseAddr,
			happy: &happySM{
				shardID:   shardID,
				replicaID: replicaID,
				topic:     topic,
			},
		}
	}
}

func (s *happySM) Lookup(ctx context.Context, query interface{}) (interface{}, error) {
	if query == nil {
		return nil, fmt.Errorf("empty query")
	}

	switch q := query.(type) {
	case SubscribeLog:
		// subscribe to real time event for log update
		return s.getSubscription()
	case QueryLog:
		// query historical log
		return s.queryLog(ctx, q)
	}

	return nil, errors.New("unsupported query")
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
	conn, ok := ctx.Value(chConnKey).(driver.Conn)
	if !ok {
		return nil, errors.New("not a clickhouse context")
	}

	var rows driver.Rows
	var err error

	if q.FromDatetime != nil {
		rows, err = conn.Query(ctx, DQLReadAll, q.ToOffset, *q.FromDatetime)
	} else {
		rows, err = conn.Query(ctx, DQLReadAll, q.ToOffset)
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

type Log struct {
	TableID         string    `json:"table_id"`
	EventID         uint64    `json:"event_id"`
	ServerTimestamp time.Time `json:"server_timestamp"`
	Data            []byte    `json:"data"`
}

// PrepareUpdate prepare the resources for upcoming message
func (s *happySM) PrepareUpdate(ctx context.Context) (context.Context, error) {
	conn, ok := ctx.Value(chConnKey).(driver.Conn)
	if !ok {
		return nil, errors.New("not a clickhouse context")
	}

	batch, err := conn.PrepareBatch(context.Background(), DMLWriteChat)
	if err != nil {
		return ctx, err
	}

	return context.WithValue(ctx, chatTableKey, batch), nil
}

// OnUpdate updates the object using the specified committed raft entry.
func (s *happySM) OnUpdate(ctx context.Context, e sm.Entry) OnAfterApply {
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

	metadata, ok := ctx.Value(metadataKey).(*Metadata)
	if !ok {
		log.Panic().Msgf("not a metadata")
	}

	chatIdx := *metadata.ChatLog_EvtIndex_Counter

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
			s.topic.Broadcast(context.Background(), Event{
				EvtName: EventName(cmd.CmdName),
				EvtID:   chatIdx, // latest chat index;
				Data:    cmd.Data,
			})

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
		*metadata.ChatLog_EvtIndex_Counter++

		return func() (sm.Result, error) {
			// we publish wrapped data
			s.topic.Broadcast(context.Background(), chat)

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
		log.Info().Msgf("different replica triggered")
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
		log.Info().Msgf("skip triggered")
		return sm.Result{Value: uint64(0), Data: resp}, nil
	}

	log.Info().Msgf("starting local subscriber: %v %v", subs.ID(), string(rawData))
	subs.Start()

	resp, _ := json.Marshal(UpdateResponse{
		Message: strconv.FormatUint(startIdx, 10),
	})

	return sm.Result{Value: ent.Index, Data: resp}, nil
}
