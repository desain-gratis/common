package chdb

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var _ content.Repository = (*AutoIncrementRepo)(nil)

// maxCounterCacheSize bounds the in-memory counter cache. When exceeded the
// cache is simply dropped and lazily reloaded from the counter table.
const maxCounterCacheSize = 100_000

// AutoIncrementRepo is a KV repository (see handler) where the ID of each
// entity is an auto-incremented integer, scoped per branch (namespace + refIDs).
//
// Besides the data table, it keeps a second table "<table>_latest" which is an
// append-only log of allocations: (event_id, namespace, ref_id_1..N, latest).
// The latest ID of a branch is max(latest), and is cached in memory.
//
// Post semantics:
//   - ID == "": allocates latest+1 and stores the data under it.
//   - ID != "": modifies existing data. ID must be a canonical positive integer
//     and must not be higher than the branch's current latest ID.
//
// Allocation is idempotent with respect to Data.EventID (the raft log index):
// re-applying the same raft entry after a restart returns the ID that was
// already allocated for it instead of allocating a new one.
//
// Get, Stream and Delete are inherited from the KV handler.
type AutoIncrementRepo struct {
	*handler
	counterTable string

	mu    sync.Mutex
	cache map[string]uint64 // branch -> latest allocated ID
}

func NewAutoIncrement(ctx context.Context, db *sql.DB, tableName string, refSize int) (*AutoIncrementRepo, error) {
	h, err := New(ctx, db, tableName, refSize)
	if err != nil {
		return nil, err
	}

	r := &AutoIncrementRepo{
		handler:      h,
		counterTable: tableName + "_latest",
		cache:        make(map[string]uint64),
	}

	if err := r.initCounterTable(ctx); err != nil {
		return nil, fmt.Errorf("failed to execute DDL for table %v: %w", r.counterTable, err)
	}

	return r, nil
}

// branch columns: namespace, ref_id_1..N (everything but the last key col "id")
func (r *AutoIncrementRepo) branchCols() []string {
	return r.keyCols[:len(r.keyCols)-1]
}

func (r *AutoIncrementRepo) initCounterTable(ctx context.Context) error {
	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS " + r.counterTable + " (\n")
	b.WriteString("\tevent_id UInt64,\n")
	for _, c := range r.branchCols() {
		b.WriteString("\t" + c + " String,\n")
	}
	b.WriteString("\tlatest UInt64\n")
	b.WriteString(") ENGINE = MergeTree ORDER BY (" + strings.Join(r.branchCols(), ", ") + ", event_id)")

	_, err := r.db.ExecContext(ctx, b.String())
	return err
}

func (r *AutoIncrementRepo) Post(ctx context.Context, namespace string, refIDs []string, ID string, input content.Data) (content.Data, error) {
	// the ID is allowed to be empty here (it will be allocated)
	if len(refIDs) != r.refSize || namespace == "" || namespace == "*" {
		return content.Data{}, fmt.Errorf("%w: incomplete reference", mycontent.ErrInvalidKey)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if input.EventID == 0 {
		input.EventID = r.nextEventID()
	}

	key := cacheKey(namespace, refIDs)

	cur, cached := r.cache[key]
	if !cached {
		var err error
		cur, err = r.queryLatest(ctx, namespace, refIDs, nil)
		if err != nil {
			return content.Data{}, err
		}
	}

	if ID == "" {
		id, newLatest, err := r.allocate(ctx, namespace, refIDs, input.EventID, cur, cached)
		if err != nil {
			return content.Data{}, err
		}
		ID = strconv.FormatUint(id, 10)
		cur = newLatest
	} else {
		n, err := parseAutoIncrementID(ID)
		if err != nil {
			return content.Data{}, err
		}
		if n > cur {
			return content.Data{}, fmt.Errorf(
				"%w: id %v is higher than the current version %v", mycontent.ErrInvalidKey, n, cur)
		}
	}

	r.setCache(key, cur)

	// counter is written first: if we crash before the data is written, replaying
	// the same event re-uses the allocated ID (see allocate).
	return r.handler.Post(ctx, namespace, refIDs, ID, input)
}

// allocate returns the ID for this event and the branch's new latest value.
func (r *AutoIncrementRepo) allocate(
	ctx context.Context, namespace string, refIDs []string, eventID, cur uint64, cached bool,
) (id, latest uint64, err error) {
	// Cold cache means we (re)started: this event may have been applied before
	// (raft replay). If so, re-use the ID it was given.
	if !cached {
		prev, err := r.queryLatest(ctx, namespace, refIDs, &eventID)
		if err != nil {
			return 0, 0, err
		}
		if prev > 0 {
			return prev, cur, nil
		}
	}

	next := cur + 1

	cols := make([]string, 0, len(r.keyCols)+2)
	args := make([]any, 0, len(r.keyCols)+2)

	cols = append(cols, "event_id", "namespace")
	args = append(args, eventID, namespace)
	for i, ref := range refIDs {
		cols = append(cols, "ref_id_"+strconv.Itoa(i+1))
		args = append(args, ref)
	}
	cols = append(cols, "latest")
	args = append(args, next)

	q := `INSERT INTO ` + r.counterTable + ` (` + strings.Join(cols, ", ") + `) VALUES (` +
		strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + `)`

	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return 0, 0, err
	}

	return next, next, nil
}

// queryLatest returns the highest allocated ID of the branch (0 if none).
// If eventID is given, only the allocation made by that event is considered.
func (r *AutoIncrementRepo) queryLatest(ctx context.Context, namespace string, refIDs []string, eventID *uint64) (uint64, error) {
	var b strings.Builder
	b.WriteString(`SELECT max(latest) FROM ` + r.counterTable + ` WHERE namespace = ?`)
	args := []any{namespace}

	for i, ref := range refIDs {
		b.WriteString(` AND ref_id_` + strconv.Itoa(i+1) + ` = ?`)
		args = append(args, ref)
	}
	if eventID != nil {
		b.WriteString(` AND event_id = ?`)
		args = append(args, *eventID)
	}

	var latest uint64
	if err := r.db.QueryRowContext(ctx, b.String(), args...).Scan(&latest); err != nil {
		return 0, err
	}

	return latest, nil
}

func (r *AutoIncrementRepo) setCache(key string, latest uint64) {
	if len(r.cache) >= maxCounterCacheSize {
		r.cache = make(map[string]uint64)
	}
	r.cache[key] = latest
}

func cacheKey(namespace string, refIDs []string) string {
	return namespace + "\x00" + strings.Join(refIDs, "\x00")
}

// parseAutoIncrementID accepts only canonical positive integers ("7", not "07" or "+7"),
// so that an ID maps to exactly one key.
func parseAutoIncrementID(ID string) (uint64, error) {
	n, err := strconv.ParseUint(ID, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != ID {
		return 0, fmt.Errorf("%w: id %q is not a valid auto-increment id", mycontent.ErrInvalidKey, ID)
	}
	return n, nil
}
