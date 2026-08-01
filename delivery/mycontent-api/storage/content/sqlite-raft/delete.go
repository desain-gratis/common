package sqliteraft

import (
	"context"
	"fmt"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

func (r *repository) delete(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) (content.Data, error) {

	if err := r.app.validateKey(namespace, refIDs); err != nil {
		return content.Data{}, err
	}

	if ID == "" {
		return content.Data{}, content.ErrInvalidKey
	}

	items, err := r.get(
		ctx,
		namespace,
		refIDs,
		ID,
	)
	if err != nil {
		return content.Data{}, err
	}

	if len(items) == 0 {
		return content.Data{}, content.ErrNotFound
	}

	query := fmt.Sprintf(`
DELETE FROM %s
WHERE %s
`,
		r.app.tableConfig.TableName,
		r.app.primaryWhere(),
	)

	args := r.app.primaryArgs(namespace, refIDs, ID)

	result, err := r.app.db.ExecContext(
		ctx,
		query,
		args...,
	)
	if err != nil {
		return content.Data{}, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return content.Data{}, err
	}

	if affected == 0 {
		return content.Data{}, content.ErrNotFound
	}

	return items[0], nil
}
