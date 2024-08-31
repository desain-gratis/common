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
	timeoutMs int
}

func New(db *sqlx.DB, tableName string, timeoutMs int) *handler {
	return &handler{
		db:        db,
		tableName: tableName,
		timeoutMs: timeoutMs,
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

	// result = make(map[string]int)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		for i := range values {
			values[i] = new(interface{})
		}

		errScan := rows.Scan(values...)
		if errScan != nil {
			continue
		}

		rowValue, errMerge := mergeColumnValue(columns, values)
		if errMerge != nil {
			log.Err(err).Msgf("Failed merge column & value")
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

	q := generateQuery(h.tableName, "Delete", pKey, UpsertData{})
	_, err = h.db.ExecContext(ctx, q)
	if err != nil {
		log.Err(err).Msgf("Delete query failed")
	}

	return
}
