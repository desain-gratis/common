package badger

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"

	"github.com/dgraph-io/badger/v4"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var _ content.Repository = (*AutoIncrementLatestBadgerRepo)(nil)

// errNodeIDRequired is returned when the shifted "last node" ID (passed as
// the ID argument to Get/Stream) is empty.
var errNodeIDRequired = errors.New("node ID (last refID) cannot be empty")

// notFoundVersion is used as a sentinel version ID when no version has ever
// been allocated for a branch. It is guaranteed to never collide with a
// real generated ID, since AutoIncrementBadgerRepo IDs start at "1". Using
// it lets the delegated BadgerRepo.Get/Stream calls surface their own
// standard "not found" behavior instead of this file duplicating it.
const notFoundVersion = "0"

// aidPrefix is the byte prefix AutoIncrementBadgerRepo uses for its
// per-branch counters: "aid!" + buildKey(false, table, namespace, refIDs, "").
const aidPrefix = "aid!"

// AutoIncrementLatestBadgerRepo exposes read access to the *latest*
// auto-incremented version stored under a given branch.
//
// Its Get/Stream methods use a shifted addressing scheme compared to
// AutoIncrementBadgerRepo: the last element that would normally live in
// refIDs is instead passed as ID, identifying the branch whose latest leaf
// version (allocated by AutoIncrementBadgerRepo.Post) should be returned.
//
// Example:
//
//	Tree:     root-node ---> node1 ---> sub-node1 ---> leaf-node (v3 latest)
//
//	Post (AutoIncrementBadgerRepo), called 3 times:
//	    namespace = "root-node"
//	    refIDs    = ["node1", "sub-node1"]
//	    ID        = ""                     // auto-allocated: "1", "2", "3"
//
//	Get (AutoIncrementLatestBadgerRepo):
//	    namespace = "root-node"
//	    refIDs    = ["node1"]
//	    ID        = "sub-node1"            // last node, shifted into ID
//
// The call above resolves to the same underlying path
// ("root-node", ["node1", "sub-node1"]) and returns only the data stored
// at the highest allocated version ("3"), as a single-element result.
//
// Special case: namespace == "*"
//
// Passing "*" as namespace returns the latest version of the given branch
// (refIDs+ID) across every namespace it exists under -- one result per
// namespace -- mirroring VersionedBadgerRepo's own namespace wildcard.
//
// This is done efficiently: AutoIncrementBadgerRepo keeps one counter key
// per branch (under the "aid!" prefix), not one per version. Scanning that
// prefix costs O(number of branches), then one targeted point-lookup is
// done per matching branch to fetch its actual latest data -- the total
// leaf/version count never has to be scanned.
type AutoIncrementLatestBadgerRepo struct {
	*AutoIncrementBadgerRepo
}

// Get resolves (namespace, refIDs, ID) to the latest allocated version
// under the shifted branch (namespace, append(refIDs, ID)), then delegates
// to the underlying BadgerRepo.Get to fetch it.
//
// If namespace is "*", it instead returns the latest version of that
// branch across every namespace it exists under (see type doc).
func (c *AutoIncrementLatestBadgerRepo) Get(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) ([]content.Data, error) {
	if namespace == "*" {
		return c.getAllNamespaces(ctx, refIDs, ID)
	}

	fullRefIDs, latestID, err := c.resolveLatest(ctx, namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	return c.AutoIncrementBadgerRepo.Get(
		ctx,
		namespace,
		fullRefIDs,
		latestID,
	)
}

// Stream resolves (namespace, refIDs, ID) the same way Get does, then
// delegates to the underlying BadgerRepo.Stream to stream it.
//
// If namespace is "*", it instead streams the latest version of that
// branch across every namespace it exists under (see type doc).
func (c *AutoIncrementLatestBadgerRepo) Stream(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) (<-chan content.Data, error) {
	if namespace == "*" {
		return c.streamAllNamespaces(ctx, refIDs, ID)
	}

	fullRefIDs, latestID, err := c.resolveLatest(ctx, namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	return c.AutoIncrementBadgerRepo.Stream(
		ctx,
		namespace,
		fullRefIDs,
		latestID,
	)
}

// resolveLatest validates the shifted addressing, builds the underlying
// (namespace, refIDs) pair expected by AutoIncrementBadgerRepo, and looks up
// the latest allocated version ID for that branch.
//
// If no version has ever been allocated for the branch, it returns
// notFoundVersion as the ID so callers can delegate to the base repo and
// get its normal "not found" behavior, rather than duplicating that logic
// here.
func (c *AutoIncrementLatestBadgerRepo) resolveLatest(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) ([]string, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}

	if ID == "" {
		return nil, "", errNodeIDRequired
	}

	// Shift ID into refIDs: it represents the last node of the actual
	// branch whose latest version we want to read.
	fullRefIDs := append(cloneStrings(refIDs), ID)

	// validateReadPath allows an empty leaf ID (used here only to check
	// namespace/refIDs shape; the actual leaf we want is resolved below
	// from the auto-increment counter, not from this call).
	if err := c.validateReadPath(namespace, fullRefIDs, ""); err != nil {
		return nil, "", err
	}

	counterKey := c.autoIncrementKey(namespace, fullRefIDs)

	var lastID uint64

	err := c.db.View(func(txn *badger.Txn) error {
		var err error

		lastID, err = c.getAutoIncrementValue(txn, counterKey)

		return err
	})
	if err != nil {
		return nil, "", err
	}

	if lastID == 0 {
		return fullRefIDs, notFoundVersion, nil
	}

	return fullRefIDs, strconv.FormatUint(lastID, 10), nil
}

// wildcardBranch identifies one branch found while scanning counters
// across every namespace, along with its already-known latest version.
type wildcardBranch struct {
	namespace string
	refIDs    []string
	latestID  string
}

// getAllNamespaces implements Get for namespace == "*": the latest version
// of the given branch (refIDs+ID) across every namespace it exists under.
func (c *AutoIncrementLatestBadgerRepo) getAllNamespaces(
	ctx context.Context,
	refIDs []string,
	ID string,
) ([]content.Data, error) {
	branches, err := c.scanWildcardBranches(ctx, refIDs, ID)
	if err != nil {
		return nil, err
	}

	if len(branches) == 0 {
		return nil, mycontent.ErrNotFound
	}

	result := make([]content.Data, 0, len(branches))

	for _, b := range branches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		items, err := c.AutoIncrementBadgerRepo.Get(
			ctx,
			b.namespace,
			b.refIDs,
			b.latestID,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"get latest for namespace %q: %w",
				b.namespace,
				err,
			)
		}

		result = append(result, items...)
	}

	return result, nil
}

// streamAllNamespaces implements Stream for namespace == "*": the latest
// version of the given branch (refIDs+ID) across every namespace it exists
// under, delivered as they're fetched.
//
// As with Stream's single-namespace case, errors after the stream has
// started simply end it early -- there's no error channel on the public
// interface to report them through.
func (c *AutoIncrementLatestBadgerRepo) streamAllNamespaces(
	ctx context.Context,
	refIDs []string,
	ID string,
) (<-chan content.Data, error) {
	branches, err := c.scanWildcardBranches(ctx, refIDs, ID)
	if err != nil {
		return nil, err
	}

	out := make(chan content.Data)

	go func() {
		defer close(out)

		for _, b := range branches {
			if ctx.Err() != nil {
				return
			}

			items, err := c.AutoIncrementBadgerRepo.Get(
				ctx,
				b.namespace,
				b.refIDs,
				b.latestID,
			)
			if err != nil {
				return
			}

			for _, item := range items {
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// scanWildcardBranches finds every branch, across every namespace, whose
// shape matches (refIDs+ID), along with the latest version already
// allocated for it.
//
// This scans only the "aid!" counter keyspace -- one entry per branch,
// regardless of how many versions that branch has -- rather than the
// (potentially much larger) leaf/version keyspace. Namespace can't be
// skipped over in the key layout (it comes before refIDs), so the prefix
// is anchored only by table name; refIDs+ID are then matched per-entry
// during the scan.
func (c *AutoIncrementLatestBadgerRepo) scanWildcardBranches(
	ctx context.Context,
	refIDs []string,
	ID string,
) ([]wildcardBranch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if ID == "" {
		return nil, errNodeIDRequired
	}

	targetRefIDs := append(cloneStrings(refIDs), ID)

	prefix := []byte(aidPrefix + c.TableName)

	var branches []wildcardBranch

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

			item := it.Item()
			key := item.KeyCopy(nil)

			if len(key) < len(aidPrefix) {
				continue
			}

			// Everything after "aid!" is a normal data key (with an
			// empty leaf ID), since that's exactly how
			// AutoIncrementBadgerRepo.autoIncrementKey built it.
			decoded, err := c.decodeDataKey(key[len(aidPrefix):])
			if err != nil {
				return fmt.Errorf(
					"decode counter key %q: %w",
					string(key),
					err,
				)
			}

			if !refIDsEqual(decoded.RefIDs, targetRefIDs) {
				continue
			}

			value, err := item.ValueCopy(nil)
			if err != nil {
				return fmt.Errorf(
					"copy counter value: %w",
					err,
				)
			}

			if len(value) != 8 {
				return fmt.Errorf(
					"invalid auto-increment counter length: expected 8, got %d",
					len(value),
				)
			}

			lastID := binary.BigEndian.Uint64(value)
			if lastID == 0 {
				// A counter key should never be written with a zero
				// value, but skip defensively rather than surface a
				// bogus "version 0".
				continue
			}

			branches = append(branches, wildcardBranch{
				namespace: decoded.Namespace,
				refIDs:    cloneStrings(decoded.RefIDs),
				latestID:  strconv.FormatUint(lastID, 10),
			})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan branches: %w", err)
	}

	return branches, nil
}

// refIDsEqual reports whether two refID slices contain the same elements
// in the same order.
func refIDsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
