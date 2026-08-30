package sqliteraft

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

func (r *repository) post(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
	data content.Data,
) (content.Data, error) {

	if err := r.app.validateKey(namespace, refIDs); err != nil {
		return content.Data{}, err
	}

	if ID == "" {
		return content.Data{}, mycontent.ErrInvalidKey
	}

	// Keep behavior compatible with clickhouse-raft.
	var v any

	if len(data.Data) > 0 {
		if err := json.Unmarshal(data.Data, &v); err != nil {
			return content.Data{}, err
		}
	}

	if len(data.Meta) > 0 {
		if err := json.Unmarshal(data.Meta, &v); err != nil {
			return content.Data{}, err
		}
	}

	query := fmt.Sprintf(`
INSERT INTO %s
(
	%s
)
VALUES
(
	%s
)
ON CONFLICT (%s)
DO UPDATE SET
	data=excluded.data,
	meta=excluded.meta;
`,
		r.app.tableConfig.TableName,
		r.app.insertColumns(),
		r.app.insertPlaceholders(),
		strings.Join(r.app.primaryColumns(), ", "),
	)

	args := r.app.primaryArgs(namespace, refIDs, ID)

	args = append(
		args,
		data.Data,
		data.Meta,
	)

	if _, err := r.app.db.ExecContext(
		ctx,
		query,
		args...,
	); err != nil {
		return content.Data{}, err
	}

	result, err := r.get(
		ctx,
		namespace,
		refIDs,
		ID,
	)
	if err != nil {
		return content.Data{}, err
	}

	if len(result) == 0 {
		return content.Data{}, mycontent.ErrNotFound
	}

	return result[0], nil
}
