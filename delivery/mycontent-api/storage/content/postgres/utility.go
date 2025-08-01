package postgres

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

const (
	COLUMN_NAME_ID        = "id"
	COLUMN_NAME_DATA      = "data"
	COLUMN_NAME_META      = "meta"
	COLUMN_NAME_NAMESPACE = "namespace"
)

func generateQuery(tableName, queryType string, primaryKey PrimaryKey, upsertData UpsertData) (query string, args []any) {
	// primaryKeys is used by SELECT, UPDATE, DELETE query
	var pkColumns []string
	var primaryKeys []string
	var columns, values []string

	// init default composite columns & values
	if primaryKey.Namespace != "" {
		if primaryKey.Namespace == "*" && queryType == "SELECT" {
			// no need where clause
		} else {
			columns = append(columns, COLUMN_NAME_NAMESPACE)
			values = append(values, "'"+primaryKey.Namespace+"'")
		}
	}

	if primaryKey.ID != "" {
		columns = append(columns, COLUMN_NAME_ID)
		values = append(values, "'"+primaryKey.ID+"'")
	}

	// if only use ref_ids
	for i, refID := range primaryKey.RefIDs {
		refIDColumn := "ref_id_" + strconv.Itoa(i+1)
		refIDValue := "'" + refID + "'"
		columns = append(columns, refIDColumn)
		values = append(values, refIDValue)
	}

	for i, column := range columns {
		primaryKeys = append(primaryKeys, column+" = "+values[i])
	}

	pkColumns = columns

	switch queryType {
	case "SELECT":
		var whereClause string
		// set where clause if any
		if len(primaryKeys) > 0 {
			whereClause = ` WHERE ` + strings.Join(primaryKeys, " AND ")
		}

		query = `SELECT * FROM ` + tableName + whereClause
	case "INSERT":
		columns = append(columns, COLUMN_NAME_DATA, COLUMN_NAME_META)
		args = append(args, string(upsertData.Data), string(upsertData.Meta))
		values = append(values, `$1`, `$2`)
		query = `INSERT INTO ` + tableName + `(` + strings.Join(columns, ", ") + `) VALUES (` + strings.Join(values, ", ") + `)` +
			` ON CONFLICT (` + strings.Join(pkColumns, ",") + `) DO UPDATE SET (` + strings.Join(columns, ", ") + `) = ` + `(` + strings.Join(values, ", ") + `) RETURNING id`
	case "DELETE":
		query = `DELETE FROM ` + tableName + ` WHERE ` + strings.Join(primaryKeys, " AND ") + ` RETURNING ` + COLUMN_NAME_ID + `, ` + COLUMN_NAME_NAMESPACE + `, ` + COLUMN_NAME_DATA
	}

	query += `;`
	return
}

func mergeColumnValue(columns []string, values []interface{}) (resp content.Data, err error) {
	if len(columns) != len(values) {
		err = fmt.Errorf("column length & value length are not same")
		return
	}

	for i, column := range columns {
		tempValue := values[i]
		b, ok := tempValue.([]byte)
		var value string
		if ok {
			value = string(b)
		} else {
			value = fmt.Sprintf("%s", tempValue)
		}

		switch {
		case column == COLUMN_NAME_NAMESPACE:
			resp.Namespace = value
		case column == COLUMN_NAME_ID:
			resp.ID = value
		case strings.Contains(column, "ref_id"):
			resp.RefIDs = append(resp.RefIDs, value)
		case column == COLUMN_NAME_DATA:
			resp.Data = []byte(value)
		case column == COLUMN_NAME_META:
			resp.Meta = []byte(value)
		default:
			// log.Info().Msgf("Unrecognized column: %s", column)
		}
	}
	return
}
