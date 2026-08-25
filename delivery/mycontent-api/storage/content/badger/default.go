package badger

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v4"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var _ content.Repository = (*BadgerRepo)(nil)

var (
	ErrNotReady = errors.New("raft not ready")

	ErrInvalidNamespace = errors.New("invalid namespace")
	ErrInvalidRefSize   = errors.New("invalid ref size")
	ErrInvalidRefID     = errors.New("invalid ref ID")
	ErrInvalidID        = errors.New("invalid ID")
)

type BadgerRepo struct {
	db        *badger.DB
	TableName string
	RefSize   int
}

// New creates a Badger-backed content repository.
func New(db *badger.DB, tableName string, refSize int) *BadgerRepo {
	return &BadgerRepo{
		db:        db,
		TableName: tableName,
		RefSize:   refSize,
	}
}

// Post stores data at:
//
//	namespace -> refIDs -> ID
//
// Writes always require the complete logical path.
func (c *BadgerRepo) Post(
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

	key := c.dataKey(namespace, refIDs, id)

	err := c.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		return txn.Set(key, data.Data)
	})
	if err != nil {
		return content.Data{}, fmt.Errorf("store content: %w", err)
	}

	return content.Data{
		Namespace: namespace,
		RefIDs:    cloneStrings(refIDs),
		ID:        id,
		Data:      cloneBytes(data.Data),
		Meta:      cloneBytes(data.Meta),
		EventID:   data.EventID,
		Version:   data.Version,
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
func (c *BadgerRepo) Get(
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

	return result, nil
}

// Delete deletes a specific ID.
//
// If the item does not exist, Badger's ErrKeyNotFound is returned.
func (c *BadgerRepo) Delete(
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

	key := c.dataKey(namespace, refIDs, id)

	var previous []byte

	err := c.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		previous, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}

		return txn.Delete(key)
	})
	if err != nil {
		return content.Data{}, fmt.Errorf("delete content: %w", err)
	}

	return content.Data{
		Namespace: namespace,
		RefIDs:    cloneStrings(refIDs),
		ID:        id,
		Data:      previous,
	}, nil
}

// Stream streams all data matching the specified logical subtree.
//
// The public interface intentionally exposes only the data channel.
// Errors occurring during iteration are ignored here.
//
// Get uses stream() directly so it can observe those errors.
func (c *BadgerRepo) Stream(
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

// stream is the internal implementation of Stream.
//
// The error channel is buffered so that a caller which intentionally ignores
// the error channel cannot leave the streaming goroutine blocked forever.
func (c *BadgerRepo) stream(
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

		prefix := c.queryPrefix(namespace, refIDs, id)

		if namespace == "*" {
			prefix = c.tablePrefix()
		}

		err := c.iterate(
			ctx,
			prefix,
			func(key []byte, value []byte) (content.Data, error) {
				decoded, err := c.decodeDataKey(key)
				if err != nil {
					return content.Data{}, err
				}

				if !matchesQuery(
					decoded,
					namespace,
					refIDs,
					id,
				) {
					return content.Data{}, nil
				}

				return content.Data{
					Namespace: decoded.Namespace,
					RefIDs:    cloneStrings(decoded.RefIDs),
					ID:        decoded.ID,
					Data:      cloneBytes(value),
				}, nil
			},
			func(data content.Data) error {
				select {
				case dataCh <- data:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		)

		if err != nil {
			errCh <- fmt.Errorf("stream content: %w", err)
		}
	}()

	return dataCh, errCh
}

func (c *BadgerRepo) iterate(
	ctx context.Context,
	prefix []byte,
	decode func(key []byte, value []byte) (content.Data, error),
	consume func(content.Data) error,
) error {
	return c.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}

			item := it.Item()

			key := item.KeyCopy(nil)

			value, err := item.ValueCopy(nil)
			if err != nil {
				return fmt.Errorf("copy badger value: %w", err)
			}

			data, err := decode(key, value)
			if err != nil {
				return fmt.Errorf(
					"decode badger key %q: %w",
					string(key),
					err,
				)
			}

			// Empty Namespace means this item was filtered out by decode.
			if data.Namespace == "" {
				continue
			}

			if err := consume(data); err != nil {
				return err
			}
		}

		return nil
	})
}

func (c *BadgerRepo) validateWritePath(
	namespace string,
	refIDs []string,
	id string,
) error {
	if err := c.validateRepository(); err != nil {
		return err
	}

	if namespace == "" || namespace == "*" {
		return ErrInvalidNamespace
	}

	if len(refIDs) != c.RefSize {
		return fmt.Errorf(
			"%w: expected %d, got %d",
			ErrInvalidRefSize,
			c.RefSize,
			len(refIDs),
		)
	}

	for i, refID := range refIDs {
		if refID == "" || refID == "*" {
			return fmt.Errorf(
				"%w at index %d",
				ErrInvalidRefID,
				i,
			)
		}
	}

	if id == "" || id == "*" {
		return ErrInvalidID
	}

	return nil
}

func (c *BadgerRepo) validateReadPath(
	namespace string,
	refIDs []string,
	id string,
) error {
	if err := c.validateRepository(); err != nil {
		return err
	}

	if namespace == "" {
		return ErrInvalidNamespace
	}

	if len(refIDs) > c.RefSize {
		return fmt.Errorf(
			"%w: maximum %d, got %d",
			ErrInvalidRefSize,
			c.RefSize,
			len(refIDs),
		)
	}

	for i, refID := range refIDs {
		if refID == "" || refID == "*" {
			return fmt.Errorf(
				"%w at index %d",
				ErrInvalidRefID,
				i,
			)
		}
	}

	if id == "*" {
		return ErrInvalidID
	}

	return nil
}

func (c *BadgerRepo) validateRepository() error {
	if c.db == nil {
		return errors.New("badger database is nil")
	}

	if c.TableName == "" {
		return errors.New("table name is required")
	}

	if c.RefSize < 0 {
		return errors.New("ref size cannot be negative")
	}

	return nil
}

func (c *BadgerRepo) tablePrefix() []byte {
	return []byte(c.TableName)
}

func (c *BadgerRepo) queryPrefix(
	namespace string,
	refIDs []string,
	id string,
) []byte {
	return c.dataKey(namespace, refIDs, id)
}

func (c *BadgerRepo) dataKey(
	namespace string,
	refIDs []string,
	id string,
) []byte {
	return buildKey(
		false,
		c.TableName,
		namespace,
		refIDs,
		id,
	)
}

type decodedDataKey struct {
	Namespace string
	RefIDs    []string
	ID        string
}

func (c *BadgerRepo) decodeDataKey(key []byte) (decodedDataKey, error) {
	raw := string(key)

	raw = strings.TrimPrefix(raw, "vsn!")

	if !strings.HasPrefix(raw, c.TableName) {
		return decodedDataKey{}, errors.New(
			"key does not belong to table",
		)
	}

	raw = strings.TrimPrefix(raw, c.TableName)

	var result decodedDataKey

	if idx := strings.LastIndex(raw, "::"); idx >= 0 {
		result.ID = raw[idx+2:]
		raw = raw[:idx]

		if result.ID == "" {
			return decodedDataKey{}, ErrInvalidID
		}
	}

	if raw == "" {
		return result, nil
	}

	if !strings.HasPrefix(raw, "__") {
		return decodedDataKey{}, errors.New(
			"invalid content key namespace",
		)
	}

	raw = strings.TrimPrefix(raw, "__")

	if idx := strings.IndexByte(raw, '_'); idx >= 0 {
		result.Namespace = raw[:idx]
		raw = raw[idx+1:]
	} else {
		result.Namespace = raw
		raw = ""
	}

	if result.Namespace == "" {
		return decodedDataKey{}, ErrInvalidNamespace
	}

	if raw == "" {
		return result, nil
	}

	parts := strings.Split(raw, "|")

	result.RefIDs = make([]string, 0, len(parts))

	for _, refID := range parts {
		if refID == "" {
			return decodedDataKey{}, ErrInvalidRefID
		}

		result.RefIDs = append(result.RefIDs, refID)
	}

	return result, nil
}

func matchesQuery(
	key decodedDataKey,
	namespace string,
	refIDs []string,
	id string,
) bool {
	if namespace != "*" && key.Namespace != namespace {
		return false
	}

	if len(key.RefIDs) < len(refIDs) {
		return false
	}

	for i, refID := range refIDs {
		if key.RefIDs[i] != refID {
			return false
		}
	}

	if id != "" && key.ID != id {
		return false
	}

	return true
}

func buildKey(
	versioned bool,
	tableName string,
	namespace string,
	refIDs []string,
	id string,
) []byte {
	var builder strings.Builder

	if versioned {
		builder.WriteString("vsn!")
	}

	builder.WriteString(tableName)

	if namespace != "" {
		if namespace == "*" {
			namespace = ""
		}

		builder.WriteString("__")
		builder.WriteString(namespace)
	}

	if len(refIDs) > 0 {
		builder.WriteByte('_')
		builder.WriteString(strings.Join(refIDs, "|"))
	}

	if id != "" {
		builder.WriteString("::")
		builder.WriteString(id)
	}

	return []byte(builder.String())
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}

	return append([]byte(nil), value...)
}

func cloneStrings(value []string) []string {
	if value == nil {
		return nil
	}

	return append([]string(nil), value...)
}
