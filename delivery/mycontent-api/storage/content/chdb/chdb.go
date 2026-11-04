package chdb

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var _ content.Repository = (*handler)(nil)

var tableNameRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// handler implements content.Repository on top of database/sql (CHDB).
//
// Storage model is an append-only event log (like the scheduler sample):
// every Post inserts a new row with a monotonically increasing event_id,
// and every read returns only the latest event per key
// (namespace, ref_id_1..N, id) using `ORDER BY event_id DESC LIMIT 1 BY ...`.
type handler struct {
	db        *sql.DB
	tableName string
	refSize   int
	keyCols   []string // namespace, ref_id_1..N, id

	lastEventID atomic.Uint64
}

// New creates the table (if missing) and returns the repository.
func New(ctx context.Context, db *sql.DB, tableName string, refSize int) (*handler, error) {
	if !tableNameRegex.MatchString(tableName) {
		return nil, fmt.Errorf("invalid table name: %q", tableName)
	}
	if refSize < 0 {
		return nil, fmt.Errorf("invalid ref size: %d", refSize)
	}

	keyCols := make([]string, 0, refSize+2)
	keyCols = append(keyCols, "namespace")
	for i := 1; i <= refSize; i++ {
		keyCols = append(keyCols, "ref_id_"+strconv.Itoa(i))
	}
	keyCols = append(keyCols, "id")

	h := &handler{
		db:        db,
		tableName: tableName,
		refSize:   refSize,
		keyCols:   keyCols,
	}

	if err := h.init(ctx); err != nil {
		return nil, fmt.Errorf("failed to execute DDL for table %v: %w", tableName, err)
	}

	return h, nil
}

func (h *handler) init(ctx context.Context) error {
	if db, _, ok := strings.Cut(h.tableName, "."); ok {
		if _, err := h.db.ExecContext(ctx, `CREATE DATABASE IF NOT EXISTS `+db); err != nil {
			return err
		}
	}

	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS ")
	b.WriteString(h.tableName)
	b.WriteString(" (\n")
	b.WriteString("\tevent_id UInt64,\n")
	for _, c := range h.keyCols {
		b.WriteString("\t")
		b.WriteString(c)
		b.WriteString(" String,\n")
	}
	b.WriteString("\tdata String,\n")
	b.WriteString("\tmeta String\n")
	b.WriteString(") ENGINE = MergeTree ORDER BY (")
	b.WriteString(strings.Join(h.keyCols, ", "))
	b.WriteString(", event_id)")

	_, err := h.db.ExecContext(ctx, b.String())
	return err
}

// nextEventID returns a strictly increasing id (time based, so it also
// survives restarts as long as the clock does not go backwards).
func (h *handler) nextEventID() uint64 {
	for {
		now := uint64(time.Now().UnixNano())
		last := h.lastEventID.Load()
		next := now
		if next <= last {
			next = last + 1
		}
		if h.lastEventID.CompareAndSwap(last, next) {
			return next
		}
	}
}

func (h *handler) validateFullKey(namespace string, refIDs []string, ID string) error {
	if len(refIDs) != h.refSize || ID == "" || namespace == "" || namespace == "*" {
		return fmt.Errorf("%w: incomplete reference", mycontent.ErrInvalidKey)
	}
	return nil
}

func (h *handler) Post(ctx context.Context, namespace string, refIDs []string, ID string, input content.Data) (content.Data, error) {
	if err := h.validateFullKey(namespace, refIDs, ID); err != nil {
		return content.Data{}, err
	}

	meta := input.Meta
	if meta == nil {
		meta = []byte(`{}`)
	}

	eventID := input.EventID
	if eventID == 0 {
		eventID = h.nextEventID()
	}

	cols := make([]string, 0, len(h.keyCols)+3)
	args := make([]any, 0, len(h.keyCols)+3)

	cols = append(cols, "event_id")
	args = append(args, eventID)

	cols = append(cols, "namespace")
	args = append(args, namespace)

	for i, r := range refIDs {
		cols = append(cols, "ref_id_"+strconv.Itoa(i+1))
		args = append(args, r)
	}

	cols = append(cols, "id", "data", "meta")
	args = append(args, ID, string(input.Data), string(meta))

	q := `INSERT INTO ` + h.tableName + ` (` + strings.Join(cols, ", ") + `) VALUES (` +
		strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + `)`

	if _, err := h.db.ExecContext(ctx, q, args...); err != nil {
		return content.Data{}, err
	}

	input.EventID = eventID
	input.Namespace = namespace
	input.RefIDs = refIDs
	input.ID = ID
	input.Meta = meta

	return input, nil
}

func (h *handler) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]content.Data, error) {
	q, args, err := h.prepareGet(namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.QueryContext(ctx, q, args...)
	if err != nil {
		slog.Error("failed to do query", slog.String("err", err.Error()),
			slog.String("components", "mycontent.storage.chdb.get"))
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", slog.String("err", err.Error()),
				slog.String("components", "mycontent.storage.chdb.get"))
		}
	}()

	resp := make([]content.Data, 0)
	for rows.Next() {
		d, err := h.scanRow(rows)
		if err != nil {
			slog.Error("failed to scan row", slog.String("err", err.Error()),
				slog.String("components", "mycontent.storage.chdb.get"))
			continue
		}
		resp = append(resp, *d)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return resp, nil
}

func (h *handler) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan content.Data, error) {
	q, args, err := h.prepareGet(namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.QueryContext(ctx, q, args...)
	if err != nil {
		slog.Error("failed to do query", slog.String("err", err.Error()),
			slog.String("components", "mycontent.storage.chdb.stream"))
		return nil, err
	}

	output := make(chan content.Data)

	go func() {
		defer close(output)
		defer func() {
			if err := rows.Close(); err != nil {
				slog.Error("failed to close rows", slog.String("err", err.Error()),
					slog.String("components", "mycontent.storage.chdb.stream"))
			}
		}()

		for rows.Next() {
			d, err := h.scanRow(rows)
			if err != nil {
				slog.Error("failed to scan row", slog.String("err", err.Error()),
					slog.String("components", "mycontent.storage.chdb.stream"))
				continue
			}

			select {
			case output <- *d:
			case <-ctx.Done():
				return
			}
		}

		if err := rows.Err(); err != nil {
			slog.Error("rows error", slog.String("err", err.Error()),
				slog.String("components", "mycontent.storage.chdb.stream"))
		}
	}()

	return output, nil
}

func (h *handler) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (content.Data, error) {
	if err := h.validateFullKey(namespace, refIDs, ID); err != nil {
		return content.Data{}, err
	}

	existing, err := h.Get(ctx, namespace, refIDs, ID)
	if err != nil {
		return content.Data{}, err
	}
	if len(existing) == 0 {
		return content.Data{}, fmt.Errorf("%w: content not found for namespace=%v refIDs=%v and ID=%v",
			mycontent.ErrNotFound, namespace, refIDs, ID)
	}

	whereQ, whereArgs, err := h.prepareWhere(namespace, refIDs, ID)
	if err != nil {
		return content.Data{}, err
	}

	// Lightweight delete: removes every event (version) of this key.
	if _, err := h.db.ExecContext(ctx, `DELETE FROM `+h.tableName+` `+whereQ, whereArgs...); err != nil {
		return content.Data{}, err
	}

	return existing[0], nil
}

// prepareGet returns only the latest event for each key.
func (h *handler) prepareGet(namespace string, refIDs []string, ID string) (string, []any, error) {
	whereQ, args, err := h.prepareWhere(namespace, refIDs, ID)
	if err != nil {
		return "", nil, err
	}

	keys := strings.Join(h.keyCols, ", ")

	var buf bytes.Buffer
	buf.WriteString(`SELECT event_id, ` + keys + `, data, meta FROM (`)
	buf.WriteString(` SELECT * FROM ` + h.tableName + ` ` + whereQ)
	buf.WriteString(` ORDER BY event_id DESC LIMIT 1 BY ` + keys)
	buf.WriteString(`) ORDER BY ` + keys)

	return buf.String(), args, nil
}

func (h *handler) prepareWhere(namespace string, refIDs []string, ID string) (string, []any, error) {
	if namespace == "" {
		return "", nil, fmt.Errorf("%w: namespace must be specified", mycontent.ErrInvalidKey)
	}
	if len(refIDs) > h.refSize {
		return "", nil, fmt.Errorf(
			"%w: ref size is greater than expected (got %v, expected %v)",
			mycontent.ErrInvalidKey, len(refIDs), h.refSize)
	}
	if len(refIDs) < h.refSize && ID != "" {
		return "", nil, fmt.Errorf(
			"%w: id provided without complete parent references", mycontent.ErrInvalidKey)
	}

	args := make([]any, 0, len(refIDs)+2)
	var buf strings.Builder
	buf.WriteString(` WHERE 1 = 1`)

	if namespace != "*" {
		buf.WriteString(` AND namespace = ?`)
		args = append(args, namespace)
	}

	for i, r := range refIDs {
		buf.WriteString(` AND ref_id_` + strconv.Itoa(i+1) + ` = ?`)
		args = append(args, r)
	}

	if ID != "" {
		buf.WriteString(` AND id = ?`)
		args = append(args, ID)
	}

	return buf.String(), args, nil
}

// scanRow scans: event_id, namespace, ref_id_1..N, id, data, meta
func (h *handler) scanRow(rows *sql.Rows) (*content.Data, error) {
	var eventID uint64
	keys := make([]string, len(h.keyCols))
	var data, meta string

	dst := make([]any, 0, len(keys)+3)
	dst = append(dst, &eventID)
	for i := range keys {
		dst = append(dst, &keys[i])
	}
	dst = append(dst, &data, &meta)

	if err := rows.Scan(dst...); err != nil {
		return nil, err
	}

	refIDs := make([]string, 0, h.refSize)
	refIDs = append(refIDs, keys[1:len(keys)-1]...)

	return &content.Data{
		EventID:   eventID,
		Namespace: keys[0],
		RefIDs:    refIDs,
		ID:        keys[len(keys)-1],
		Data:      []byte(data),
		Meta:      []byte(meta),
	}, nil
}
