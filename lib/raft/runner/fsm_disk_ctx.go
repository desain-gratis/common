package runner

import (
	"context"
	"database/sql"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/lni/dragonboat/v4"
)

const (
	chConnKey        ContextKey = "clickhouse-conn"
	metadataBatchKey ContextKey = "metadata-batch"
	contextKey       ContextKey = "context-key"
)

type ContextKey string

type RaftContext struct {
	ID        string
	ShardID   uint64
	ReplicaID uint64
	Type      string
	AppConfig any
	DHost     *dragonboat.NodeHost

	// internal state
	isBootstrap bool
}

func GetClickhouseConnection(ctx context.Context) driver.Conn {
	return ctx.Value(chConnKey).(driver.Conn)
}

func GetMetadata(ctx context.Context, namespace string) ([]byte, error) {
	conn := GetClickhouseConnection(ctx)
	var payload string
	if err := conn.QueryRow(ctx, DQLReadRaftMetadata, namespace).Scan(&payload); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	return []byte(payload), nil
}

func SetMetadata(ctx context.Context, namespace string, data []byte) error {
	batch := ctx.Value(metadataBatchKey).(driver.Batch)
	return batch.Append(namespace, string(data))
}

func GetRaftContext(ctx context.Context) RaftContext {
	raftCtx := ctx.Value(contextKey).(RaftContext)
	return raftCtx
}
