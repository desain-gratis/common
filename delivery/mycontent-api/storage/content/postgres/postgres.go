package postgres

import (
	"context"
	"net/http"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	types "github.com/desain-gratis/common/types/http"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

var _ content.Repository = &handler{}

type handler struct {
	db        *sqlx.DB
	tableName string
	refSize   int
}

func New(db *sqlx.DB, tableName string, refSize int) *handler {
	return &handler{
		db:        db,
		tableName: tableName,
		refSize:   refSize,
	}
}

func (h *handler) Get(ctx context.Context, namespace string, refIDs []string, ID string) (resp []content.Data, err *types.CommonError) {
	pKey := PrimaryKey{
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
	}

	q, _ := generateQuery(h.tableName, "SELECT", pKey, UpsertData{})
	rows, errQuery := h.db.QueryContext(ctx, q)
	if errQuery != nil {
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
	defer rows.Close()

	columns, errColumns := rows.Columns()
	if errColumns != nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Read column failed",
				},
			},
		}
		return
	}

	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))

	for rows.Next() {
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		errScan := rows.Scan(valuePtrs...)
		if errScan != nil {
			log.Err(errScan).Msgf("Failed scan row")
			continue
		}

		rowValue, errMerge := mergeColumnValue(columns, values)
		if errMerge != nil {
			log.Err(errMerge).Msgf("Failed merge column & value")
			continue
		}

		resp = append(resp, rowValue)
	}
	return
}

func (h *handler) Post(ctx context.Context, namespace string, refIDs []string, ID string, input content.Data) (out content.Data, err *types.CommonError) {
	pKey := PrimaryKey{
		Namespace: namespace,
		ID:        ID,
		RefIDs:    refIDs,
	}

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

	q, args := generateQuery(h.tableName, "INSERT", pKey, UpsertData{Data: input.Data, Meta: input.Meta})
	rows, errExec := h.db.QueryContext(ctx, q, args...)
	if errExec != nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Update query failed: " + errExec.Error(),
				},
			},
		}
		return input, err
	}
	defer rows.Close()

	var idstr string
	for rows.Next() {
		_err := rows.Scan(&idstr)
		if _err != nil {
			log.Err(_err).Msgf("ERROR %v", _err)
			continue
		}
	}

	// idstr := strconv.FormatInt(id, 10)

	input.ID = idstr

	return input, nil
}

func (h *handler) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (out content.Data, err *types.CommonError) {
	pKey := PrimaryKey{
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
	}

	q, _ := generateQuery(h.tableName, "DELETE", pKey, UpsertData{})

	rows, errExec := h.db.QueryContext(ctx, q)
	if errExec != nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Delete query failed",
				},
			},
		}
		return content.Data{}, err
	}
	defer rows.Close()

	var id string
	var resultNamespace string
	var payload []byte

	var count int
	for rows.Next() {
		err := rows.Scan(&id, &resultNamespace, &payload)
		if err != nil {
			log.Err(err).Msgf("Err %v", err)
			break
		}
		count++
	}

	if count == 0 {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "NO_OP",
					Message:  "Deleted record does not exist",
				},
			},
		}
		return content.Data{}, err
	}

	out = content.Data{
		ID:        id,
		Data:      payload,
		Namespace: resultNamespace,
	}

	return out, nil
}

func (h *handler) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan content.Data, *types.CommonError) {
	return nil, nil
}
