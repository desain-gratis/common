package clickhouse

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	types "github.com/desain-gratis/common/types/http"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

var _ content.Repository = &handler{}

type handler struct {
	db          driver.Conn
	tableName   string
	refSize     int
	addressCols []string
}

func New(db driver.Conn, tableName string, refSize int) *handler {
	refsTemplate := make([]string, refSize+2)
	for i := range refsTemplate {
		refsTemplate[i] = "?"
	}

	addressCols := make([]string, 1+refSize+1)
	addressCols[0] = "namespace"
	for i := 1; i < len(addressCols)-1; i++ {
		addressCols[i] = "ref_id_" + strconv.Itoa(i)
	}
	addressCols[len(addressCols)-1] = "id"

	dq := getDdl(tableName, refSize)

	err := db.Exec(context.Background(), dq)
	if err != nil {
		log.Err(err).Msgf("ggwp")
		log.Panic().Msgf("ggwp! %v", err)
	}

	return &handler{
		db:          db,
		tableName:   tableName,
		refSize:     refSize,
		addressCols: addressCols,
	}
}
func getDdl(tableName string, refSize int) string {
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	buf.WriteString(`CREATE TABLE IF NOT EXISTS ` + tableName + ` (
		namespace String,
`)

	key := make([]string, 0, refSize)
	key = append(key, "namespace")

	for i := 0; i < refSize; i++ {
		refID := "ref_id_" + strconv.Itoa(i+1)
		buf.WriteString(refID + " String, \n")
		key = append(key, refID)
	}

	key = append(key, "id")

	buf.WriteString(`
		id String,
		data String,
		meta String,
)
ENGINE = ReplacingMergeTree
ORDER BY (` + strings.Join(key, ",") + `) ;

	`)
	return buf.String()
}

func (h *handler) Get(ctx context.Context, namespace string, refIDs []string, ID string) (resp []content.Data, err *types.CommonError) {
	q, args, cerr := h.prepareGet(namespace, refIDs, ID)
	if cerr != nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Failed get query: " + q,
				},
			},
		}
		return
	}

	rows, cerr := h.db.Query(ctx, q, args...)
	if cerr != nil {
		log.Err(cerr).Msgf("ada apa dengan query `%v` | `%v`", q, args)
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Failed gettt query: " + q + " " + cerr.Error(),
				},
			},
		}
		return
	}

	defer rows.Close()

	for rows.Next() {
		result := make([]string, 1+h.refSize+1+2) // data and meta
		resultany := make([]any, len(result))
		for i := range result {
			resultany[i] = &result[i]
		}
		errScan := rows.Scan(resultany...)
		if errScan != nil {
			log.Err(errScan).Msgf("Failed scan row")
			continue
		}

		for i := range result {
			resultany[i] = result[i]
		}

		rowData := h.convertGetData(result)
		resp = append(resp, *rowData)
	}
	return
}

func (h *handler) Post(ctx context.Context, namespace string, refIDs []string, ID string, input content.Data) (out content.Data, err *types.CommonError) {
	if len(refIDs) != h.refSize {
		return input, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Please specify complete reference",
				},
			},
		}
	}

	if input.Meta == nil {
		input.Meta = []byte(`{}`)
	}

	id, cols, args, tmplt := h.preparePost(namespace, refIDs, ID, string(input.Data), string(input.Meta))

	q := `INSERT INTO ` + h.tableName + `(` + strings.Join(cols, ",") + `) 
	VALUES (` + strings.Join(tmplt, ",") + `);
	`

	cerr := h.db.Exec(ctx, q, args...)
	if cerr != nil {
		return input, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Err insert",
				},
			},
		}
	}

	input.ID = id

	return input, nil
}

func (h *handler) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (out content.Data, err *types.CommonError) {
	if len(refIDs) != h.refSize || ID == "" || namespace == "" || namespace == "*" {
		return content.Data{}, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Please specify complete reference",
				},
			},
		}
	}

	q, args, cerr := h.prepareGet(namespace, refIDs, ID)
	if cerr != nil {
		log.Err(cerr).Msgf("Q: %v ARGS: %v", q, args)
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Prepare get fail: " + q,
				},
			},
		}
		return
	}
	row, errQuery := h.db.Query(ctx, q, args...)
	if errQuery != nil {
		log.Err(errQuery).Msgf("err when query Q: %v ARGS: %v", q, args)
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Delete Query: " + q,
				},
			},
		}
	}
	defer row.Close()

	var data *content.Data
	for row.Next() {
		result, dest := h.allocateResultDst(true, true)
		errScan := row.Scan(dest...)
		if errScan != nil {
			log.Err(errScan).Msgf("Q: %v ARGS: %v", q, args)
			err = &types.CommonError{
				Errors: []types.Error{
					{
						HTTPCode: http.StatusInternalServerError,
						Code:     "INTERNAL_SERVER_ERROR",
						Message:  "Failed Scan: " + q,
					},
				},
			}
			return
		}
		data = h.convertGetData(result)
	}
	errRow := row.Err()
	if errRow != nil {
		log.Err(errRow).Msgf("Q: %v ARGS: %v", q, args)
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Failed erRRow: " + q,
				},
			},
		}
		return
	}

	if data == nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "BAD_REQUEST",
					Message:  "no data" + q,
				},
			},
		}
		return
	}

	whereQ, whereArgs, errWhere := h.prepareWhereQuery(namespace, refIDs, ID)
	if errWhere != nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "BAD_REQUEST",
					Message:  "no data" + q,
				},
			},
		}
		return
	}

	qDel := `DELETE FROM ` + h.tableName + ` ` + whereQ
	errDel := h.db.Exec(ctx, qDel, whereArgs...)
	if errDel != nil {
		log.Err(errDel).Msgf("q: %v. params: %v", qDel, whereArgs)
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "DELETE ERR",
					Message:  "delete err " + errDel.Error(),
				},
			},
		}
		return
	}

	return *data, nil
}

func (h *handler) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan content.Data, *types.CommonError) {
	return nil, nil
}

func (h *handler) allocateResultDst(withData, withMeta bool) ([]string, []any) {
	c := 0
	if withData {
		c++
	}
	if withMeta {
		c++
	}
	dst := make([]string, len(h.addressCols)+c)
	wrapped := make([]any, len(dst))
	for i := range len(dst) {
		wrapped[i] = &dst[i]
	}
	return dst, wrapped
}

func (h *handler) preparePost(namespace string, refIDs []string, ID string, data string, meta string) (string, []string, []any, []string) {
	columns := make([]string, 0, 1+h.refSize+1+2)
	tmplt := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))

	columns = append(columns, "namespace")
	args = append(args, namespace)

	for i := 0; i < h.refSize; i++ {
		columns = append(columns, "ref_id_"+strconv.Itoa(i+1))
		args = append(args, refIDs[i])
	}

	id := ID
	if id == "" {
		id = uuid.NewString()
	}
	columns = append(columns, "id")
	args = append(args, id)

	columns = append(columns, "data", "meta")
	args = append(args, data, meta)

	for range len(columns) {
		tmplt = append(tmplt, "?")
	}

	return id, columns, args, tmplt
}

func (h *handler) prepareGet(namespace string, refIDs []string, ID string) (string, []any, error) {
	if len(refIDs) > h.refSize {
		return "", nil, errors.New("invalid ref size")
	}

	buf := bytes.NewBuffer(make([]byte, 0, 100))

	// TODO: handle namespace = "*"

	buf.WriteString(`SELECT ` + strings.Join(append(h.addressCols, "data", "meta"), ",") + ` FROM "` + h.tableName + `" FINAL `)

	whereQ, whereArgs, err := h.prepareWhereQuery(namespace, refIDs, ID)
	if err != nil {
		return "", nil, err
	}
	buf.WriteString(whereQ)

	return buf.String(), whereArgs, nil
}

func (h *handler) prepareWhereQuery(namespace string, refIDs []string, ID string) (string, []any, error) {
	args := make([]any, 0, len(refIDs)+2)
	buf := bytes.NewBuffer(make([]byte, 0, 100))

	if namespace == "*" {
		return "", args, nil
	}
	if namespace == "" {
		return "", args, errors.New("namespace must be specified")
	}

	buf.WriteString(" WHERE namespace = ?")
	args = append(args, namespace)

	for idx, refID := range refIDs {
		buf.WriteString(" AND ")
		buf.WriteString(" ref_id_" +
			strconv.Itoa(idx+1) + " = ?",
		)
		args = append(args, refID)
	}
	if ID != "" {
		buf.WriteString(" AND id = ?")
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
