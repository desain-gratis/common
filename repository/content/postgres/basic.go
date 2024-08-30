package postgres

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
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

func (h *handler) Insert(ctx context.Context, userID, ID string, refIDs []string, payloadJSON string) (err *types.CommonError) {

	return
}

func (h *handler) Get(ctx context.Context, userID, ID string, refIDs []string) (err *types.CommonError) {

	return
}
