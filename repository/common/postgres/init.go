package postgres

import (
	i "github.com/desain-gratis/common/repository/common"
	"github.com/desain-gratis/common/repository/content"
	"github.com/desain-gratis/common/repository/content/postgres"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type repo[T any] struct {
	client    content.Repository[string]
	tableName string
	timeoutMs int
}

func New[T any](db *sqlx.DB, tableName string, timeoutMs int) i.Repository[T] {
	client := postgres.New[string](db, tableName)
	return &repo[T]{
		client:    client,
		tableName: tableName,
		timeoutMs: timeoutMs,
	}
}
