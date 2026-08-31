package badger

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dgraph-io/badger/v4"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

// newTestDB opens a fresh in-memory Badger instance for a single test and
// registers cleanup so it's closed automatically.
func newTestDB(t *testing.T) *badger.DB {
	t.Helper()

	opts := badger.DefaultOptions("").
		WithInMemory(true).
		WithLoggingLevel(badger.ERROR)

	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("open in-memory badger db: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close badger db: %v", err)
		}
	})

	return db
}

// postAutoIncrement POSTs with an empty ID, letting AutoIncrementBadgerRepo
// allocate the next numeric ID, and fails the test on error.
func postAutoIncrement(
	t *testing.T,
	repo *AutoIncrementBadgerRepo,
	namespace string,
	refIDs []string,
	payload string,
) content.Data {
	t.Helper()

	data, err := repo.Post(
		context.Background(),
		namespace,
		refIDs,
		"", // empty ID -> auto-increment
		content.Data{
			Data: []byte(payload),
			Meta: []byte(`{}`),
		},
	)
	if err != nil {
		t.Fatalf("post auto-increment content: %v", err)
	}

	return data
}

// TestAutoIncrementLatestBadgerRepo_ThreeRefIDs covers the "3 ref IDs" case:
// POST several times under a 3-element refIDs path, then read it back
// through AutoIncrementLatestBadgerRepo by moving the 3rd ref into ID.
// Only the single latest version should come back.
func TestAutoIncrementLatestBadgerRepo_ThreeRefIDs(t *testing.T) {
	db := newTestDB(t)

	const refSize = 3

	base := NewAutoIncrement(db, "content", refSize)

	namespace := "root-node"
	refIDs := []string{"node1", "sub-node1", "sub-sub-node1"}

	const postCount = 3

	posted := make([]content.Data, 0, postCount)

	for i := 0; i < postCount; i++ {
		payload := fmt.Sprintf("payload-%d", i+1)
		posted = append(posted, postAutoIncrement(t, base, namespace, refIDs, payload))
	}

	latest := base.GetLatest()

	// Query using only the first 2 refs; the 3rd ref is moved into ID.
	queryRefIDs := refIDs[:2]
	queryID := refIDs[2]

	results, err := latest.Get(
		context.Background(),
		namespace,
		queryRefIDs,
		queryID,
	)
	if err != nil {
		t.Fatalf("get latest branch: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected to get `sub-sub-node1` latest ID")
	}

	last := results[0]
	wantLast := posted[len(posted)-1]

	if last.ID != wantLast.ID {
		t.Errorf("expected last version ID %q, got %q", wantLast.ID, last.ID)
	}

	if string(last.Data) != string(wantLast.Data) {
		t.Errorf("expected last version data %q, got %q", wantLast.Data, last.Data)
	}
}

// TestAutoIncrementLatestBadgerRepo_SingleRefID covers a 1-element refIDs
// path: POST several times, then read it back through
// AutoIncrementLatestBadgerRepo with nil refIDs and the single ref moved
// into ID. Only the single latest version should come back.
func TestAutoIncrementLatestBadgerRepo_SingleRefID(t *testing.T) {
	db := newTestDB(t)

	const refSize = 1

	base := NewAutoIncrement(db, "content", refSize)

	namespace := "root-node"
	refIDs := []string{"only-node"}

	const postCount = 4

	posted := make([]content.Data, 0, postCount)

	for i := 0; i < postCount; i++ {
		payload := fmt.Sprintf("payload-%d", i+1)
		posted = append(posted, postAutoIncrement(t, base, namespace, refIDs, payload))
	}

	latest := base.GetLatest()

	// nil refIDs; the single ref is moved into ID.
	results, err := latest.Get(
		context.Background(),
		namespace,
		nil,
		refIDs[0],
	)
	if err != nil {
		t.Fatalf("get latest branch: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected to get `only-node` latest ID")
	}

	last := results[0]
	wantLast := posted[len(posted)-1]

	if last.ID != wantLast.ID {
		t.Errorf("expected last version ID %q, got %q", wantLast.ID, last.ID)
	}

	if string(last.Data) != string(wantLast.Data) {
		t.Errorf("expected last version data %q, got %q", wantLast.Data, last.Data)
	}
}

// TestAutoIncrementLatestBadgerRepo_Stream checks that Stream surfaces the
// same single latest entry that Get returns.
func TestAutoIncrementLatestBadgerRepo_Stream(t *testing.T) {
	db := newTestDB(t)

	const refSize = 3

	base := NewAutoIncrement(db, "content", refSize)

	namespace := "root-node"
	refIDs := []string{"node1", "sub-node1", "sub-sub-node1"}

	const postCount = 5

	var wantLast content.Data

	for i := 0; i < postCount; i++ {
		payload := fmt.Sprintf("payload-%d", i+1)
		wantLast = postAutoIncrement(t, base, namespace, refIDs, payload)
	}

	latest := base.GetLatest()

	ch, err := latest.Stream(
		context.Background(),
		namespace,
		refIDs[:2],
		refIDs[2],
	)
	if err != nil {
		t.Fatalf("stream latest branch: %v", err)
	}

	var got []content.Data
	for item := range ch {
		got = append(got, item)
	}

	if len(got) != 1 {
		t.Fatalf("expected exactly 1 streamed entry (the latest), got %d", len(got))
	}

	if got[0].ID != wantLast.ID {
		t.Errorf("expected streamed entry ID %q, got %q", wantLast.ID, got[0].ID)
	}

	if string(got[0].Data) != string(wantLast.Data) {
		t.Errorf("expected streamed entry data %q, got %q", wantLast.Data, got[0].Data)
	}
}

// TestAutoIncrementLatestBadgerRepo_NotFound checks that querying a branch
// that has never been posted to returns mycontent.ErrNotFound.
func TestAutoIncrementLatestBadgerRepo_NotFound(t *testing.T) {
	db := newTestDB(t)

	const refSize = 3

	base := NewAutoIncrement(db, "content", refSize)
	latest := base.GetLatest()

	_, err := latest.Get(
		context.Background(),
		"root-node",
		[]string{"node1"},
		"never-posted",
	)
	if !errors.Is(err, mycontent.ErrNotFound) {
		t.Fatalf("expected mycontent.ErrNotFound, got %v", err)
	}
}
