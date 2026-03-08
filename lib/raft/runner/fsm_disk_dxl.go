package runner

import (
	"context"
	"fmt"
)

func DDLRaftMetadata(ctx context.Context) string {
	raftCtx, _ := GetRaftContext(ctx)
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_metadata (
	namespace String,
	data String,
)
ENGINE = ReplacingMergeTree
ORDER BY namespace;	`, raftCtx.ID)

}

// TODO: REFACTOR MAXXING

func DQLReadRaftMetadata(ctx context.Context) string {
	raftCtx, _ := GetRaftContext(ctx)
	return fmt.Sprintf(`
SELECT data FROM %s_metadata FINAL WHERE namespace=?;
	`, raftCtx.ID)
}

func DMLWriteRaftMetadata(ctx context.Context) string {
	raftCtx, _ := GetRaftContext(ctx)
	return fmt.Sprintf(`
SELECT data FROM %v_metadata FINAL WHERE namespace=?;
	`, raftCtx.ID)
}
