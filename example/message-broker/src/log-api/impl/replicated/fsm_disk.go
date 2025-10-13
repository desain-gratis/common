package replicated

import (
	"context"
	"database/sql"
	"io"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/rs/zerolog/log"

	sm "github.com/lni/dragonboat/v4/statemachine"
)

type baseDiskSM struct {
	lastApplied    uint64
	conn           driver.Conn
	closed         bool
	smMetadata     *Metadata
	happy          Happy
	database       string
	clickhouseAddr string
}

// NewDiskKV creates a new disk kv test state machine.
func NewDiskKV(shardID uint64, replicaID uint64) sm.IOnDiskStateMachine {
	d := &baseDiskSM{
		database: "_default",
	}
	return d
}

// Open opens the state machine and return the index of the last Raft Log entry
// already updated into the state machine.
func (d *baseDiskSM) Open(stopc <-chan struct{}) (uint64, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{d.clickhouseAddr},
		Auth: clickhouse.Auth{
			Username: "default",
			Password: "default",
		},
		Settings: map[string]interface{}{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		DialTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second,
	})
	if err != nil {
		log.Fatal().Msgf("failed to open connection base to clickhouse: %v err: %v", d.clickhouseAddr, err)
	}

	d.conn = conn

	ctx := context.Background()

	// get or create database
	err = d.prepareDB(ctx)
	if err != nil {
		log.Fatal().Msgf("failed to prepare DB: %v err: %v", d.clickhouseAddr, err)
	}

	// get or create schema
	err = d.prepareSchema(ctx)
	if err != nil {
		log.Fatal().Msgf("failed to prepare schema: %v err: %v", d.clickhouseAddr, err)
	}

	// get or create metadata
	err = d.loadMetadata(ctx)
	if err != nil {
		log.Fatal().Msgf("failed to load metadata to clickhouse: %v err: %v", d.clickhouseAddr, err)
	}

	return d.lastApplied, nil
}

// Lookup queries the state machine.
func (d *baseDiskSM) Lookup(key interface{}) (interface{}, error) {
	// Inject with context
	ctx := context.WithValue(context.Background(), chConnKey, d.conn)
	ctx = context.WithValue(ctx, metadataKey, d.smMetadata)

	return d.happy.Lookup(ctx, key)
}

func (d *baseDiskSM) Update(ents []sm.Entry) ([]sm.Entry, error) {
	// Inject with context
	ctx := context.WithValue(context.Background(), chConnKey, d.conn)
	ctx = context.WithValue(ctx, metadataKey, d.smMetadata)

	ctx, err := d.happy.PrepareUpdate(ctx)
	if err != nil {
		// need to panic
		return nil, err
	}

	// Process message one-by-one
	afterApplys := make([]OnAfterApply, len(ents))
	for idx := range ents {
		afterApplys[idx] = d.happy.OnUpdate(ctx, ents[idx])
	}

	// Apply update to disk
	err = d.happy.Apply(ctx)
	if err != nil {
		// need to panic
		return nil, err
	}

	// Execute function after successful apply
	for idx := range ents {
		res, err := afterApplys[idx]()
		if err != nil {
			// or panic..
			continue
		}
		ents[idx].Result = res
	}

	*d.smMetadata.AppliedIndex = ents[len(ents)-1].Index

	err = d.saveMetadata()
	if err != nil {
		log.Panic().Msgf("base save metadata failed")
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
func (s *baseDiskSM) prepareDB(ctx context.Context) error {
	// Check if the database exists
	if err := s.conn.Exec(ctx, "CREATE DATABASE IF NOT EXISTS `"+s.database+"`"); err != nil {
		return err
	}

	if err := s.conn.Exec(ctx, "USE "+s.database); err != nil {
		return err
	}

	return nil
}

func (s *baseDiskSM) prepareSchema(ctx context.Context) error {
	// prepare raft metadata table
	if err := s.conn.Exec(ctx, DDLRaftMetadata); err != nil {
		return err
	}

	// prepare chat log table
	if err := s.conn.Exec(ctx, DDLChatLog); err != nil {
		return err
	}

	return nil
}

func (s *baseDiskSM) loadMetadata(ctx context.Context) error {
	var payload string
	if err := s.conn.QueryRow(ctx, DQLReadRaftMetadata).Scan(&payload); err != nil && err != sql.ErrNoRows {
		return err
	}

	metadata, err := deserializeMetadata([]byte(payload))
	if err != nil {
		return err
	}

	s.smMetadata = metadata

	return nil
}

func (s *baseDiskSM) saveMetadata() error {
	payload, err := serializeMetadata(s.smMetadata)
	if err != nil {
		return err
	}

	err = s.conn.Exec(context.Background(), DMLWriteRaftMetadata, string(payload))
	if err != nil {
		return err
	}

	return nil
}
