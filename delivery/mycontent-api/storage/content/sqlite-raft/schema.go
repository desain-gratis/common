package sqliteraft

import (
	"fmt"
	"strings"
)

func (a *ContentApp) initSchema() error {
	if err := a.createContentTable(); err != nil {
		return err
	}

	if err := a.createIndexes(); err != nil {
		return err
	}

	return nil
}

func (a *ContentApp) createContentTable() error {
	var sb strings.Builder

	fmt.Fprintf(&sb, `
CREATE TABLE IF NOT EXISTS %s (
	event_id INTEGER PRIMARY KEY AUTOINCREMENT,
	namespace TEXT NOT NULL,
`, a.tableConfig.TableName)

	for i := 0; i < a.tableConfig.RefSize; i++ {
		fmt.Fprintf(&sb, "	ref%d TEXT NOT NULL,\n", i)
	}

	sb.WriteString(`
	id TEXT NOT NULL,

	data BLOB NOT NULL,
	meta BLOB NOT NULL
);
`)

	_, err := a.db.Exec(sb.String())
	return err
}

func (a *ContentApp) createIndexes() error {

	cols := []string{"namespace"}

	for i := 0; i < a.tableConfig.RefSize; i++ {
		cols = append(cols, fmt.Sprintf("ref%d", i))
	}

	ownerCols := strings.Join(cols, ", ")

	primaryCols := ownerCols + ", id"

	stmts := []string{
		fmt.Sprintf(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_%s_primary
ON %s (%s);
`,
			a.tableConfig.TableName,
			a.tableConfig.TableName,
			primaryCols,
		),

		fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS idx_%s_owner
ON %s (%s);
`,
			a.tableConfig.TableName,
			a.tableConfig.TableName,
			ownerCols,
		),
	}

	for _, stmt := range stmts {
		if _, err := a.db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}
