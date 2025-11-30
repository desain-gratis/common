package clickhouse

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

var _ content.Repository = &handler{}

type handler struct {
	db        driver.Conn
	tableName string
	refSize   int
	keyCols   []string
}

func New(db driver.Conn, tableName string, refSize int) *handler {
	keySize := 1 + refSize + 1

	keyCols := make([]string, keySize)
	keyCols[0] = `namespace`
	for i := 1; i < len(keyCols)-1; i++ {
		keyCols[i] = `ref_id_` + strconv.Itoa(i)
	}
	keyCols[len(keyCols)-1] = `id`

	dq := getDdl(tableName, refSize)

	err := db.Exec(context.Background(), dq)
	if err != nil {
		panic(fmt.Sprintf("failed to execute DDL for table name: %v %v", tableName, err))
	}

	return &handler{
		db:        db,
		tableName: tableName,
		refSize:   refSize,
		keyCols:   keyCols,
	}
}

func (h *handler) Get(ctx context.Context, namespace string, refIDs []string, ID string) (resp []content.Data, err error) {
	q, args, err := h.prepareGet(namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.Query(ctx, q, args...)
	if err != nil {
		slog.Error(
			"failed to do query", slog.String("err", err.Error()),
			slog.String("components", "mycontent.storage.clickhouse.get"))
		return nil, err
	}

	defer func() {
		err := rows.Close()
		if err != nil {
			slog.Error(
				"failed to close rows", slog.String("err", err.Error()),
				slog.String("components", "mycontent.storage.clickhouse.get"))
		}
	}()

	for rows.Next() {
		keyAndContentSize := 1 + h.refSize + 1 + 2 // key + data and meta
		result := make([]string, keyAndContentSize)
		resultany := make([]any, len(result))

		for i := range result {
			resultany[i] = &result[i]
		}

		err := rows.Scan(resultany...)
		if err != nil {
			log.Err(err).Msgf("Failed scan row")
			slog.Error(
				"failed to scan row", slog.String("err", err.Error()),
				slog.String("components", "mycontent.storage.clickhouse.get"))
			continue
		}

		for i := range result {
			resultany[i] = result[i]
		}

		rowData := h.convertGetData(result)
		resp = append(resp, *rowData)
	}

	return resp, nil
}

func (h *handler) Post(ctx context.Context, namespace string, refIDs []string, ID string, input content.Data) (out content.Data, err error) {
	// TODO: move to dedicated valdiation func
	if len(refIDs) != h.refSize || ID == "" || namespace == "" || namespace == "*" {
		return content.Data{}, fmt.Errorf("%w: incomplete reference", content.ErrInvalidKey)
	}

	if input.Meta == nil {
		input.Meta = []byte(`{}`)
	}

	id, cols, args, tmplt := h.preparePost(namespace, refIDs, ID, string(input.Data), string(input.Meta))

	q := `INSERT INTO ` + h.tableName + `(` + strings.Join(cols, ",") + `) 
	VALUES (` + strings.Join(tmplt, ",") + `);
	`

	err = h.db.Exec(ctx, q, args...)
	if err != nil {
		return content.Data{}, err
	}

	input.ID = id

	return input, nil
}

func (h *handler) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (out content.Data, err error) {
	// TODO: move to dedicated valdiation func
	if len(refIDs) != h.refSize || ID == "" || namespace == "" || namespace == "*" {
		return content.Data{}, fmt.Errorf("%w: incomplete reference", content.ErrInvalidKey)
	}

	q, args, err := h.prepareGet(namespace, refIDs, ID)
	if err != nil {
		return content.Data{}, err
	}

	row, err := h.db.Query(ctx, q, args...)
	if err != nil {
		return content.Data{}, err
	}

	defer func() {
		err := row.Close()
		if err != nil {
			slog.Error(
				"failed to close row", slog.String("err", err.Error()),
				slog.String("components", "mycontent.storage.clickhouse.delete"))
		}
	}()

	var data *content.Data
	for row.Next() {
		result, dest := h.allocateResultDst(true, true)
		err := row.Scan(dest...)
		if err != nil {
			slog.Warn(
				"failed to scan row. skipping", slog.String("err", err.Error()),
				slog.String("components", "mycontent.storage.clickhouse.get"))
			continue
		}
		data = h.convertGetData(result)
	}

	err = row.Err()
	if err != nil {
		return content.Data{}, err
	}

	if data == nil {
		return content.Data{}, fmt.Errorf("%w: content not found for namespace=%v refIDs=%v and ID=%v",
			content.ErrNotFound, namespace, refIDs, ID)
	}

	whereQ, whereArgs, err := h.prepareWhereQuery(namespace, refIDs, ID)
	if err != nil {
		return content.Data{}, err
	}

	qDel := `DELETE FROM ` + h.tableName + ` ` + whereQ
	err = h.db.Exec(ctx, qDel, whereArgs...)
	if err != nil {
		return content.Data{}, err
	}

	return *data, nil
}

func (h *handler) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (data <-chan content.Data, err error) {
	q, args, err := h.prepareGet(namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.Query(ctx, q, args...)
	if err != nil {
		slog.Error(
			"failed to do query", slog.String("err", err.Error()),
			slog.String("components", "mycontent.storage.clickhouse.get"))
		return nil, err
	}

	output := make(chan content.Data)

	go func() {
		defer close(output)
		defer func() {
			err := rows.Close()
			if err != nil {
				slog.Error(
					"failed to close rows", slog.String("err", err.Error()),
					slog.String("components", "mycontent.storage.clickhouse.get"))
			}
		}()

		for rows.Next() {
			keyAndContentSize := 1 + h.refSize + 1 + 2 // key + data and meta
			result := make([]string, keyAndContentSize)
			resultany := make([]any, len(result))

			for i := range result {
				resultany[i] = &result[i]
			}

			err := rows.Scan(resultany...)
			if err != nil {
				log.Err(err).Msgf("Failed scan row")
				slog.Error(
					"failed to scan row", slog.String("err", err.Error()),
					slog.String("components", "mycontent.storage.clickhouse.get"))
				continue
			}

			for i := range result {
				resultany[i] = result[i]
			}

			rowData := h.convertGetData(result)
			output <- *rowData
		}
	}()

	return output, nil
}

func (h *handler) allocateResultDst(withData, withMeta bool) ([]string, []any) {
	c := 0
	if withData {
		c++
	}
	if withMeta {
		c++
	}

	dst := make([]string, len(h.keyCols)+c)
	wrapped := make([]any, len(dst))
	for i := range len(dst) {
		wrapped[i] = &dst[i]
	}

	return dst, wrapped
}

func (h *handler) preparePost(namespace string, refIDs []string, ID string, data string, meta string) (string, []string, []any, []string) {
	// key columns + data & meta column
	keyAndContentSize := 1 + h.refSize + 1 + 2

	columns := make([]string, 0, keyAndContentSize)
	tmplt := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))

	columns = append(columns, `namespace`)
	args = append(args, namespace)

	for i := 0; i < h.refSize; i++ {
		columns = append(columns, `ref_id_`+strconv.Itoa(i+1))
		args = append(args, refIDs[i])
	}

	id := ID
	if id == "" {
		id = uuid.NewString()
	}

	columns = append(columns, `id`)
	args = append(args, id)

	columns = append(columns, `data`, `meta`)
	args = append(args, data, meta)

	for range len(columns) {
		tmplt = append(tmplt, `?`)
	}

	return id, columns, args, tmplt
}

func (h *handler) prepareGet(namespace string, refIDs []string, ID string) (string, []any, error) {
	if len(refIDs) > h.refSize {
		return "", nil, fmt.Errorf(
			"%w: ref size is greater than expected (got %v, expected %v)", content.ErrInvalidKey, len(refIDs), h.refSize)
	}

	buf := bytes.NewBuffer(make([]byte, 0, 100))

	_, err := buf.WriteString(`SELECT ` + strings.Join(append(h.keyCols, "data", "meta"), ",") + ` FROM "` + h.tableName + `" FINAL `)
	if err != nil {
		return "", nil, err
	}

	whereQ, whereArgs, err := h.prepareWhereQuery(namespace, refIDs, ID)
	if err != nil {
		return "", nil, err
	}

	_, err = buf.WriteString(whereQ)
	if err != nil {
		return "", nil, err
	}

	return buf.String(), whereArgs, nil
}

func (h *handler) prepareWhereQuery(namespace string, refIDs []string, ID string) (string, []any, error) {
	// keys consist of (namespace, ...refIDs, ID)
	keySize := len(refIDs) + 2

	args := make([]any, 0, keySize)
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	// query all data
	if namespace == "*" {
		return "", args, nil
	}

	// if not querying all data, namespace must be specified
	if namespace == "" {
		return "", args, fmt.Errorf("%w: namespace must be specified", content.ErrInvalidKey)
	}

	_, err := buf.WriteString(` WHERE namespace = ?`)
	if err != nil {
		return "", nil, err
	}
	args = append(args, namespace)

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
	if len(refIDs) < h.refSize {
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

func (h *handler) convertGetData(result []string) *content.Data {
	refIDs := make([]string, 0, h.refSize)

	for i := 1; i < len(result)-2; i++ {
		refIDs = append(refIDs, result[i])
	}

	return &content.Data{
		Namespace: result[0],
		RefIDs:    refIDs,
		Data:      []byte(result[len(result)-2]),
		Meta:      []byte(result[len(result)-1]),
	}
}

func getDdl(tableName string, refSize int) string {
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	buf.WriteString(`CREATE TABLE IF NOT EXISTS ` + tableName + ` (
		namespace String,
`)

	key := make([]string, 0, refSize)
	key = append(key, `namespace`)

	for i := 0; i < refSize; i++ {
		refID := `ref_id_` + strconv.Itoa(i+1)
		buf.WriteString(refID + ` String, ` + "\n")
		key = append(key, refID)
	}

	key = append(key, `id`)

	buf.WriteString(`
		id String,
		data String,
		meta String,
) ENGINE = ReplacingMergeTree ORDER BY (` + strings.Join(key, ", ") + `) ;`)

	return buf.String()
}
