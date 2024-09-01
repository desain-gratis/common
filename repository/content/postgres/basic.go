package postgres

import (
	"context"
	"net/http"

	"github.com/desain-gratis/common/repository/content"
	types "github.com/desain-gratis/common/types/http"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

var _ content.Repository[any] = &handler[any]{}

type handler[T any] struct {
	db        *sqlx.DB
	tableName string
}

func New[T any](db *sqlx.DB, tableName string) *handler[T] {
	return &handler[T]{
		db:        db,
		tableName: tableName,
	}
}

func (h *handler[T]) Get(ctx context.Context, userID, ID string, refIDs []string) (resp []content.Data[T], err *types.CommonError) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	q := generateQuery(h.tableName, "SELECT", pKey, UpsertData{})
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

		rowValue, errMerge := mergeColumnValue[T](columns, values)
		if errMerge != nil {
			log.Err(errMerge).Msgf("Failed merge column & value")
			continue
		}

		resp = append(resp, rowValue)
	}
	return
}

func (h *handler[T]) Post(ctx context.Context, userID, ID string, refIDs []string, data content.Data[T]) (err *types.CommonError) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	payload, isRecognized := getData(data.Data)
	if !isRecognized {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusNotAcceptable,
					Code:     "NOT_ACCEPTABLE",
					Message:  "Data type is not recognized",
				},
			},
		}
		return
	}

	q := generateQuery(h.tableName, "INSERT", pKey, UpsertData{PayloadJSON: payload})
	_, errExec := h.db.ExecContext(ctx, q)
	if errExec != nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Insert query failed",
				},
			},
		}
	}

	return
}

// Put(ctx context.Context, userID, ID string, refIDs []string, data Data[T]) (Data[T], *types.CommonError)

func (h *handler[T]) Put(ctx context.Context, userID, ID string, refIDs []string, data content.Data[T]) (_ content.Data[T], err *types.CommonError) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	payload, isRecognized := getData(data.Data)
	if !isRecognized {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusNotAcceptable,
					Code:     "NOT_ACCEPTABLE",
					Message:  "Data type is not recognized",
				},
			},
		}
		return
	}

	q := generateQuery(h.tableName, "UPDATE", pKey, UpsertData{RefIDs: data.RefIDs, PayloadJSON: payload})
	_, errExec := h.db.ExecContext(ctx, q)
	if errExec != nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Update query failed: " + q,
				},
			},
		}
	}

	return
}

func (h *handler[T]) Delete(ctx context.Context, userID, ID string, refIDs []string) (_ content.Data[T], err *types.CommonError) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	q := generateQuery(h.tableName, "DELETE", pKey, UpsertData{})
	_, errExec := h.db.ExecContext(ctx, q)
	if errExec != nil {
		err = &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "INTERNAL_SERVER_ERROR",
					Message:  "Delete query failed: " + q,
				},
			},
		}
	}

	return
}

func (h *handler[T]) GetByID(ctx context.Context, userID, ID string) (_ content.Data[T], _ *types.CommonError) {
	// not used
	return
}

func (h *handler[T]) GetByMainRefID(ctx context.Context, userID, mainRefID string) (_ []content.Data[T], _ *types.CommonError) {
	// not used
	return
}
