package sqliteraft

import (
	"database/sql"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

type scanner interface {
	Scan(dest ...any) error
}

func (r *repository) scanRow(s scanner) (content.Data, error) {

	var d content.Data

	refs := make([]string, r.app.tableConfig.RefSize)

	dest := []any{
		&d.EventID,
		&d.Namespace,
	}

	for i := 0; i < r.app.tableConfig.RefSize; i++ {
		dest = append(dest, &refs[i])
	}

	dest = append(dest,
		&d.ID,
		&d.Data,
		&d.Meta,
	)

	if err := s.Scan(dest...); err != nil {
		if err == sql.ErrNoRows {
			return content.Data{}, content.ErrNotFound
		}
		return content.Data{}, err
	}

	d.RefIDs = refs

	return d, nil
}

func (r *repository) scanRows(rows *sql.Rows) ([]content.Data, error) {
	defer rows.Close()

	result := make([]content.Data, 0)

	for rows.Next() {
		item, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
