package sqliteraft

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type TableConfig struct {
	TableName                  string
	RefSize                    int
	Versioned                  bool
	VersionedGetLimit          uint32
	VersionedUseOptimisticLock bool
}

type ContentApp struct {
	db *sql.DB

	tableConfig TableConfig
}

func NewStorageClient(
	filename string,
	tableConfig TableConfig,
) (*ContentApp, error) {

	db, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, err
	}

	// SQLite tuning.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	app := &ContentApp{
		db:          db,
		tableConfig: tableConfig,
	}

	// if err := app.initSQLite(); err != nil {
	// 	_ = db.Close()
	// 	return nil, err
	// }

	if err := app.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return app, nil
}

func (a *ContentApp) Close() error {
	if a.db == nil {
		return nil
	}
	return a.db.Close()
}

func (a *ContentApp) Repository() *repository {
	return &repository{
		app: a,
	}
}
