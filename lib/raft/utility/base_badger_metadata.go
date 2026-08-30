package utility

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/desain-gratis/common/lib/raft"
	"github.com/dgraph-io/badger/v4"
)

type BadgerMetadataWriter struct {
	db                  *badger.DB
	lastAppliedIndexKey string
}

func NewBadgerMetadataWriter(db *badger.DB, lastAppliedIndexKey string) *BadgerMetadataWriter {
	if lastAppliedIndexKey == "" {
		lastAppliedIndexKey = "last-applied-index"
	}

	return &BadgerMetadataWriter{
		db:                  db,
		lastAppliedIndexKey: lastAppliedIndexKey,
	}
}

func (m *BadgerMetadataWriter) Apply(ctx context.Context, entry raft.EntryV2) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], entry.Index)

	err := m.db.Update(func(txn *badger.Txn) error {
		// todo: make the delete/post inside here as well.. later
		return txn.Set([]byte(m.lastAppliedIndexKey), buf[:])
	})
	if err != nil {
		return err
	}

	return nil
}

func (m *BadgerMetadataWriter) GetLastAppliedIndex(ctx context.Context) (uint64, error) {
	tmp := make([]byte, 0, 8)
	err := m.db.View(func(txn *badger.Txn) error {
		// todo: make the delete/post inside here as well.. later
		value, err := txn.Get([]byte(m.lastAppliedIndexKey))
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
