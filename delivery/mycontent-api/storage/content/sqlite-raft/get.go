package sqliteraft

import (
	"context"
	"fmt"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

func (r *repository) get(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) ([]content.Data, error) {

	if err := r.app.validateKey(namespace, refIDs); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
SELECT
	%s
FROM %s
WHERE %s`,
		r.app.selectColumns(),
		r.app.tableConfig.TableName,
		r.app.ownerWhere(),
	)

	args := r.app.ownerArgs(namespace, refIDs)

	if ID != "" {
		query += "\nAND id=?"
		args = append(args, ID)
	}

	query += "\nORDER BY event_id ASC"

	rows, err := r.app.db.QueryContext(
		ctx,
		query,
		args...,
	)
	if err != nil {
		return nil, err
	}

	return r.scanRows(rows)
}
