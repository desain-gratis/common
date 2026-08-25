package badger

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/dgraph-io/badger/v4"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var _ content.Repository = (*VersionedBadgerRepo)(nil)

// VersionedBadgerRepo stores:
//
//	vsn!table__namespace_refIDs::ID -> uint32 version
//
// and the actual data at:
//
//	table__namespace_refIDs_ID::version
//
// In other words, the versioned entry acts as an index/pointer to the
// actual immutable data entry.
type VersionedBadgerRepo struct {
	*BadgerRepo
}

// NewVersioned creates a versioned Badger repository.
func NewVersioned(
	db *badger.DB,
	tableName string,
	refSize int,
) *VersionedBadgerRepo {
	return &VersionedBadgerRepo{
		BadgerRepo: &BadgerRepo{
			db:        db,
			TableName: tableName,
			RefSize:   refSize,
		},
	}
}

// Post writes a new immutable version of an entry.
//
// The version index and actual data are written in the same Badger
// transaction, so the index cannot be committed without the data.
func (c *VersionedBadgerRepo) Post(
	ctx context.Context,
	namespace string,
	refIDs []string,
	id string,
	data content.Data,
) (content.Data, error) {
	if err := c.validateWritePath(namespace, refIDs, id); err != nil {
		return content.Data{}, err
	}

	if err := ctx.Err(); err != nil {
		return content.Data{}, err
	}

	// Keep the existing behavior of validating Meta as content.Meta JSON.
	var meta content.Meta
	if err := json.Unmarshal(data.Meta, &meta); err != nil {
		return content.Data{}, fmt.Errorf(
			"invalid content meta: %w",
			err,
		)
	}

	keyVersion := buildKey(
		true,
		c.TableName,
		namespace,
		refIDs,
		id,
	)

	var version uint32

	err := c.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		prevVersion, err := c.getEntryVersion(txn, keyVersion)
		if err != nil {
			return err
		}

		if prevVersion == math.MaxUint32 {
			return errors.New("content version exhausted")
		}

		version = prevVersion + 1

		// The actual data uses the logical ID as another refID, and
		// the version becomes the physical leaf ID.
		actualRefIDs := make([]string, 0, len(refIDs)+1)
		actualRefIDs = append(actualRefIDs, refIDs...)
		actualRefIDs = append(actualRefIDs, id)

		keyEntry := buildKey(
			false,
			c.TableName,
			namespace,
			actualRefIDs,
			strconv.FormatUint(uint64(version), 10),
		)

		var versionBuf [4]byte
		binary.BigEndian.PutUint32(versionBuf[:], version)

		if err := txn.Set(keyVersion, versionBuf[:]); err != nil {
			return fmt.Errorf("write version index: %w", err)
		}

		if err := txn.Set(keyEntry, data.Data); err != nil {
			return fmt.Errorf("write versioned data: %w", err)
		}

		return nil
	})
	if err != nil {
		return content.Data{}, fmt.Errorf(
			"store versioned content: %w",
			err,
		)
	}

	return content.Data{
		Namespace: namespace,
		RefIDs:    cloneStrings(refIDs),
		ID:        id,
		Data:      cloneBytes(data.Data),
		Meta:      cloneBytes(data.Meta),
		EventID:   uint64(version),
		Version:   uint64(version),
	}, nil
}

// Get returns the latest version of every logical entry matching the query.
//
// The versioned index is queried first. Each matching index entry contains
// the version needed to locate the actual immutable data.
func (c *VersionedBadgerRepo) Get(
	ctx context.Context,
	namespace string,
	refIDs []string,
	id string,
) ([]content.Data, error) {
	if err := c.validateReadPath(namespace, refIDs, id); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dataCh, errCh := c.stream(ctx, namespace, refIDs, id)

	result := make([]content.Data, 0)

	for dataCh != nil || errCh != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case data, ok := <-dataCh:
			if !ok {
				dataCh = nil
				continue
			}

			result = append(result, data)

		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}

			if err != nil {
				return nil, err
			}
		}
	}

	if id != "" && len(result) == 0 {
		return nil, content.ErrNotFound
	}

	return result, nil
}

// Delete removes the logical entry and all of its immutable versions.
//
// A versioned entry consists of:
//
//	vsn!table__namespace_refIDs::ID -> current version
//
// and one or more:
//
//	table__namespace_refIDs_ID::version -> data
//
// We delete both the version index and all physical versions in the same
// Badger transaction.
//
// This is important because deleting only the version index would cause the
// next Post() to start again at version 1 while old physical versions could
// still exist.
func (c *VersionedBadgerRepo) Delete(
	ctx context.Context,
	namespace string,
	refIDs []string,
	id string,
) (content.Data, error) {
	if err := c.validateWritePath(namespace, refIDs, id); err != nil {
		return content.Data{}, err
	}

	if err := ctx.Err(); err != nil {
		return content.Data{}, err
	}

	keyVersion := buildKey(
		true,
		c.TableName,
		namespace,
		refIDs,
		id,
	)

	var (
		previous []byte
		version  uint32
	)

	err := c.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Read the current version from the index.
		currentVersion, err := c.getEntryVersion(txn, keyVersion)
		if err != nil {
			return err
		}

		// getEntryVersion() returns zero for a missing index. Since versions
		// start at 1, zero means that the logical entry does not exist.
		if currentVersion == 0 {
			return content.ErrNotFound
		}

		version = currentVersion

		// Build the current actual-data key.
		keyEntry := c.entryKey(
			namespace,
			refIDs,
			id,
			currentVersion,
		)

		item, err := txn.Get(keyEntry)
		if errors.Is(err, badger.ErrKeyNotFound) {
			// The index exists but the actual current data is missing.
			// Treat this as an inconsistent repository state rather than
			// silently deleting the index.
			return fmt.Errorf(
				"version index points to missing data: version=%d: %w",
				currentVersion,
				mycontent.ErrNotFound,
			)
		}
		if err != nil {
			return err
		}

		previous, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}

		// Delete the version index first.
		if err := txn.Delete(keyVersion); err != nil {
			return fmt.Errorf(
				"delete version index: %w",
				err,
			)
		}

		// Delete all physical versions of this logical entry.
		//
		// The physical key is:
		//
		//	table__namespace_refIDs_ID::version
		//
		// Therefore everything below this exact prefix belongs to this
		// logical entry.
		actualRefIDs := make([]string, 0, len(refIDs)+1)
		actualRefIDs = append(actualRefIDs, refIDs...)
		actualRefIDs = append(actualRefIDs, id)

		prefix := buildKey(
			false,
			c.TableName,
			namespace,
			actualRefIDs,
			"",
		)

		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}

			key := it.Item().KeyCopy(nil)

			if err := txn.Delete(key); err != nil {
				return fmt.Errorf(
					"delete versioned data %q: %w",
					string(key),
					err,
				)
			}
		}

		return nil
	})
	if err != nil {
		return content.Data{}, fmt.Errorf(
			"delete versioned content: %w",
			err,
		)
	}

	return content.Data{
		Namespace: namespace,
		RefIDs:    cloneStrings(refIDs),
		ID:        id,
		Data:      previous,
		EventID:   uint64(version),
		Version:   uint64(version),
	}, nil
}

// Stream exposes the versioned stream through the public Repository API.
//
// Errors after the stream has started cannot be returned through the public
// interface, so the internal error channel is intentionally ignored here.
func (c *VersionedBadgerRepo) Stream(
	ctx context.Context,
	namespace string,
	refIDs []string,
	id string,
) (<-chan content.Data, error) {
	if err := c.validateReadPath(namespace, refIDs, id); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dataCh, _ := c.stream(ctx, namespace, refIDs, id)

	return dataCh, nil
}

// stream is the internal versioned implementation.
//
// The version index is:
//
//	vsn!table__namespace_refIDs::ID -> uint32 version
//
// The actual value is then found at:
//
//	table__namespace_refIDs_ID::version
func (c *VersionedBadgerRepo) stream(
	ctx context.Context,
	namespace string,
	refIDs []string,
	id string,
) (<-chan content.Data, <-chan error) {
	dataCh := make(chan content.Data)
	errCh := make(chan error, 1)

	go func() {
		defer close(dataCh)
		defer close(errCh)

		prefix := c.versionQueryPrefix(
			namespace,
			refIDs,
			id,
		)

		err := c.db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.PrefetchValues = true
			opts.Prefix = prefix

			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				if err := ctx.Err(); err != nil {
					return err
				}

				versionKey := it.Item().KeyCopy(nil)

				decoded, err := c.decodeDataKey(versionKey)
				if err != nil {
					return fmt.Errorf(
						"decode version key %q: %w",
						string(versionKey),
						err,
					)
				}

				if !matchesQuery(
					decoded,
					namespace,
					refIDs,
					id,
				) {
					continue
				}

				version, err := c.getEntryVersion(
					txn,
					versionKey,
				)
				if err != nil {
					return fmt.Errorf(
						"read version for %q: %w",
						string(versionKey),
						err,
					)
				}

				entryKey := c.entryKey(
					decoded.Namespace,
					decoded.RefIDs,
					decoded.ID,
					version,
				)

				entry, err := txn.Get(entryKey)
				if errors.Is(err, badger.ErrKeyNotFound) {
					// The version index exists but the actual data does
					// not. This indicates inconsistent storage.
					//
					// Preserve the previous behavior of skipping it,
					// rather than failing the entire query.
					continue
				}
				if err != nil {
					return fmt.Errorf(
						"read versioned entry: %w",
						err,
					)
				}

				entryData, err := entry.ValueCopy(nil)
				if err != nil {
					return fmt.Errorf(
						"copy versioned entry: %w",
						err,
					)
				}

				data := content.Data{
					Namespace: decoded.Namespace,
					RefIDs:    cloneStrings(decoded.RefIDs),
					ID:        decoded.ID,
					Data:      entryData,
					EventID:   uint64(version),
					Version:   uint64(version),
				}

				select {
				case dataCh <- data:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			return nil
		})

		if err != nil {
			errCh <- fmt.Errorf("stream versioned content: %w", err)
		}
	}()

	return dataCh, errCh
}

// getEntryVersion gets the current version from the version index.
//
// Missing version means this is the first version, so zero is returned.
//
// The version value must be exactly four bytes because it is stored as a
// uint32 in big-endian form.
func (c *VersionedBadgerRepo) getEntryVersion(
	txn *badger.Txn,
	keyVersion []byte,
) (uint32, error) {
	item, err := txn.Get(keyVersion)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	value, err := item.ValueCopy(nil)
	if err != nil {
		return 0, err
	}

	if len(value) != 4 {
		return 0, fmt.Errorf(
			"invalid version value length: expected 4, got %d",
			len(value),
		)
	}

	return binary.BigEndian.Uint32(value), nil
}

// versionQueryPrefix returns the narrowest version-index prefix possible.
//
// For namespace="*" we have to scan all version-index keys because namespace
// occurs before refIDs in the physical key.
func (c *VersionedBadgerRepo) versionQueryPrefix(
	namespace string,
	refIDs []string,
	id string,
) []byte {
	if namespace == "*" {
		return []byte("vsn!" + c.TableName)
	}

	return buildKey(
		true,
		c.TableName,
		namespace,
		refIDs,
		id,
	)
}

// entryKey constructs the physical key containing the immutable data.
//
// Logical:
//
//	namespace -> refIDs -> ID
//
// Physical:
//
//	namespace -> refIDs -> ID -> version
func (c *VersionedBadgerRepo) entryKey(
	namespace string,
	refIDs []string,
	id string,
	version uint32,
) []byte {
	actualRefIDs := make([]string, 0, len(refIDs)+1)
	actualRefIDs = append(actualRefIDs, refIDs...)
	actualRefIDs = append(actualRefIDs, id)

	return buildKey(
		false,
		c.TableName,
		namespace,
		actualRefIDs,
		strconv.FormatUint(uint64(version), 10),
	)
}
