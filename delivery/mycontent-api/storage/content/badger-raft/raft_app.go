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
var _ raft.ApplicationV2 = &badgerRaftApp{}

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

type badgerRaftApp struct {
	// state
	db            *badger.DB
	tableConfig   map[string]TableConfig
	repositoryMap map[string]content.Repository
}

type TableConfig struct {
	Name                       string
	RefSize                    int
	Versioned                  bool
	VersionedGetLimit          uint32
	VersionedUseOptimisticLock bool
}

func NewWithFile() {
	// convenience func
}

// The Raft Application
func New(db *badger.DB, tableConfig ...TableConfig) *badgerRaftApp {
	tableConfigMap := make(map[string]TableConfig)
	repositoryMap := make(map[string]content.Repository)

	for _, c := range tableConfig {
		tableConfigMap[c.Name] = c
		if c.RefSize < 0 || c.RefSize > 20 {
			log.Panic().Msgf("invalid refSize: %v", c.RefSize)
		}
		if !c.Versioned {
			repositoryMap[c.Name] = content_badger.New(db, c.Name, c.RefSize)
		} else {
			repositoryMap[c.Name] = content_badger.NewVersioned(db, c.Name, c.RefSize)
		}
	}

	return &badgerRaftApp{
		db:            db,
		tableConfig:   tableConfigMap,
		repositoryMap: repositoryMap,
	}
}

func (s *badgerRaftApp) InitV2(ctx context.Context) (uint64, error) {
	tmp := make([]byte, 0, 8)
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

// Simpler API for distributed state machine
// If return error, we will acknowledge it as applied. If you don't want, just crash the state machine.
func (s *badgerRaftApp) OnUpdateV2(ctx context.Context, e raft.EntryV2) (any, error) {
	cmd, err := parseAs[Command](e.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse command as JSON (%v)", err, string(e.Data))
	}

	repo, ok := s.repositoryMap[cmd.TableName]
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

// For external access
func (s *badgerRaftApp) GetContentRepository(ctx context.Context, tableName string) (*badgerRaftRepo, error) {
	repo, ok := s.repositoryMap[tableName]
	if !ok {
		return nil, errors.New("table not found")
	}

	// right now we just use what we have
	raftCtx, ok := raft.GetRaftContext(ctx).(*runneretcd.RaftContext)
	if !ok {
		return nil, fmt.Errorf("cannot raft maxxingo8i")
	}

	return &badgerRaftRepo{
		Repository:  repo,
		TableName:   tableName,
		raftContext: raftCtx,
	}, nil
}

func parseAs[T any](payload []byte) (T, error) {
	var t T
	err := json.Unmarshal(payload, &t)
	return t, err
}
