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

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	"github.com/desain-gratis/common/lib/raft"
	raft_runner "github.com/desain-gratis/common/lib/raft/runner"
)

const (
	appName = "chat_app"

	TopicChatLog = "chat_log"
)

var _ raft.Application = &ContentApp{}

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
type ContentApp struct {
	state       *state
	tableConfig map[string]TableConfig
}

type TableConfig struct {
	Name                       string
	RefSize                    int
	Versioned                  bool
	VersionedGetLimit          uint8
	VersionedUseOptimisticLock bool
}

func New(tableConfig ...TableConfig) *ContentApp {
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

	return &ContentApp{
		tableConfig: tableConfigMap,
	}
}

// GetRepository offers the usual mycontent.Usecase interface for code *inside* raft.Application
func (s *ContentApp) GetStorage(tableName string) (content.Repository, error) {
	tableCfg, ok := s.tableConfig[tableName]
	if !ok {
		return nil, errors.New("table not found")
	}

	return &repository{
		base:        s,
		tableConfig: tableCfg,
	}, nil
}

func (s *ContentApp) Init(ctx context.Context) error {
	conn := raft_runner.GetClickhouseConnection(ctx)

	for _, table := range s.tableConfig {
		ddl := getDDL(table.Name, table.RefSize, table.Versioned)
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

func (s *ContentApp) Lookup(ctx context.Context, query interface{}) (interface{}, error) {
	if query == nil {
		return nil, fmt.Errorf("empty query")
	}

	switch q := query.(type) {
	case QueryMyContent:
		// todo can accept limit (but later)
		return s.queryMyContent(ctx, q, 0)
	}

	return nil, errors.New("unsupported query")
}

// PrepareUpdate prepare the resources for upcoming message
func (s *ContentApp) PrepareUpdate(ctx context.Context) (context.Context, context.CancelFunc, error) {
	// conn := statemachine.GetClickhouseConnection(ctx)

	// batch, err := conn.PrepareBatch(ctx, DMLWriteChat)
	// if err != nil {
	// 	return ctx, err
	// }

	// return context.WithValue(ctx, chatTableKey, batch), nil
	return ctx, func() {}, nil
}

// OnUpdate updates the object using the specified committed raft entry.
func (s *ContentApp) OnUpdate(ctx context.Context, e raft.Entry) (raft.OnAfterApply, error) {
	// chatIdx := *s.state.ChatIndex

	payload, err := parseAs[DataWrapper](e.Value)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse command as JSON (%v)", err, string(e.Value))
	}

	switch e.Command {
	case "gratis.desain.mycontent.post": //TODO use command type
		result, err := s.post(ctx, payload)
		if err != nil {
			return nil, err
		}

		encResult, err := json.Marshal(*result)
		if err != nil {
			return func() (raft.Result, error) {
				return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error marshal: %v", err))}, nil
			}, nil
		}

		return func() (raft.Result, error) {
			return raft.Result{Value: 0, Data: encResult}, nil
		}, nil

	case "gratis.desain.mycontent.delete":
		result, err := s.delete(ctx, payload)
		if err != nil {
			return nil, err
		}

		encResult, err := json.Marshal(*result)
		if err != nil {
			return func() (raft.Result, error) {
				return raft.Result{Value: 1, Data: []byte(fmt.Sprintf("error marshal: %v", err))}, nil
			}, nil
		}

		return func() (raft.Result, error) {
			return raft.Result{Value: 0, Data: encResult}, nil
		}, nil

	case "gratis.desain.mycontent.subscribe":
		return s.subscribe(ctx, payload)
	}

	return nil, fmt.Errorf("raft update %w: %v", raft.ErrUnsupported, e.Command)
}

// Apply or "Sync". The core of dragonboat's state machine "Update" function.
func (s *ContentApp) Apply(ctx context.Context) error {
	// save metadata
	payload, err := json.Marshal(s.state)
	if err != nil {
		return err
	}

	return raft_runner.SetMetadata(ctx, appName, payload)
}

func (s *ContentApp) subscribe(_ context.Context, payload DataWrapper) (raft.OnAfterApply, error) {
	tableCfg, ok := s.tableConfig[payload.Table]
	if !ok {
		return func() (raft.Result, error) {
			errMsg := fmt.Errorf("table %v not found", payload.Table)

			return raft.Result{Value: 1, Data: []byte(errMsg.Error())}, nil
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

func (s *ContentApp) queryMyContent(ctx context.Context, query QueryMyContent, limit int) (QueryMyContentResponse, error) {
	tableCfg, ok := s.tableConfig[query.Table]
	if !ok {
		return nil, fmt.Errorf("table not found: %v", query.Table)
	}

	conn := raft_runner.GetClickhouseConnection(ctx)
	q, args, scanFn, err := s.prepareGet(tableCfg, query, query.Namespace, query.RefIDs, query.ID, limit)
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

			rowData := s.convertScanResult(scanResult, tableCfg.Versioned)
			out <- rowData
		}
	}()

	var rq QueryMyContentResponse
	rq = (<-chan *content.Data)(out)

	return rq, nil
}

func (s *ContentApp) post(ctx context.Context, payload DataWrapper) (*DataWrapper, error) {
	tableCfg, ok := s.tableConfig[payload.Table]
	if !ok {
		return nil, fmt.Errorf("table %v not found", payload.Table)
	}

	if len(payload.RefIDs) != tableCfg.RefSize {
		return nil, fmt.Errorf("unexpected reference count: expected %v got %v for table '%v'", tableCfg.RefSize, len(payload.RefIDs), tableCfg.Name)
	}

	var validate map[string]any
	err := json.Unmarshal(payload.Data, &validate)
	if err != nil {
		// opinionated
		return nil, fmt.Errorf("expected json mycontent data payload: %v", string(payload.Data))
	}

	var meta mycontent.Meta
	err = json.Unmarshal(payload.Meta, &meta)
	if err != nil {
		// opinionated
		return nil, fmt.Errorf("expected json mycontent meta payload: %v", string(payload.Meta))
	}

	const separator = "\\" // TODO: add separator validation on post / make this configurable

	combined := []string{tableCfg.Name, payload.Namespace}
	combined = append(combined, payload.RefIDs...) // notice no id
	versionKey := strings.Join(combined, separator)
	versionIdx, ok := s.state.VersionIndexes[versionKey]
	if !ok {
		var index uint64
		s.state.VersionIndexes[versionKey] = &index
		versionIdx = &index
	}

	eventIdx, ok := s.state.EventIndexes[tableCfg.Name]
	if !ok {
		var index uint64
		s.state.EventIndexes[tableCfg.Name] = &index
		eventIdx = &index
	}

	// For "new" version (my content data specified without ID) of versioned data, we support optimistic lock
	// TODO refactor
	newData := tableCfg.Versioned && payload.ID == ""
	if newData && tableCfg.VersionedUseOptimisticLock {
		if meta.OptimisticLockVersion == nil {
			if *eventIdx > 0 {
				return nil, fmt.Errorf("no optimistic lock version specified, and there is already data inside")
			}
			lockVer := uint64(0)
			meta.OptimisticLockVersion = &lockVer
		}

		if *versionIdx != *meta.OptimisticLockVersion {
			return nil, fmt.Errorf("optimistic lock version mismatch, expected %v got %v", *versionIdx, *meta.OptimisticLockVersion)
		}
	}

	// For versioned table, you can also enforce immutability here by disabling post by ID;
	// But I will not, since you can turn it off on API level;
	// This allows server side code to still modify it. (design choice)

	var id any
	var increment bool
	if tableCfg.Versioned { // TODO: REFACTOR
		if payload.ID == "" {
			id = *versionIdx
			increment = true
		} else {
			idUint, err := strconv.ParseUint(payload.ID, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid ID value for versioned table (%v): %v", tableCfg.Name, payload.ID)
			}
			id = idUint
		}
	} else {
		id = payload.ID
		if payload.ID == "" {
			id = uuid.NewString()
		}
	}

	cols, args, tmplt := s.preparePost(
		tableCfg,
		*eventIdx,
		payload.Namespace,
		payload.RefIDs,
		id,
		string(payload.Data),
		string(payload.Meta),
		false,
	)

	q := `INSERT INTO ` + tableCfg.Name + `(` + strings.Join(cols, ",") + `) 
	VALUES (` + strings.Join(tmplt, ",") + `);
	`

	conn := raft_runner.GetClickhouseConnection(ctx)

	err = conn.Exec(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("error writing to table: %v", err)
	}

	*s.state.EventIndexes[tableCfg.Name]++

	if increment { // after exec todo: refactormaxxing
		*s.state.VersionIndexes[versionKey]++
	}

	finalResult := payload

	var fID string
	if ids, ok := id.(string); ok {
		fID = ids
	}
	if ids, ok := id.(uint64); ok { // versioned todo: refactor
		fID = strconv.FormatUint(ids, 10)
	}

	finalResult.ID = fID
	finalResult.EventID = *eventIdx

	return &finalResult, nil
}

func (s *ContentApp) delete(ctx context.Context, payload DataWrapper) (*DataWrapper, error) {
	tableCfg, ok := s.tableConfig[payload.Table]
	if !ok {
		return nil, fmt.Errorf("table %v not found", payload.Table)
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
	}, 1)

	var toDelete *content.Data
	for d := range prevData {
		toDelete = d
	}

	if toDelete == nil {
		return nil, errors.New("not found")
	}

	cols, args, tmplt := s.preparePost(
		tableCfg,
		*eventIdx,
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
		return nil, fmt.Errorf("error writing to table: %v %w", tableCfg.Name, err)
	}

	*s.state.EventIndexes[tableCfg.Name]++
	// *s.state.VersionIndexes[versionKey]++

	// which is the same..
	payload.ID = toDelete.ID
	payload.EventID = toDelete.EventID

	return &payload, nil
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
func getDDL(tableName string, refSize int, incrementalID bool) string {
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

	// TODO: consider using actual delete without is_deleted column
	// TODO: refactor
	if !incrementalID {
		_, err = buf.WriteString(
			`		id String,`,
		)
	} else {
		_, err = buf.WriteString(
			`		id UInt64,`,
		)
	}
	if err != nil {
		log.Panic().Msgf("error write string buffer in getDDL: %v", err)
	}

	_, err = buf.WriteString(
		`data String,
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

func (s *ContentApp) preparePost(tableConfig TableConfig, eventID uint64, namespace string, refIDs []string, ID any, data string, meta string, delete bool) ([]string, []any, []string) {
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

	columns = append(columns, `id`)
	args = append(args, ID)

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

	return columns, args, tmplt
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
func (s *ContentApp) prepareGet(tableConfig TableConfig, _ QueryMyContent, namespace string, refIDs []string, ID any, limit int) (string, []any, func() (*scanResult, []any), error) {
	if len(refIDs) > tableConfig.RefSize {
		return "", nil, nil, fmt.Errorf(
			"%w: ref size is greater than expected (got %v, expected %v)", content.ErrInvalidKey, len(refIDs), tableConfig.RefSize)
	}

	if tableConfig.Versioned && len(refIDs) != tableConfig.RefSize {
		// TODO!!
		// return "", nil, nil, fmt.Errorf(
		// 	"%w: reference params must be fully specified for 'IncrementalID' table", content.ErrInvalidKey)
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
	if tableConfig.Versioned {
		_, err = buf.WriteString(` ORDER BY id DESC `) // (previously was event_id, just to make it consistent now is ID)

		finalLimit := 20

		if tableConfig.VersionedGetLimit > 0 {
			finalLimit = int(tableConfig.VersionedGetLimit)
		}

		if limit > 0 {
			finalLimit = int(limit)
		}

		_, err = fmt.Fprintf(buf, `	LIMIT %v`, finalLimit)
	}

	return buf.String(), whereArgs, func() (*scanResult, []any) {
		sr := &scanResult{
			keys: make([]string, len(keyCols)-1), // keyCols without ID
		}

		scanAny := make([]any, len(allCols))

		// populate keys result receiver
		var idx int
		for range sr.keys {
			scanAny[idx] = &sr.keys[idx]
			idx++
		}

		// the ID
		if !tableConfig.Versioned {
			scanAny[idx] = &sr.id
			idx++
		} else {
			scanAny[idx] = &sr.idVersioned
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
	keys []string

	id          string
	idVersioned uint64

	data    string
	meta    string
	eventID uint64
}

func (s *ContentApp) prepareWhereQuery(refSize int, namespace string, refIDs []string, ID any) (string, []any, error) {
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

	if id, ok := ID.(string); ok && id != "" {
		buf.WriteString(` AND id = ?`)
		args = append(args, id)
	} else if id, ok := ID.(uint64); ok && id == 0 {
		buf.WriteString(` AND id = ?`)
		args = append(args, id)
	}

	return buf.String(), args, nil
}

func (s *ContentApp) convertScanResult(sr *scanResult, isVersioned bool) *content.Data {
	var id string
	if isVersioned {
		id = strconv.FormatUint(sr.idVersioned, 10)
	} else {
		id = sr.id
	}

	return &content.Data{
		Namespace: sr.keys[0],
		RefIDs:    sr.keys[1:len(sr.keys)],
		ID:        id,
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
