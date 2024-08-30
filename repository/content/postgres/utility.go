package postgres

import (
	"strconv"
	"strings"
)

func generateQueryAndArgs(tableName, queryType string, primaryKey PrimaryKey, upsertData UpsertData) (query string) {
	// primaryKeys is used by SELECT, UPDATE, DELETE query
	var primaryKeys []string
	var columns, values []string

	// init default composite columns & values
	columns = []string{"user_id", "id"}
	values = []string{"'" + primaryKey.UserID + "'", "'" + primaryKey.ID + "'"}

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

	switch queryType {
	case "SELECT":
		query = `SELECT * FROM ` + tableName + ` WHERE ` + strings.Join(primaryKeys, " AND ")
	case "INSERT":
		columns = append(columns, "payload")
		values = append(values, `'`+upsertData.PayloadJSON+`'::jsonb`)
		query = `INSERT INTO ` + tableName + `(` + strings.Join(columns, ", ") + `) VALUES (` + strings.Join(values, ", ") + `)`
	case "UPDATE":
		arguments := generateSetArguments(upsertData)
		query = `UPDATE ` + tableName + ` SET ` + strings.Join(arguments, ", ") + ` WHERE ` + strings.Join(primaryKeys, " AND ")
	case "DELETE":
		query = `DELETE FROM ` + tableName + `WHERE ` + strings.Join(primaryKeys, " AND ")
	}

	query += `;`
	return
}

func generateSetArguments(upsertData UpsertData) (arguments []string) {
	for i, refID := range upsertData.RefIDs {
		arg := "ref_id_" + strconv.Itoa(i+1) + " = '" + refID + "'"
		arguments = append(arguments, arg)
	}

	if upsertData.PayloadJSON != "" {
		arg := `payload = '` + upsertData.PayloadJSON + `'::jsonb`
		arguments = append(arguments, arg)
	}
	return
}
