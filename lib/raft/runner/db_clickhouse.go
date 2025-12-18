package runner

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/rs/zerolog/log"
)

func Connect(address, database string) driver.Conn {
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

	return conn
}

func createClickhouseDB(address, database string) error {
	opts := &clickhouse.Options{
		Addr: []string{address},
		Auth: clickhouse.Auth{
			Username: "default",
			Password: "default",
		},
		Settings: map[string]interface{}{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		DialTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second,
	}

	var conn driver.Conn
	var err error
	err = retry(func() error {
		conn, err = clickhouse.Open(opts)
		return err
	}, 3)
	if err != nil {
		log.Fatal().Msgf("failed to open connection base to clickhouse: %v err: %v", address, err)
	}

	defer conn.Close()

	ctx := context.Background()
	err = retry(func() error {
		return conn.Ping(ctx)
	}, 3)
	if err != nil {
		log.Fatal().Msgf("failed to open connection base to clickhouse: %v err: %v", address, err)
	}

	if err := conn.Exec(context.Background(), "CREATE DATABASE IF NOT EXISTS `"+database+"`"); err != nil {
		log.Fatal().Msgf("failed to create DB clickhouse: %v err: %v", address, err)
	}

	return nil
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
