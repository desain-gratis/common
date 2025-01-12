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

var _ content.Repository = &handler{}

type handler struct {
	db        *sqlx.DB
	tableName string
}

func New(db *sqlx.DB, tableName string) *handler {
	return &handler{
		db:        db,
		tableName: tableName,
	}
}

func (h *handler) Get(ctx context.Context, userID, ID string, refIDs []string) (resp []content.Data, err *types.CommonError) {
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

		rowValue, errMerge := mergeColumnValue(columns, values)
		if errMerge != nil {
			log.Err(errMerge).Msgf("Failed merge column & value")
			continue
		}

		resp = append(resp, rowValue)
	}
	return
}

func (h *handler) Post(ctx context.Context, userID, ID string, refIDs []string, input content.Data) (out content.Data, err *types.CommonError) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	q := generateQuery(h.tableName, "INSERT", pKey, UpsertData{PayloadJSON: input.Data})
	log.Info().Msgf("query nya adalah: %v", q)
	result, errExec := h.db.QueryContext(ctx, q)
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

	var idstr string
	for result.Next() {
		_err := result.Scan(&idstr)
		if _err != nil {
			log.Err(_err).Msgf("ERROR %v", _err)
			continue
		}
	}

	// idstr := strconv.FormatInt(id, 10)

	input.ID = idstr

	return input, nil
}

// Put(ctx context.Context, userID, ID string, refIDs []string, data Data[T]) (Data[T], *types.CommonError)

func (h *handler) Put(ctx context.Context, userID, ID string, refIDs []string, input content.Data) (out content.Data, err *types.CommonError) {
	return h.Post(ctx, userID, ID, refIDs, input)
}

func (h *handler) Delete(ctx context.Context, userID, ID string, refIDs []string) (_ content.Data, err *types.CommonError) {
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

func (h *handler) GetByID(ctx context.Context, userID, ID string) (_ content.Data, _ *types.CommonError) {
	// not used
	return
}

func (h *handler) GetByMainRefID(ctx context.Context, userID, mainRefID string) (_ []content.Data, _ *types.CommonError) {
	// not used
	return
}
