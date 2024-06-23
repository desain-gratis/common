package pg

import (
	"database/sql"

	_ "github.com/lib/pq"
)

type DriverName string

const DRIVERNAME_POSTGRES DriverName = "postgres"

func GetConnection(conn string) (db *sql.DB, err error) {
	return sql.Open(string(DRIVERNAME_POSTGRES), conn)
}
