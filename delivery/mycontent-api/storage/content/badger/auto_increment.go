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

var _ content.Repository = (*AutoIncrementBadgerRepo)(nil)

// AutoIncrementBadgerRepo behaves like BadgerRepo, except that when Post()
// receives an empty ID, it automatically allocates a numeric string ID.
//
// Counter:
//
//	aid!table__namespace_refIDs -> uint64 last allocated ID
//
// Actual data:
//
//	table__namespace_refIDs::ID -> data
//
// The generated ID is therefore stored in the key, not in the value.
type AutoIncrementBadgerRepo struct {
	*BadgerRepo
}

// NewAutoIncrement creates an auto-incrementing Badger repository.
// TODO: make phyisically compatible with Versioned (by adding !vsn index)
func NewAutoIncrement(
	db *badger.DB,
	tableName string,
	refSize int,
) *AutoIncrementBadgerRepo {
	return &AutoIncrementBadgerRepo{
		BadgerRepo: &BadgerRepo{
			db:        db,
			TableName: tableName,
			RefSize:   refSize,
		},
	}
}

func (c *AutoIncrementBadgerRepo) GetLatest() *AutoIncrementLatestBadgerRepo {
	return &AutoIncrementLatestBadgerRepo{
		c,
	}
}

// Post creates an entry.
//
// If id is non-empty, this behaves exactly like the normal BadgerRepo.
//
// If id is empty, a new numeric string ID is allocated atomically.
//
// IDs start at "1".
func (c *AutoIncrementBadgerRepo) Post(
	ctx context.Context,
	namespace string,
	refIDs []string,
	id string,
	data content.Data,
) (content.Data, error) {
	// For explicitly supplied IDs, use the normal implementation.
	if id != "" {
		return c.BadgerRepo.Post(
			ctx,
			namespace,
			refIDs,
			id,
			data,
		)
	}

	if err := ctx.Err(); err != nil {
		return content.Data{}, err
	}

	// The normal validateWritePath requires an ID. For auto-increment
	// entries there is intentionally no ID yet, so validate the parts
	// that are already known instead.
	if err := c.validateAutoIncrementPath(namespace, refIDs); err != nil {
		return content.Data{}, err
	}

	// Keep the same Meta validation behavior as the normal repository.
	var meta content.Meta
	if err := json.Unmarshal(data.Meta, &meta); err != nil {
		return content.Data{}, fmt.Errorf(
			"invalid content meta: %w",
			err,
		)
	}

	counterKey := c.autoIncrementKey(
		namespace,
		refIDs,
	)

	var generatedID string

	err := c.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastID, err := c.getAutoIncrementValue(
			txn,
			counterKey,
		)
		if err != nil {
			return fmt.Errorf(
				"read auto-increment counter: %w",
				err,
			)
		}

		for {
			if lastID == math.MaxUint64 {
				return errors.New("content ID exhausted")
			}

			lastID++

			generatedID = strconv.FormatUint(lastID, 10)

			keyEntry := buildKey(
				false,
				c.TableName,
				namespace,
				refIDs,
				generatedID,
			)

			// Protect against IDs that may already have been manually
			// created.
			_, err := txn.Get(keyEntry)

			if errors.Is(err, badger.ErrKeyNotFound) {
				// The generated ID is available.

				if err := txn.Set(keyEntry, data.Data); err != nil {
					return fmt.Errorf(
						"write auto-increment content: %w",
						err,
					)
				}

				// Store the last allocated ID.
				var counterBuf [8]byte
				binary.BigEndian.PutUint64(
					counterBuf[:],
					lastID,
				)

				if err := txn.Set(
					counterKey,
					counterBuf[:],
				); err != nil {
					return fmt.Errorf(
						"write auto-increment counter: %w",
						err,
					)
				}

				return nil
			}

			if err != nil {
				return fmt.Errorf(
					"check auto-increment ID %q: %w",
					generatedID,
					err,
				)
			}

			// ID already exists. Try the next one.
		}
	})

	if err != nil {
		return content.Data{}, fmt.Errorf(
			"store auto-increment content: %w",
			err,
		)
	}

	ver, _ := strconv.ParseUint(generatedID, 10, 64) // todo: make more efficient ofcorss

	return content.Data{
		Namespace: namespace,
		RefIDs:    cloneStrings(refIDs),
		ID:        generatedID,
		Data:      cloneBytes(data.Data),
		Meta:      cloneBytes(data.Meta),
		EventID:   0,
		Version:   &ver, // TODOmaxxxing
	}, nil
}

// Get returns all data matching the specified logical subtree.
//
// Read paths may be partial:
//
//	namespace="foo", refIDs=[]        -> everything in foo
//	namespace="foo", refIDs=["a"]     -> everything below foo/a
//	namespace="*", refIDs=[]          -> everything
//	namespace="*", refIDs=["a"]       -> a below every namespace
//
// If ID is specified, only the exact ID is returned.
//
// For auto-increment entries, Version is populated from the numeric ID.
func (c *AutoIncrementBadgerRepo) Get(
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

			// Auto-increment IDs are numeric and represent the
			// physical version/sequence of the entry.
			if data.ID != "" {
				version, err := strconv.ParseUint(data.ID, 10, 64)
				if err == nil {
					data.Version = &version
				}
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
		return nil, fmt.Errorf(
			"get %w: id=%s refIds=%v",
			mycontent.ErrNotFound,
			id,
			refIDs,
		)
	}

	return result, nil
}

// validateAutoIncrementPath validates everything except the ID.
//
// An auto-increment Post deliberately has no ID before the transaction,
// because the ID is generated by the repository.
func (c *AutoIncrementBadgerRepo) validateAutoIncrementPath(
	namespace string,
	refIDs []string,
) error {
	if namespace == "" {
		return errors.New("namespace cannot be empty")
	}

	if len(refIDs) != c.RefSize {
		return fmt.Errorf(
			"invalid refIDs length: expected %d, got %d",
			c.RefSize,
			len(refIDs),
		)
	}

	for i, refID := range refIDs {
		if refID == "" {
			return fmt.Errorf(
				"refID[%d] cannot be empty",
				i,
			)
		}
	}

	return nil
}

// autoIncrementKey returns the counter key for a namespace/refIDs
// collection.
//
// The ID is intentionally NOT part of this key because the counter is
// shared by all entries under the same namespace/refIDs path.
func (c *AutoIncrementBadgerRepo) autoIncrementKey(
	namespace string,
	refIDs []string,
) []byte {
	key := buildKey(
		false,
		c.TableName,
		namespace,
		refIDs,
		"",
	)

	return append([]byte("aid!"), key...)
}

// getAutoIncrementValue returns the last allocated ID.
//
// A missing counter means that no auto-increment ID has been allocated yet,
// so zero is returned and the first generated ID becomes "1".
func (c *AutoIncrementBadgerRepo) getAutoIncrementValue(
	txn *badger.Txn,
	key []byte,
) (uint64, error) {
	item, err := txn.Get(key)

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

	if len(value) != 8 {
		return 0, fmt.Errorf(
			"invalid auto-increment counter length: expected 8, got %d",
			len(value),
		)
	}

	return binary.BigEndian.Uint64(value), nil
}
