package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	cccc "github.com/desain-gratis/common/example/user-profile/src/conn/clickhouse"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

var POSTGRES_SUITE_API *sqlx.DB
var CLICKHOUSE_API driver.Conn

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

func GET_CLICKHOUSE_API() (driver.Conn, error) {
	address := fmt.Sprintf("%v", CONFIG["clickhouse.example.address"])
	database := fmt.Sprintf("%v", CONFIG["clickhouse.example.database"])
	cccc.CreateDB(address, database)
	opts := &clickhouse.Options{
		Addr: []string{address},
		Auth: clickhouse.Auth{
			Username: "default",
			Password: "default",
			Database: database,
		},
		Settings: map[string]interface{}{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		// DialTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second,
		// MaxOpenConns: 2048,
	}

	var conn driver.Conn
	var err error
	err = retry(func() error {
		conn, err = clickhouse.Open(opts)
		return err
	}, 3)

	ctx := context.Background()
	err = retry(func() error {
		return conn.Ping(ctx)
	}, 3)

	if err != nil {
		log.Fatal().Msgf("failed to open connection base to clickhouse: %v  err: %v", address, err)
	}

	log.Info().Msgf("✅ Connected to ClickHouse")

	return conn, nil
}

func retry(fn func() error, times int) error {
	var attempt int

	var err error
	for {
		attempt++
		err = fn()
		if err == nil || attempt >= times {
			break
		}
		time.Sleep(1 * time.Second)
	}

	return err
}
