package badger

import (
	"context"
	"errors"
	"strconv"

	"github.com/dgraph-io/badger/v4"

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
type AutoIncrementLatestBadgerRepo struct {
	*AutoIncrementBadgerRepo
}

// Get resolves (namespace, refIDs, ID) to the latest allocated version
// under the shifted branch (namespace, append(refIDs, ID)), then delegates
// to the underlying BadgerRepo.Get to fetch it.
func (c *AutoIncrementLatestBadgerRepo) Get(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) ([]content.Data, error) {
	fullRefIDs, latestID, err := c.resolveLatest(ctx, namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	return c.BadgerRepo.Get(
		ctx,
		namespace,
		fullRefIDs,
		latestID,
	)
}

// Stream resolves (namespace, refIDs, ID) the same way Get does, then
// delegates to the underlying BadgerRepo.Stream to stream it.
func (c *AutoIncrementLatestBadgerRepo) Stream(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) (<-chan content.Data, error) {
	fullRefIDs, latestID, err := c.resolveLatest(ctx, namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	return c.BadgerRepo.Stream(
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
