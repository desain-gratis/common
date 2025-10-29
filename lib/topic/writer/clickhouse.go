package writer

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type OnAfterCommit func() ([]byte, error)

type BatchFn func(driver.Batch, any) error

type TableDefs struct {
	DDL       string
	BatchStmt string
	BatchFn   BatchFn
}

type EventWriter[T any] interface {
	// allows write & publish using the same struct
	Write(t T)
	Publish(t T)
}

func Get[T any](name string) EventWriter[T] {
	return nil
}

type ClickhouseWriter interface {
	Load(state []byte)
	Save() []byte
	GetTables() map[string]TableDefs
	Update(batch map[string]any, data []byte) OnAfterCommit
	Query(conn driver.Conn, query any) (any, error)
}

func TES() {
	ggwp := Get[*eventPurchased]("ggwp")

	t := &eventPurchased{}

	ggwp.Write(t)

	ggwp.Publish(t)
}

type eventPurchased struct {
	eventID   uint64
	timestamp time.Time
	userID    uint64
	itemID    uint64
}
