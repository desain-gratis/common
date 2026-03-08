package runner

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/rs/zerolog/log"
)

// As root
func Connect(address, username, password, database string) driver.Conn {
	opts := &clickhouse.Options{
		Addr: []string{address},
		Auth: clickhouse.Auth{
			Username: username,
			Password: password,
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
