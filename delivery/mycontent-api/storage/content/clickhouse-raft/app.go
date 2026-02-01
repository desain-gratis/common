package clickhouseraft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
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

type QueryMyContent struct {
	Table     string   `json:"table"`
	Namespace string   `json:"namespace"`
	RefIDs    []string `json:"ref_ids"`
	ID        string   `json:"id"`
}

type QueryMyContentResponse <-chan *content.Data

type DataWrapper struct {
	// todo: mycontent data might need to use this instead of

	Table     string          `json:"table"`
	Namespace string          `json:"namespace"`
	RefIDs    []string        `json:"ref_ids"`
	ID        string          `json:"id"`
	EventID   uint64          `json:"event_id"`
	Data      json.RawMessage `json:"data,omitempty"` // todo use ref omitempty
	Meta      json.RawMessage `json:"meta,omitempty"` // omitempty
}

// happySM to isolate all business logic from the state machine technicality
// because this is an OLAP usecase,  writing to DB, choosing the appropriate DB & indexes are tightly coupled.
// will not try to abstract away
type chatWriterApp struct {
	state       *state
	topicReg    notifierhelper.TopicRegistry
	tableConfig map[string]TableConfig
}

type TableConfig struct {
	Name                  string
	RefSize               int
	IncrementalID         bool
	IncrementalIDGetLimit uint8
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

	// TODO: later we might need to use dedicated table for this
	// but now we store it in meta for simplicity
	if s.state.VersionIndexes == nil {
		s.state.VersionIndexes = make(map[string]*uint64)
	}

	return nil
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
	case QueryMyContent:
		return s.queryMyContent(ctx, q)
	}

	return nil, errors.New("unsupported query")
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

// OnUpdate updates the object using the specified committed raft entry.
func (s *chatWriterApp) OnUpdate(ctx context.Context, e raft.Entry) (raft.OnAfterApply, error) {
	// chatIdx := *s.state.ChatIndex

	payload, err := parseAs[DataWrapper](e.Value)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse command as JSON (%v)", err, string(e.Value))
	}

	switch e.Command {
	case "gratis.desain.mycontent.post":
		return s.post(ctx, payload)
	case "gratis.desain.mycontent.delete":
		return s.delete(ctx, payload)
	case "gratis.desain.mycontent.subscribe":
		return s.subscribe(ctx, payload)
	}

	return nil, fmt.Errorf("raft update %w: %v", raft.ErrUnsupported, e.Command)
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

func (s *chatWriterApp) queryMyContent(ctx context.Context, query QueryMyContent) (QueryMyContentResponse, error) {
	tableCfg, ok := s.tableConfig[query.Table]
	if !ok {
		return nil, fmt.Errorf("table not found: %v", query.Table)
	}

	conn := raft_runner.GetClickhouseConnection(ctx)
	q, args, scanFn, err := s.prepareGet(tableCfg, query, query.Namespace, query.RefIDs, query.ID)
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}

	out := make(chan *content.Data)

	go func() {
		defer close(out)
		defer rows.Close()

		for rows.Next() {
			// todo: check context done

			scanResult, scanReceiver := scanFn()

			err := rows.Scan(scanReceiver...)
			if err != nil {
				log.Err(err).Msgf("Failed scan row")
				slog.Error(
					"failed to scan row", slog.String("err", err.Error()),
					slog.String("components", "mycontent.storage.clickhouse.get"))
				continue
			}

			rowData := s.convertScanResult(scanResult)
			out <- rowData
		}
	}()

	var rq QueryMyContentResponse
	rq = (<-chan *content.Data)(out)

	return rq, nil
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
			return raft.Result{Value: 1, Data: []byte(
				fmt.Sprintf("unexpected reference count: expected %v got %v", tableCfg.RefSize, len(payload.RefIDs)),
			)}, nil
		}, nil
	}

	conn := raft_runner.GetClickhouseConnection(ctx)

	eventIdx, ok := s.state.EventIndexes[tableCfg.Name]
	if !ok {
		var index uint64
		s.state.EventIndexes[tableCfg.Name] = &index
		eventIdx = &index
	}

	const separator = "\\" // TODO: add separator validation on post / make this configurable

	combined := []string{payload.Namespace}
	combined = append(combined, payload.RefIDs...) // notice no id
	versionKey := strings.Join(combined, separator)
	versionIdx, ok := s.state.VersionIndexes[versionKey]
	if !ok {
		var index uint64
		s.state.VersionIndexes[versionKey] = &index
		versionIdx = &index
	}

	id, cols, args, tmplt := s.preparePost(
		tableCfg,
		*eventIdx,
		strconv.FormatUint(*versionIdx, 10),
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
	*s.state.VersionIndexes[versionKey]++

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

	const separator = "\\" // TODO: add separator validation on post / make this configurable

	// combined := []string{payload.Namespace}
	// combined = append(combined, payload.RefIDs...) // notice no id
	// versionKey := strings.Join(combined, separator)
	// versionIdx, ok := s.state.VersionIndexes[versionKey]
	// if !ok {
	// 	var index uint64
	// 	s.state.VersionIndexes[versionKey] = &index
	// 	versionIdx = &index
	// }

	prevData, err := s.queryMyContent(ctx, QueryMyContent{
		Table:     tableCfg.Name,
		Namespace: payload.Namespace,
		RefIDs:    payload.RefIDs,
		ID:        payload.ID,
	})

	var toDelete *content.Data
	for d := range prevData {
		toDelete = d
	}

	if toDelete == nil {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte("not found")}, nil // todo: decide behaviour (return error or not)
		}, nil
	}

	_, cols, args, tmplt := s.preparePost(
		tableCfg,
		*eventIdx,
		toDelete.ID,
		payload.Namespace,
		payload.RefIDs,
		payload.ID,
		string(payload.Data),
		string(payload.Meta),
		true,
	)

	q := `INSERT INTO ` + tableCfg.Name + `(` + strings.Join(cols, ",") + `) 
	VALUES (` + strings.Join(tmplt, ",") + `);`

	err = conn.Exec(ctx, q, args...)
	if err != nil {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error writing to table: %v", err))}, nil
		}, nil
	}

	*s.state.EventIndexes[tableCfg.Name]++
	// *s.state.VersionIndexes[versionKey]++

	encResult, err := json.Marshal(toDelete)
	if err != nil {
		return func() (raft.Result, error) {
			return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error marshal: %v", err))}, nil
		}, nil
	}

	return func() (raft.Result, error) {
		return raft.Result{Value: 0, Data: encResult}, nil
	}, nil
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

// getDDLLogType sort key only "event_id"
func getDDLLogType(tableName string, refSize int) string {
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	_, err := buf.WriteString(`CREATE TABLE IF NOT EXISTS ` + tableName + ` (
		event_id UInt64,
	`)
	if err != nil {
		log.Panic().Msgf("error write string buffer in getDDL: %v", err)
	}

	keys := []string{"namespace String"}

	for i := 0; i < refSize; i++ {
		refID := `ref_id_` + strconv.Itoa(i+1)
		buf.WriteString(refID + ` String, ` + "\n")
		keys = append(keys, refID)
	}

	keys = append(keys, "id")

	_, err = buf.WriteString(`
		` + strings.Join(keys, ", \n") + `
		data String,
		meta String,
		server_time DateTime,
		is_deleted UInt8,
		) ENGINE = ReplacingMergeTree ORDER BY (` + strings.Join(keys, ",") + `);
	`) // -- consider deletion as a business event.
	// -- consider also using ordinary merge tree, but uses  namespace, ref IDs + ref for KV access
	// OR, we can create separate table just for the head of the KV.
	// because right now I focused on the events log
	if err != nil {
		log.Panic().Msgf("error write string buffer in getDDL: %v", err)
	}

	return buf.String()
}

// getDDLLogType sort key is all my content ref (namespace, ref IDs, id) as a Key Value (KV) store
// might need to disable background merge "SYSTEM STOP MERGES db.table" if want to retain historical data
// or another implementation strategy is combined this with above log type table and do a double write
// maybe can optimize get by using the ordered event id also
func getDDL(tableName string, refSize int) string {
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	_, err := buf.WriteString(`CREATE TABLE IF NOT EXISTS ` + tableName + ` (
		event_id UInt64,
		`)
	if err != nil {
		log.Panic().Msgf("error write string buffer in getDDL: %v", err)
	}

	buf.WriteString("namespace String,\n")

	keyCols := []string{"namespace"}

	for i := 0; i < refSize; i++ {
		refID := `ref_id_` + strconv.Itoa(i+1)
		buf.WriteString(`		` + refID + " String,\n")
		keyCols = append(keyCols, refID)
	}

	keyCols = append(keyCols, "id")

	_, err = buf.WriteString(
		`		id String,
		data String,
		meta String,
		server_time DateTime,
		is_deleted UInt8
		) ENGINE = ReplacingMergeTree ORDER BY (` + strings.Join(keyCols, ", ") + `);
	`) // -- consider deletion as a business event.
	// -- consider also using ordinary merge tree, but uses  namespace, ref IDs + ref for KV access
	// OR, we can create separate table just for the head of the KV.
	// because right now I focused on the events log
	if err != nil {
		log.Panic().Msgf("error write string buffer in getDDL: %v", err)
	}

	return buf.String()
}

func (s *chatWriterApp) preparePost(tableConfig TableConfig, eventID uint64, versionID string, namespace string, refIDs []string, ID string, data string, meta string, delete bool) (string, []string, []any, []string) {
	// event id column + key columns + data & meta column
	keyAndContentSize := 1 + 1 + tableConfig.RefSize + 1 + 2

	columns := make([]string, 0, keyAndContentSize)
	tmplt := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))

	columns = append(columns, `event_id`, `namespace`)
	args = append(args, eventID, namespace)

	for i := 0; i < tableConfig.RefSize; i++ {
		columns = append(columns, `ref_id_`+strconv.Itoa(i+1))
		args = append(args, refIDs[i])
	}

	id := ID
	if id == "" {
		if tableConfig.IncrementalID {
			id = versionID
		} else {
			id = uuid.NewString()
		}
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

// todo: re-organize / move this to clickhouse to share shared query logic

// reference query:
// SELECT namespace, ref_id_1, id, t.1 AS data, t.2 AS meta, t.4 AS event_id
// FROM (
//
//	SELECT namespace, ref_id_1, id,
//	       argMax((data, meta, is_deleted, event_id), event_id) AS t
//	FROM "artifactd_build"
//	GROUP BY namespace, ref_id_1, id
//
// )
// WHERE t.3 = 0;
func (s *chatWriterApp) prepareGet(tableConfig TableConfig, query QueryMyContent, namespace string, refIDs []string, ID string) (string, []any, func() (*scanResult, []any), error) {
	if len(refIDs) > tableConfig.RefSize {
		return "", nil, nil, fmt.Errorf(
			"%w: ref size is greater than expected (got %v, expected %v)", content.ErrInvalidKey, len(refIDs), tableConfig.RefSize)
	}

	if tableConfig.IncrementalID && len(refIDs) != tableConfig.RefSize {
		// return "", nil, nil, fmt.Errorf(
		// 	"%w: reference params must be fully specified for 'IncrementalID' table", content.ErrInvalidKey)
		log.Warn().Msgf("reference params not fully specified for 'IncrementalID' table; result might be not correct because of result limit")
	}

	buf := bytes.NewBuffer(make([]byte, 0, 100))

	keyCols := make([]string, 0, tableConfig.RefSize)

	whereQ, whereArgs, err := s.prepareWhereQuery(tableConfig.RefSize, namespace, refIDs, ID)
	if err != nil {
		return "", nil, nil, err
	}

	keyCols = append(keyCols, "namespace") // event id is no longer a key

	for i := range tableConfig.RefSize {
		// TODO: reuse the one inside get DDL function
		refID := `ref_id_` + strconv.Itoa(i+1)
		keyCols = append(keyCols, refID)
	}

	keyCols = append(keyCols, "id")

	// keys + data column
	allCols := append(keyCols, "t.1 AS data", "t.2 AS meta", "t.4 AS event_id")

	_, err = buf.WriteString(
		`SELECT ` + strings.Join(allCols, ", ") + ` FROM (` +
			`SELECT ` + strings.Join(keyCols, ", ") + `, ` +
			`argMax((data, meta, is_deleted, event_id), event_id) AS t ` +
			`FROM "` + tableConfig.Name + `" ` +
			whereQ + ` ` +
			`GROUP BY ` + strings.Join(keyCols, ", ") +
			`) ` +
			`WHERE t.3 = 0`,
	)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to build query: %w", err)
	}

	// Technically, this is still Key-Value store, so this is "like" a "hack" (not necessarily actual hack)
	// Querying without a full ref ID will not be defined clearly.
	if tableConfig.IncrementalID {
		_, err = buf.WriteString(` ORDER BY event_id DESC `)

		limit := 20
		if tableConfig.IncrementalIDGetLimit > 0 {
			limit = int(tableConfig.IncrementalIDGetLimit)
		}
		_, err = fmt.Fprintf(buf, `	LIMIT %v`, limit)
	}

	return buf.String(), whereArgs, func() (*scanResult, []any) {
		sr := &scanResult{
			keys: make([]string, len(keyCols)),
		}

		scanAny := make([]any, len(allCols))

		// populate keys result receiver
		var idx int
		for range sr.keys {
			scanAny[idx] = &sr.keys[idx]
			idx++
		}

		// populate data
		// note: if sr is not a reference type, it will create new reference
		scanAny[idx] = &sr.data
		idx++

		scanAny[idx] = &sr.meta
		idx++

		scanAny[idx] = &sr.eventID
		idx++

		return sr, scanAny
	}, nil
}

type scanResult struct {
	keys    []string
	data    string
	meta    string
	eventID uint64
}

func (s *chatWriterApp) prepareWhereQuery(refSize int, namespace string, refIDs []string, ID string) (string, []any, error) {
	// keys consist of (namespace, ...refIDs, ID)
	keySize := len(refIDs) + 2

	args := make([]any, 0, keySize)
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	// if not querying all data, namespace must be specified
	if namespace == "" {
		return "", args, fmt.Errorf("%w: namespace must be specified", content.ErrInvalidKey)
	}

	// just to start the where statement..
	_, err := buf.WriteString(` WHERE 1=1 `)
	if err != nil {
		return "", nil, err
	}

	if namespace != "*" {
		_, err := buf.WriteString(` AND namespace = ?`)
		if err != nil {
			return "", nil, err
		}
		args = append(args, namespace)
	}

	for idx, refID := range refIDs {
		_, err := buf.WriteString(` AND `)
		if err != nil {
			return "", nil, err
		}

		_, err = buf.WriteString(` ref_id_` +
			strconv.Itoa(idx+1) + ` = ?`,
		)
		if err != nil {
			return "", nil, err
		}

		args = append(args, refID)
	}

	// check if it's not looking for ID
	if len(refIDs) < refSize {
		if ID != "" {
			return "", nil, fmt.Errorf(
				"%w: id provided without complete parent references", content.ErrInvalidKey)
		}
		return buf.String(), args, nil
	}

	if ID != "" {
		buf.WriteString(` AND id = ?`)
		args = append(args, ID)
	}

	return buf.String(), args, nil
}

func (s *chatWriterApp) convertScanResult(sr *scanResult) *content.Data {
	return &content.Data{
		Namespace: sr.keys[0],
		RefIDs:    sr.keys[1 : len(sr.keys)-1],
		ID:        sr.keys[len(sr.keys)-1],
		Data:      []byte(sr.data),
		Meta:      []byte(sr.meta),
		EventID:   sr.eventID,
	}
}

func parseAs[T any](payload []byte) (T, error) {
	var t T
	err := json.Unmarshal(payload, &t)
	return t, err
}
