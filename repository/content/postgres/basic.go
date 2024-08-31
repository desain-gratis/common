package postgres

import (
	"context"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

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

func (h *handler) Select(ctx context.Context, userID, ID string, refIDs []string) (resp []Response, err error) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	q := generateQuery(h.tableName, "SELECT", pKey, UpsertData{})
	rows, err := h.db.QueryContext(ctx, q)
	if err != nil {
		log.Err(err).Msgf("Select query failed")
		return
	}

	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		log.Err(err).Msgf("Read column failed")
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

func (h *handler) Insert(ctx context.Context, userID, ID string, refIDs []string, payloadJSON string) (err error) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	q := generateQuery(h.tableName, "INSERT", pKey, UpsertData{PayloadJSON: payloadJSON})
	_, err = h.db.ExecContext(ctx, q)
	if err != nil {
		log.Err(err).Msgf("Insert query failed")
	}

	return
}

func (h *handler) Update(ctx context.Context, userID, ID string, refIDs []string, upsertData UpsertData) (err error) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	q := generateQuery(h.tableName, "UPDATE", pKey, upsertData)
	_, err = h.db.ExecContext(ctx, q)
	if err != nil {
		log.Err(err).Msgf("Update query failed")
	}

	return
}

func (h *handler) Delete(ctx context.Context, userID, ID string, refIDs []string) (err error) {
	pKey := PrimaryKey{
		UserID: userID,
		ID:     ID,
		RefIDs: refIDs,
	}

	q := generateQuery(h.tableName, "DELETE", pKey, UpsertData{})
	_, err = h.db.ExecContext(ctx, q)
	if err != nil {
		log.Err(err).Msgf("Delete query failed")
	}

	return
}
