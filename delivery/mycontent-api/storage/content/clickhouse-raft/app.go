package clickhouseraft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/desain-gratis/common/lib/notifier"
	"github.com/desain-gratis/common/lib/raft"
	notifierhelper "github.com/desain-gratis/common/lib/raft/notifier-helper"
	raft_runner "github.com/desain-gratis/common/lib/raft/runner"
	"github.com/google/uuid"
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
	state       *state
	topicReg    notifierhelper.TopicRegistry
	tableConfig map[string]TableConfig
}

type TableConfig struct {
	Name    string
	RefSize int
}

func New(topic notifier.Topic, tableConfig ...TableConfig) *chatWriterApp {
	nh := notifierhelper.NewTopicRegistry(map[string]notifier.Topic{
		TopicChatLog: topic,
	})

	if len(tableConfig) == 0 {
		log.Panic().Msgf("empty table config")
	}

	tableConfigMap := make(map[string]TableConfig)
	for _, c := range tableConfig {
		tableConfigMap[c.Name] = c
		if c.RefSize < 0 || c.RefSize > 20 {
			log.Panic().Msgf("invalid refSize: %v", c.RefSize)
		}
	}

	return &chatWriterApp{
		topicReg:    nh,
		tableConfig: tableConfigMap,
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

	for _, table := range s.tableConfig {
		ddl := getDDL(table.Name, table.RefSize)
		err := conn.Exec(ctx, ddl)
		if err != nil {
			log.Panic().Msgf("failed to execute DDL for table %v (%v): %v", table.Name, table.RefSize, err)
		}
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

	if s.state.EventIndexes == nil {
		s.state.EventIndexes = make(map[string]*uint64)
	}

	return nil
}

// PrepareUpdate prepare the resources for upcoming message
func (s *chatWriterApp) PrepareUpdate(ctx context.Context) (context.Context, context.CancelFunc, error) {
	// conn := statemachine.GetClickhouseConnection(ctx)

	// batch, err := conn.PrepareBatch(ctx, DMLWriteChat)
	// if err != nil {
	// 	return ctx, err
	// }

	// return context.WithValue(ctx, chatTableKey, batch), nil
	return ctx, func() {}, nil
}

func (s *chatWriterApp) subscribe(ctx context.Context, payload DataWrapper) (raft.OnAfterApply, error) {
	tableCfg, ok := s.tableConfig[payload.Table]
	if !ok {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte("invalid table")}, nil
		}, nil
	}

	_ = tableCfg

	encResult, err := json.Marshal(map[string]any{
		"message": "ggwp",
	})
	if err != nil {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error marshal: %v", err))}, nil
		}, nil
	}

	return func() (raft.Result, error) {
		return raft.Result{Value: 1, Data: encResult}, nil
	}, nil
}

func (s *chatWriterApp) post(ctx context.Context, payload DataWrapper) (raft.OnAfterApply, error) {
	tableCfg, ok := s.tableConfig[payload.Table]
	if !ok {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte("invalid table")}, nil
		}, nil
	}

	if len(payload.RefIDs) != tableCfg.RefSize {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte("invalid reference")}, nil
		}, nil
	}

	conn := raft_runner.GetClickhouseConnection(ctx)

	eventIdx, ok := s.state.EventIndexes[tableCfg.Name]
	if !ok {
		var index uint64
		s.state.EventIndexes[tableCfg.Name] = &index
		eventIdx = &index
	}

	id, cols, args, tmplt := s.preparePost(
		tableCfg.RefSize,
		*eventIdx,
		payload.Namespace,
		payload.RefIDs,
		payload.ID,
		string(payload.Data),
		string(payload.Meta),
		false,
	)

	q := `INSERT INTO ` + tableCfg.Name + `(` + strings.Join(cols, ",") + `) 
	VALUES (` + strings.Join(tmplt, ",") + `);
	`

	err := conn.Exec(ctx, q, args...)
	if err != nil {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error writing to table: %v", err))}, nil
		}, nil
	}

	*s.state.EventIndexes[tableCfg.Name]++

	payload.ID = id
	payload.EventID = *eventIdx

	encResult, err := json.Marshal(payload)
	if err != nil {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error marshal: %v", err))}, nil
		}, nil
	}

	return func() (raft.Result, error) {
		return raft.Result{Value: 0, Data: encResult}, nil
	}, nil
}

func (s *chatWriterApp) delete(ctx context.Context, payload DataWrapper) (raft.OnAfterApply, error) {
	tableCfg, ok := s.tableConfig[payload.Table]
	if !ok {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte("invalid table")}, nil
		}, nil
	}

	conn := raft_runner.GetClickhouseConnection(ctx)

	eventIdx, ok := s.state.EventIndexes[tableCfg.Name]
	if !ok {
		var index uint64
		s.state.EventIndexes[tableCfg.Name] = &index
		eventIdx = &index
	}

	_, cols, args, tmplt := s.preparePost(
		tableCfg.RefSize,
		*eventIdx,
		payload.Namespace,
		payload.RefIDs,
		payload.ID,
		string(payload.Data),
		string(payload.Meta),
		true,
	)

	q := `INSERT INTO ` + tableCfg.Name + `(` + strings.Join(cols, ",") + `) 
	VALUES (` + strings.Join(tmplt, ",") + `);
	`

	err := conn.Exec(ctx, q, args...)
	if err != nil {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error writing to table: %v", err))}, nil
		}, nil
	}

	*s.state.EventIndexes[tableCfg.Name]++

	encResult, err := json.Marshal(map[string]any{
		"result": "success",
	})
	if err != nil {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error marshal: %v", err))}, nil
		}, nil
	}

	return func() (raft.Result, error) {
		return raft.Result{Value: 0, Data: encResult}, nil
	}, nil
}

type DataWrapper struct {
	// todo: mycontent data might need to use this instead of

	Table     string          `json:"table"`
	Namespace string          `json:"namespace"`
	RefIDs    []string        `json:"ref_ids"`
	ID        string          `json:"id"`
	EventID   uint64          `json:"event_id"`
	Data      json.RawMessage `json:"data"`
	Meta      json.RawMessage `json:"meta"`
}

func parseAs[T any](payload []byte) (T, error) {
	var t T
	err := json.Unmarshal(payload, &t)
	return t, err
}

// OnUpdate updates the object using the specified committed raft entry.
func (s *chatWriterApp) OnUpdate(ctx context.Context, e raft.Entry) (raft.OnAfterApply, error) {
	// chatIdx := *s.state.ChatIndex

	payload, err := parseAs[DataWrapper](e.Value)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse command", raft.ErrUnsupported)
	}

	switch e.Command {
	case "gratis.desain.mycontent.post":
		return s.post(ctx, payload)
	case "gratis.desain.mycontent.delete":
		return s.delete(ctx, payload)
	case "gratis.desain.mycontent.subscribe":
		return s.subscribe(ctx, payload)
	}

	return nil, fmt.Errorf("%w: %v", raft.ErrUnsupported, e.Command)
}

// Apply or "Sync". The core of dragonboat's state machine "Update" function.
func (s *chatWriterApp) Apply(ctx context.Context) error {
	// save metadata
	payload, _ := json.Marshal(s.state)
	err := raft_runner.SetMetadata(ctx, appName, payload)
	if err != nil {
		return err
	}

	return nil
}

func (s *chatWriterApp) startSubscription(ctx context.Context, _ raft.Entry, rawData json.RawMessage, startIdx uint64) error {
	raftCtx := raft_runner.GetRaftContext(ctx)
	var data notifierhelper.StartSubscriptionRequest
	_ = json.Unmarshal(rawData, &data)
	err := s.topicReg.StartSubscription(raftCtx.ReplicaID, startIdx, data)
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

func getDDL(tableName string, refSize int) string {
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	_, err := buf.WriteString(`CREATE TABLE IF NOT EXISTS ` + tableName + ` (
		event_id UInt64,
		namespace String,
	`)
	if err != nil {
		log.Panic().Msgf("error write string buffer in getDDL: %v", err)
	}

	for i := 0; i < refSize; i++ {
		refID := `ref_id_` + strconv.Itoa(i+1)
		buf.WriteString(refID + ` String, ` + "\n")
	}

	_, err = buf.WriteString(`
		id String,
		data String,
		meta String,
		server_time DateTime,
		is_deleted UInt8,
		) ENGINE = ReplacingMergeTree ORDER BY (event_id);
	`) // -- consider deletion as a business event.
	if err != nil {
		log.Panic().Msgf("error write string buffer in getDDL: %v", err)
	}

	return buf.String()
}

func (s *chatWriterApp) preparePost(refSize int, eventID uint64, namespace string, refIDs []string, ID string, data string, meta string, delete bool) (string, []string, []any, []string) {
	// event id column + key columns + data & meta column
	keyAndContentSize := 1 + 1 + refSize + 1 + 2

	columns := make([]string, 0, keyAndContentSize)
	tmplt := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))

	columns = append(columns, `event_id`, `namespace`)
	args = append(args, eventID, namespace)

	for i := 0; i < refSize; i++ {
		columns = append(columns, `ref_id_`+strconv.Itoa(i+1))
		args = append(args, refIDs[i])
	}

	id := ID
	if id == "" {
		id = uuid.NewString()
	}

	columns = append(columns, `id`)
	args = append(args, id)

	if !delete {
		columns = append(columns, `data`, `meta`)
		args = append(args, data, meta)
	} else {
		columns = append(columns, `is_deleted`)
		args = append(args, 1)
	}

	columns = append(columns, `server_time`)
	args = append(args, time.Now())

	for range len(columns) {
		tmplt = append(tmplt, `?`)
	}

	return id, columns, args, tmplt
}
