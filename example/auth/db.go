package main

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

var POSTGRES_SUITE_API *sqlx.DB

// var CLICKHOUSE_SUITE_API

func GET_POSTGRES_SUITE_API() (*sqlx.DB, bool) {
	if POSTGRES_SUITE_API != nil {
		return POSTGRES_SUITE_API, true
	}

	var err error
	connString := fmt.Sprintf("user=%s dbname=%s sslmode=disable password=%s host=%s",
		CONFIG["postgres.example.user"],
		CONFIG["postgres.example.database_name"],
		CONFIG["postgres.example.password"],
		CONFIG["postgres.example.host"],
	)

	POSTGRES_SUITE_API, err = sqlx.Connect(
		"postgres",
		connString,
	)
	if err != nil {
		log.Fatal().Msgf("failed to connect postgres db")
	}

	return POSTGRES_SUITE_API, POSTGRES_SUITE_API != nil
}
