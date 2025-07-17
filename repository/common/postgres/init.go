package postgres

import (
	content "github.com/desain-gratis/common/delivery/mycontent-api"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/postgres"
	i "github.com/desain-gratis/common/repository/common"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type repo[T any] struct {
	client    content.Repository
	tableName string
	timeoutMs int
}

func New[T any](db *sqlx.DB, tableName string, refSize int, timeoutMs int) i.Repository[T] {
	client := postgres.New(db, tableName, refSize)
	return &repo[T]{
		client:    client,
		tableName: tableName,
		timeoutMs: timeoutMs,
	}
}
