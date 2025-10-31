package statemachine

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/desain-gratis/common/example/message-broker/src/log-api/conn/clickhouse"
	"github.com/desain-gratis/common/lib/logwriter"
	"github.com/rs/zerolog/log"

	sm "github.com/lni/dragonboat/v4/statemachine"
)

type baseDiskSM struct {
	lastApplied    uint64
	conn           driver.Conn
	closed         bool
	smMetadata     *Metadata
	initialApplied uint64
	happy          logwriter.Happy
	database       string
	clickhouseAddr string
}

func NewWithHappy(clickhouseAddr string, happy logwriter.Happy) func(shardID uint64, replicaID uint64) sm.IOnDiskStateMachine {
	return func(shardID uint64, replicaID uint64) sm.IOnDiskStateMachine {
		database := fmt.Sprintf("%v_%v_%v", "chat_app", shardID, replicaID)
		clickhouse.CreateDB(clickhouseAddr, database)
		return &baseDiskSM{
			database:       database,
			clickhouseAddr: clickhouseAddr,
			happy:          happy,
		}
	}
}

// Open opens the state machine and return the index of the last Raft Log entry
// already updated into the state machine.
func (d *baseDiskSM) Open(stopc <-chan struct{}) (uint64, error) {
	ctx := context.Background()

	d.conn = clickhouse.Connect(d.clickhouseAddr, d.database)

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

	err = d.happy.Init(ctx)
	if err != nil {
		log.Fatal().Msgf("failed to init happy err: %v", err)
	}

	return d.lastApplied, nil
}

// Lookup queries the state machine.
func (d *baseDiskSM) Lookup(key interface{}) (interface{}, error) {
	// Inject with context
	ctx := context.WithValue(context.Background(), chConnKey, d.conn)

	return d.happy.Lookup(ctx, key)
}

func (d *baseDiskSM) Update(ents []sm.Entry) ([]sm.Entry, error) {
	ctx := context.WithValue(context.Background(), chConnKey, d.conn)

	metadataBatch, err := d.conn.PrepareBatch(ctx, `INSERT INTO metadata (namespace, data)`)
	if err != nil {
		log.Panic().Msgf("failed to apply")
	}
	defer metadataBatch.Close()

	ctx = context.WithValue(ctx, metadataBatchKey, metadataBatch)

	ctx, err = d.happy.PrepareUpdate(ctx)
	if err != nil {
		log.Panic().Msgf("failed to prepare for update: %v", err)
	}

	// Process message one-by-one
	afterApplys := make([]logwriter.OnAfterApply, len(ents))
	for idx := range ents {
		if ents[idx].Index <= d.initialApplied {
			log.Panic().Msgf("oh no initial")
		}
		afterApplys[idx] = d.happy.OnUpdate(ctx, ents[idx])
	}

	// Apply update to disk
	err = d.happy.Apply(ctx)
	if err != nil {
		log.Panic().Msgf("failed to apply")
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

	// Execute function after successful apply
	for idx := range ents {
		res, err := afterApplys[idx]()
		if err != nil {
			continue
		}
		ents[idx].Result = res
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
