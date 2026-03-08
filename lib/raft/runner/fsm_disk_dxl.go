package runner

import (
	"context"
	"fmt"
	"strings"
)

func DDLRaftMetadata(ctx context.Context) string {
	raftCtx, _ := GetRaftContext(ctx)
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s__metadata (
	namespace String,
	data String,
)
ENGINE = ReplacingMergeTree
ORDER BY namespace;	`, strings.ReplaceAll(raftCtx.ID, "-", "_"))

}

// TODO: REFACTOR MAXXING

func DQLReadRaftMetadata(ctx context.Context) string {
	raftCtx, _ := GetRaftContext(ctx)
	return fmt.Sprintf(`
SELECT data FROM %s__metadata FINAL WHERE namespace=?;
	`, strings.ReplaceAll(raftCtx.ID, "-", "_"))
}

func DMLWriteRaftMetadataAsync(ctx context.Context) string {
	raftCtx, _ := GetRaftContext(ctx)
	return fmt.Sprintf(
		`INSERT INTO %s__metadata (namespace, data) VALUES (?, ?)`, strings.ReplaceAll(raftCtx.ID, "-", "_"))
}
