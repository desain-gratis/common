package chdbraft

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	content_chdb "github.com/desain-gratis/common/delivery/mycontent-api/storage/content/chdb"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content/raftstorage"
	"github.com/desain-gratis/common/lib/raft"
	runneretcd "github.com/desain-gratis/common/lib/raft/runner-etcd"
)

// This is a raft application that is able to produce multiple "chdb" repositories.
// Supported table types: KV and auto-increment.
var _ raft.ApplicationV2 = &ChdbRaftApp{}

type TableType string

const (
	// TableTypeKV is a plain key-value table; the caller provides the ID.
	TableTypeKV TableType = "kv"

	// TableTypeAutoIncrement allocates an incremental ID (per namespace + refIDs)
	// when Post is called with an empty ID. A second table "<name>_latest" tracks
	// the latest ID of each branch. Posting with an ID modifies existing data;
	// the ID must be a positive integer not higher than the current latest.
	TableTypeAutoIncrement TableType = "auto-increment"
)

type TableConfig struct {
	// Name of the table. May be "database.table".
	Name      string
	RefSize   int
	TableType TableType // empty defaults to KV
}

type ChdbRaftApp struct {
	db          *sql.DB
	tableConfig map[string]TableConfig
	repos       map[string]content.Repository
}

// New creates the raft application and the backing tables.
// It returns an error instead of panicking on invalid config / DDL failure.
func New(ctx context.Context, db *sql.DB, tableConfig ...TableConfig) (*ChdbRaftApp, error) {
	tableConfigMap := make(map[string]TableConfig, len(tableConfig))
	repos := make(map[string]content.Repository, len(tableConfig))

	for _, c := range tableConfig {
		if c.RefSize < 0 || c.RefSize > 20 {
			return nil, fmt.Errorf("invalid refSize for table %q: %v", c.Name, c.RefSize)
		}
		if _, exist := tableConfigMap[c.Name]; exist {
			return nil, fmt.Errorf("duplicate table config: %q", c.Name)
		}

		if c.TableType == "" {
			c.TableType = TableTypeKV
		}

		switch c.TableType {
		case TableTypeKV:
			repo, err := content_chdb.New(ctx, db, c.Name, c.RefSize)
			if err != nil {
				return nil, fmt.Errorf("failed to create table %q: %w", c.Name, err)
			}
			repos[c.Name] = repo
		case TableTypeAutoIncrement:
			repo, err := content_chdb.NewAutoIncrement(ctx, db, c.Name, c.RefSize)
			if err != nil {
				return nil, fmt.Errorf("failed to create auto-increment table %q: %w", c.Name, err)
			}
			repos[c.Name] = repo
		default:
			return nil, fmt.Errorf("unsupported table type for table %q: %q", c.Name, c.TableType)
		}

		tableConfigMap[c.Name] = c
	}

	log.Info().Msgf("chdb raft app initialized with %d table(s)", len(repos))

	return &ChdbRaftApp{
		db:          db,
		tableConfig: tableConfigMap,
		repos:       repos,
	}, nil
}

// or "Build()"
func (s *ChdbRaftApp) GetRepository(raftInstance *runneretcd.RaftContext, tableName string) (content.Repository, bool) {
	repo, ok := s.repos[tableName]
	if !ok {
		return nil, false
	}
	return raftstorage.Wrap(raftInstance, tableName, repo), true
}

// Simpler API for distributed state machine.
// If return error, we will acknowledge it as applied. If you don't want, just crash the state machine.
func (s *ChdbRaftApp) OnUpdateV2(ctx context.Context, e raft.EntryV2) (any, error) {
	cmd, err := parseAs[raftstorage.Command](e.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse command as JSON (%v)", err, string(e.Data))
	}

	repo, ok := s.repos[cmd.TableName]
	if !ok {
		return nil, fmt.Errorf("table not found: %s", cmd.TableName)
	}

	var resp content.Data

	switch cmd.Name {
	case "delete":
		resp, err = repo.Delete(ctx, cmd.Namespace, cmd.RefIDs, cmd.ID)
	case "post":
		// IMPORTANT: the state machine must be deterministic. Without this, the
		// chdb repo generates a wall-clock event_id, which differs on every replica
		// (and on replay). Derive it from the raft log index instead.
		// For auto-increment tables the event_id also makes ID allocation
		// idempotent when an entry is re-applied after a restart.
		// NOTE: adjust `e.Index` to the actual field name on raft.EntryV2.
		cmd.Data.EventID = e.Index
		resp, err = repo.Post(ctx, cmd.Namespace, cmd.RefIDs, cmd.ID, cmd.Data)
	default:
		return nil, fmt.Errorf("unsupported command: %s", cmd.Name)
	}

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func parseAs[T any](data []byte) (T, error) {
	var t T
	if err := json.Unmarshal(data, &t); err != nil {
		return t, err
	}
	return t, nil
}
