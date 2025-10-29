package writer

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

// todo: rename to clickhouseSM or something
type baseDiskSM struct {
	appName        string
	lastApplied    uint64
	conn           driver.Conn
	closed         bool
	smMetadata     *Metadata
	initialApplied uint64
	happy          Happy
	database       string
	clickhouseAddr string

	chwriter ClickhouseWriter
}

// Open opens the state machine and return the index of the last Raft Log entry
// already updated into the state machine.
func (d *baseDiskSM) Open(stopc <-chan struct{}) (uint64, error) {
	opts := &clickhouse.Options{
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
	}

	var attempt int
	var conn driver.Conn
	var err error

	for {
		attempt++
		conn, err = clickhouse.Open(opts)
		if err == nil {
			break
		} else if attempt >= 5 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	var s string
	if attempt > 1 {
		s = "s"
	}

	if err != nil {
		log.Fatal().Msgf("failed to open connection base to clickhouse: %v after %v attempt%v err: %v", d.clickhouseAddr, attempt, s, err)
	}

	log.Info().Msgf("✅ Connected to ClickHouse in %v attempt%v", attempt, s)

	ctx := context.Background()

	d.conn = conn

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

func (d *baseDiskSM) Lookup(key interface{}) (interface{}, error) {
	// base query handling; lookup in local state machine

	return d.chwriter.Query(d.conn, key)
}

func (d *baseDiskSM) Update(ents []sm.Entry) ([]sm.Entry, error) {
	ctx := context.Background()
	batches := make(map[string]driver.Batch)
	batchFns := make(map[string]BatchFn)

	for name, table := range d.chwriter.GetTables() {
		batch, err := d.conn.PrepareBatch(ctx, table.BatchStmt)
		if err != nil {
			log.Panic().Msgf("prepare batch%v", err)
		}
		batches[name] = batch
		batchFns[name] = table.BatchFn
	}

	// Process message one-by-one
	afterApplys := make([]OnAfterCommit, len(ents))
	for idx := range ents {
		if ents[idx].Index <= d.initialApplied {
			log.Panic().Msgf("oh no initial")
		}
		afterApplys[idx] = d.chwriter.Update(batchFns, ents[idx].Cmd)
	}

	log.Info().Msgf("entry size: %v", len(ents))

	// commit batches

	for _, batch := range batches {
		err := batch.Send()
		if err != nil {
			log.Panic().Msgf("batch send %v", err)
		}
		err = batch.Close()
		if err != nil {
			log.Panic().Msgf("batch close %v", err)
		}
	}

	// Execute function after successful apply
	for idx := range ents {
		res, err := afterApplys[idx]()
		if err != nil {
			// or panic..
			continue
		}
		ents[idx].Result.Data = res
	}

	*d.smMetadata.AppliedIndex = ents[len(ents)-1].Index

	err = d.saveMetadata()
	if err != nil {
		log.Panic().Msgf("base save metadata failed %v", err)
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

	opts := &clickhouse.Options{
		Addr: []string{s.clickhouseAddr},
		Auth: clickhouse.Auth{
			Username: "default",
			Password: "default",
			Database: s.database,
		},
		Settings: map[string]interface{}{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		DialTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second,
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return err
	}
	// replace with default database
	s.conn = conn

	return nil
}

func (s *baseDiskSM) prepareSchema(ctx context.Context) error {
	// prepare raft metadata table
	if err := s.conn.Exec(ctx, DDLRaftMetadata); err != nil {
		return err
	}

	ctx = context.WithValue(ctx, chConnKey, s.conn)
	err := s.happy.PrepareSchema(ctx)
	if err != nil {
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

	s.initialApplied = *metadata.AppliedIndex
	s.lastApplied = *metadata.AppliedIndex

	return nil
}

func (s *baseDiskSM) saveMetadata() error {
	payload, err := serializeMetadata(s.smMetadata)
	if err != nil {
		return err
	}

	// err = s.conn.Exec(context.Background(), DMLWriteRaftMetadata, string(payload))
	// if err != nil {
	// 	return err
	// }

	err = s.conn.AsyncInsert(context.Background(), DMLWriteRaftMetadata, true, string(payload))
	if err != nil {
		return err
	}

	return nil
}
