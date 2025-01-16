package postgres

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/desain-gratis/common/repository/content"
	"github.com/rs/zerolog/log"
)

func generateQuery(tableName, queryType string, primaryKey PrimaryKey, upsertData UpsertData) (query string) {
	// primaryKeys is used by SELECT, UPDATE, DELETE query
	var primaryKeys []string
	var columns, values []string

	// init default composite columns & values
	if primaryKey.UserID != "" {
		columns = append(columns, "user_id")
		values = append(values, "'"+primaryKey.UserID+"'")
	}

	if primaryKey.ID != "" {
		columns = append(columns, "id")
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

	switch queryType {
	case "SELECT":
		var whereClause string
		// set where clause if any
		if len(primaryKeys) > 0 {
			whereClause = ` WHERE ` + strings.Join(primaryKeys, " AND ")
		}

		query = `SELECT * FROM ` + tableName + whereClause
	case "INSERT":
		columns = append(columns, "payload")
		values = append(values, `'`+string(upsertData.PayloadJSON)+`'::jsonb`)
		query = `INSERT INTO ` + tableName + `(` + strings.Join(columns, ", ") + `) VALUES (` + strings.Join(values, ", ") + `)` + ` ON CONFLICT (` + strings.Join(columns[:len(columns)-1], ",") + `) DO UPDATE SET (` + strings.Join(columns, ", ") + `) = ` + `(` + strings.Join(values, ", ") + `) RETURNING id`
	case "UPDATE":
		arguments := generateSetArguments(upsertData)
		query = `UPDATE ` + tableName + ` SET ` + strings.Join(arguments, ", ") + ` WHERE ` + strings.Join(primaryKeys, " AND ")
	case "DELETE":
		query = `DELETE FROM ` + tableName + ` WHERE ` + strings.Join(primaryKeys, " AND ") + ` RETURNING id, user_id, payload`
	}

	query += `;`
	return
}

func generateSetArguments(upsertData UpsertData) (arguments []string) {
	for i, refID := range upsertData.RefIDs {
		arg := "ref_id_" + strconv.Itoa(i+1) + " = '" + refID + "'"
		arguments = append(arguments, arg)
	}

	if len(upsertData.PayloadJSON) > 0 {
		arg := `payload = '` + string(upsertData.PayloadJSON) + `'::jsonb`
		arguments = append(arguments, arg)
	}
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
		case column == "user_id":
			resp.UserID = value
		case column == "id":
			resp.ID = value
		case strings.Contains(column, "ref_id"):
			resp.RefIDs = append(resp.RefIDs, value)
		case column == "payload":
			resp.Data = []byte(value)
		default:
			log.Info().Msgf("Unrecognized column: %s", column)
		}
	}
	return
}
