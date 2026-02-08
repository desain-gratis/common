package runner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/desain-gratis/common/lib/raft"
	"github.com/rs/zerolog/log"

	sm "github.com/lni/dragonboat/v4/statemachine"
)

type baseDiskSM struct {
	lastApplied    uint64
	conn           driver.Conn
	closed         bool
	smMetadata     *Metadata
	initialApplied uint64
	app            raft.Application
	database       string
	clickhouseAddr string
	raftContext    RaftContext
}

func newBaseDiskSM(address, database string, app raft.Application) func(shardID uint64, replicaID uint64) sm.IOnDiskStateMachine {
	return func(shardID uint64, replicaID uint64) sm.IOnDiskStateMachine {
		return &baseDiskSM{
			clickhouseAddr: address,
			database:       database,
			app:            app,
			raftContext:    RaftContext{ShardID: shardID, ReplicaID: replicaID},
		}
	}
}

// Open opens the state machine and return the index of the last Raft Log entry
// already updated into the state machine.
func (d *baseDiskSM) Open(stopc <-chan struct{}) (uint64, error) {
	ctx := context.Background()

	d.conn = Connect(d.clickhouseAddr, d.database)

	ctx = context.WithValue(ctx, chConnKey, d.conn)

	err := prepareSchema(ctx, d.conn)
	if err != nil {
		log.Fatal().Msgf("failed to prepare schema: %v err: %v", d.clickhouseAddr, err)
	}

	metadata, err := d.loadMetadata(ctx)
	if err != nil {
		log.Fatal().Msgf("failed to load metadata to clickhouse: %v err: %v", d.clickhouseAddr, err)
	}

	d.smMetadata = metadata
	d.initialApplied = *metadata.AppliedIndex
	d.lastApplied = *metadata.AppliedIndex

	err = d.app.Init(ctx)
	if err != nil {
		log.Fatal().Msgf("failed to init app err: %v", err)
	}

	return d.lastApplied, nil
}

// Lookup queries the state machine.
func (d *baseDiskSM) Lookup(key interface{}) (interface{}, error) {
	// Inject with context
	ctx := context.WithValue(context.Background(), chConnKey, d.conn)
	ctx = context.WithValue(ctx, contextKey, d.raftContext)

	return d.app.Lookup(ctx, key)
}

// Raft Command
type Command struct {
	Command raft.Command    `json:"command"`
	Value   json.RawMessage `json:"value"`

	// ReplicaID of the requester
	ReplicaID *uint64 `json:"replica_id,omitempty"`
}

func (d *baseDiskSM) Update(ents []sm.Entry) ([]sm.Entry, error) {
	ctx := context.WithValue(context.Background(), chConnKey, d.conn)
	ctx = context.WithValue(ctx, contextKey, d.raftContext)

	metadataBatch, err := d.conn.PrepareBatch(ctx, `INSERT INTO metadata (namespace, data)`)
	if err != nil {
		log.Panic().Msgf("failed to apply metadata batch: %v", err)
	}

	ctx = context.WithValue(ctx, metadataBatchKey, metadataBatch)

	ctx, cleanup, err := d.app.PrepareUpdate(ctx)
	if err != nil {
		log.Panic().Msgf("failed to prepare for update: %v", err)
	}
	defer cleanup()

	// Process message one-by-one
	afterApplys := make([]raft.OnAfterApply, len(ents))
	for idx := range ents {
		if ents[idx].Index <= d.initialApplied {
			log.Panic().Msgf("oh no initial")
		}
		var msg Command
		err := json.Unmarshal(ents[idx].Cmd, &msg)
		if err != nil {
			afterApplys[idx] = func() (raft.Result, error) {
				return raft.Result{Value: 1, Data: []byte("invalid message: not a valid JSON")}, nil
			}
			continue
		}

		afterApplys[idx], err = d.app.OnUpdate(ctx, raft.Entry{
			Entry:     &ents[idx],
			Index:     ents[idx].Index,
			Command:   raft.Command(msg.Command),
			Value:     []byte(msg.Value),
			ReplicaID: msg.ReplicaID,
		})
		if err != nil && errors.Is(err, raft.ErrUnsupported) {
			afterApplys[idx] = func() (raft.Result, error) {
				return raft.Result{Value: 1, Data: []byte(err.Error())}, nil
			}
			continue
		}
		if err != nil {
			afterApplys[idx] = func() (raft.Result, error) {
				return raft.Result{Value: 1, Data: []byte(err.Error())}, nil
			}
			continue
		}
	}

	// Apply update to disk
	err = d.app.Apply(ctx)
	if err != nil {
		log.Panic().Msgf("failed to apply: %v", err)
	}

	*d.smMetadata.AppliedIndex = ents[len(ents)-1].Index

	err = d.saveMetadata(metadataBatch)
	if err != nil {
		log.Panic().Msgf("base save metadata failed %v", err)
	}

	err = metadataBatch.Send()
	if err != nil {
		log.Panic().Msgf("base save metadata failed send %v", err)
	}

	err = metadataBatch.Close()
	if err != nil {
		log.Panic().Msgf("base save metadata failed close %v", err)
	}

	// Execute function after successful apply
	for idx := range ents {
		res, err := afterApplys[idx]()
		if err != nil {
			continue
		}
		ents[idx].Result = sm.Result(res)
	}

	return ents, nil
}

// Sync synchronizes all in-core state of the state machine. Since the Update
// method in this example already does that every time when it is invoked, the
// Sync method here is a NoOP.
func (d *baseDiskSM) Sync() error {
	return nil
}

func (d *baseDiskSM) PrepareSnapshot() (interface{}, error) {
	if d.closed {
		panic("prepare snapshot called after Close()")
	}

	// if it's KV, can freeze all table.. & freeze all metadata up to the current applied index (for other node to join)
	//    ALTER TABLE <database_name>.<table_name> FREEZE [PARTITION partition_expr] [WITH NAME 'backup_name']

	return nil, nil
}

// SaveSnapshot saves the state machine state identified by the state
// identifier provided by the input ctx parameter. Note that SaveSnapshot
// is not suppose to save the latest state.
func (d *baseDiskSM) SaveSnapshot(ctx interface{},
	w io.Writer, done <-chan struct{}) error {
	if d.closed {
		panic("prepare snapshot called after Close()")
	}

	// TODO: read from context all the table that is frozen; and write them to a single stream (maybe use simple framing)

	return nil
}

// RecoverFromSnapshot
func (d *baseDiskSM) RecoverFromSnapshot(r io.Reader,
	done <-chan struct{}) error {
	if d.closed {
		panic("recover from snapshot called after Close()")
	}

	// read from the stream, frame as
	// write to a file as clickhouse file
	// load to clickhouse
	// restart clickhouse

	return nil
}

// Close closes the state machine.
func (d *baseDiskSM) Close() error {
	return nil
}

func prepareSchema(ctx context.Context, conn driver.Conn) error {
	// prepare raft metadata table
	if err := conn.Exec(ctx, DDLRaftMetadata); err != nil {
		return err
	}

	return nil
}

func (s *baseDiskSM) loadMetadata(ctx context.Context) (*Metadata, error) {
	var payload string
	if err := s.conn.QueryRow(ctx, DQLReadRaftMetadata, "default").
		Scan(&payload); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	metadata, err := deserializeMetadata([]byte(payload))
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

func (s *baseDiskSM) saveMetadata(batch driver.Batch) error {
	payload, err := serializeMetadata(s.smMetadata)
	if err != nil {
		return err
	}

	return batch.Append("default", string(payload))
}
