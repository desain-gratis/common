package badgerraft

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	content_badger "github.com/desain-gratis/common/delivery/mycontent-api/storage/content/badger"
	"github.com/desain-gratis/common/lib/raft"
	runneretcd "github.com/desain-gratis/common/lib/raft/runner-etcd"
)

const (
	appName = "chat_app"

	TopicChatLog = "chat_log"
)

// This is a raft application that able to produce multiple "badger" repository
var _ raft.ApplicationV2 = &BadgerRaftApp{}

type QueryMyContent struct {
	Table     string   `json:"table"`
	Namespace string   `json:"namespace"`
	RefIDs    []string `json:"ref_ids"`
	ID        string   `json:"id"`
}

type QueryMyContentResponse <-chan *content.Data

type DataWrapper struct {
	// todo: mycontent data might need to use this instead of

	Table     string          `json:"table"`
	Namespace string          `json:"namespace"`
	RefIDs    []string        `json:"ref_ids"`
	ID        string          `json:"id"`
	EventID   uint64          `json:"event_id"`
	Data      json.RawMessage `json:"data,omitempty"` // todo use ref omitempty
	Meta      json.RawMessage `json:"meta,omitempty"` // omitempty
}

type BadgerRaftApp struct {
	// state
	db                 *badger.DB
	tableConfig        map[string]TableConfig
	repos              map[string]*content_badger.BadgerRepo // or KV
	versionedRepos     map[string]*content_badger.VersionedBadgerRepo
	autoIncrementRepos map[string]*content_badger.AutoIncrementBadgerRepo
	// to avoid confusion, might need to add auto increment repository
}

// TODO: integrate as mycontent itself, so we can configure it via entity (eg. GetTableType(...))
type TableType string

const (
	TableTypeKV            TableType = "kv"
	TableTypeVersioned     TableType = "versioned"
	TableTypeAutoIncrement TableType = "auto-increment"
)

type TableConfig struct {
	Name      string
	RefSize   int
	TableType TableType
	// Versioned                  bool
	VersionedGetLimit          uint32
	VersionedUseOptimisticLock bool
}

func NewWithFile() {
	// convenience func
}

// The Raft Application
func New(db *badger.DB, tableConfig ...TableConfig) *BadgerRaftApp {
	tableConfigMap := make(map[string]TableConfig)

	repos := make(map[string]*content_badger.BadgerRepo)
	versionedRepos := make(map[string]*content_badger.VersionedBadgerRepo)
	autoIncrementRepos := make(map[string]*content_badger.AutoIncrementBadgerRepo)

	for _, c := range tableConfig {
		tableConfigMap[c.Name] = c
		if c.RefSize < 0 || c.RefSize > 20 {
			log.Panic().Msgf("invalid refSize: %v", c.RefSize)
		}
		switch c.TableType {
		case TableTypeVersioned:
			versionedRepos[c.Name], _ = content_badger.NewVersioned(db, c.Name, c.RefSize)
		case TableTypeAutoIncrement:
			autoIncrementRepos[c.Name] = content_badger.NewAutoIncrement(db, c.Name, c.RefSize)
		case TableTypeKV:
		default:
			repos[c.Name] = content_badger.New(db, c.Name, c.RefSize)
		}
	}

	return &BadgerRaftApp{
		db:                 db,
		tableConfig:        tableConfigMap,
		repos:              repos,
		versionedRepos:     versionedRepos,
		autoIncrementRepos: autoIncrementRepos,
	}
}

func (s *BadgerRaftApp) InitV2(ctx context.Context) (uint64, error) {
	tmp := make([]byte, 8)
	err := s.db.View(func(txn *badger.Txn) error {
		// todo: make the delete/post inside here as well.. later
		value, err := txn.Get([]byte("last-applied-index"))
		if err != nil {
			return err
		}

		_, err = value.ValueCopy(tmp)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return 0, nil
		}
		return 0, err
	}

	lastAppliedIndex := binary.BigEndian.Uint64(tmp[:])

	return lastAppliedIndex, nil
}

func (s *BadgerRaftApp) getRepository(tablelName string) (content.Repository, bool) {
	if repo, ok := s.repos[tablelName]; ok {
		return repo, true
	}

	if repo, ok := s.autoIncrementRepos[tablelName]; ok {
		return repo, true
	}

	if repo, ok := s.versionedRepos[tablelName]; ok {
		return repo, true
	}

	return nil, false
}

// Simpler API for distributed state machine
// If return error, we will acknowledge it as applied. If you don't want, just crash the state machine.
func (s *BadgerRaftApp) OnUpdateV2(ctx context.Context, e raft.EntryV2) (any, error) {
	cmd, err := parseAs[Command](e.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse command as JSON (%v)", err, string(e.Data))
	}

	repo, ok := s.getRepository(cmd.TableName)
	if !ok {
		return nil, fmt.Errorf("table not found: %s", cmd.TableName)
	}

	var resp content.Data

	switch cmd.Name {
	case "delete":
		resp, err = repo.Delete(ctx, cmd.Namespace, cmd.RefIDs, cmd.ID)
	case "post":
		resp, err = repo.Post(ctx, cmd.Namespace, cmd.RefIDs, cmd.ID, cmd.Data)
	default:
		return nil, fmt.Errorf("unsupported command: %s", cmd.Name)
	}

	if err != nil {
		return nil, err
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], e.Index)

	err = s.db.Update(func(txn *badger.Txn) error {
		// todo: make the delete/post inside here as well.. later
		return txn.Set([]byte("last-applied-index"), buf[:])
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *BadgerRaftApp) GetKVTable(ctx context.Context, tableName string) (*badgerRaftRepo, error) {
	return s.GetContentRepository(ctx, TableTypeKV, tableName)
}

func (s *BadgerRaftApp) GetAutoIncrementTable(ctx context.Context, tableName string) (*badgerRaftRepo, error) {
	return s.GetContentRepository(ctx, TableTypeAutoIncrement, tableName)
}

// TODOO: MAKE IT MORE CLEAN
// For external access
// Todo: rename to GetRaftRepository or GetRaftEnabledRepository
func (s *BadgerRaftApp) GetContentRepository(ctx context.Context, tableType TableType, tableName string) (*badgerRaftRepo, error) {

	// right now we just use what we have
	raftCtx, ok := raft.GetRaftContext(ctx).(*runneretcd.RaftContext)
	if !ok {
		return nil, fmt.Errorf("cannot raft maxxing")
	}

	switch tableType {
	case TableTypeKV:
		repo, ok := s.repos[tableName]
		if !ok {
			return nil, fmt.Errorf("table not found: %s", tableName)
		}
		return &badgerRaftRepo{
			Repository:  repo,
			TableName:   tableName,
			raftContext: raftCtx,
		}, nil
	case TableTypeAutoIncrement:
		repo, ok := s.autoIncrementRepos[tableName]
		if !ok {
			return nil, fmt.Errorf("table not found: %s", tableName)
		}
		return &badgerRaftRepo{
			Repository:  repo,
			TableName:   tableName,
			raftContext: raftCtx,
		}, nil
	}

	return nil, fmt.Errorf("table not foound %s (type: %s)", tableName, tableType)
}

func parseAs[T any](payload []byte) (T, error) {
	var t T
	err := json.Unmarshal(payload, &t)
	return t, err
}
