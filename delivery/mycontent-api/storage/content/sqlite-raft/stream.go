package sqliteraft

import (
	"context"
	"fmt"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

func (r *repository) stream(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) (<-chan content.Data, error) {

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

	ch := make(chan content.Data)

	go func() {
		defer close(ch)
		defer rows.Close()

		for rows.Next() {

			item, err := r.scanRow(rows)
			if err != nil {
				return
			}

			select {
			case <-ctx.Done():
				return

			case ch <- item:
			}
		}
	}()

	return ch, nil
}
