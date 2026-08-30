package sqliteraft

import (
	"fmt"
	"strings"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
)

func (a *ContentApp) validateKey(
	namespace string,
	refIDs []string,
) error {

	if namespace == "" {
		return mycontent.ErrInvalidKey
	}

	if len(refIDs) != a.tableConfig.RefSize {
		return mycontent.ErrInvalidKey
	}

	for _, ref := range refIDs {
		if ref == "" {
			return mycontent.ErrInvalidKey
		}
	}

	return nil
}

func (a *ContentApp) refColumns() []string {
	cols := make([]string, a.tableConfig.RefSize)

	for i := range cols {
		cols[i] = fmt.Sprintf("ref%d", i)
	}

	return cols
}

func (a *ContentApp) ownerColumns() []string {
	cols := []string{"namespace"}
	cols = append(cols, a.refColumns()...)
	return cols
}

func (a *ContentApp) primaryColumns() []string {
	cols := a.ownerColumns()
	cols = append(cols, "id")
	return cols
}

func placeholders(n int) string {
	p := make([]string, n)
	for i := range p {
		p[i] = "?"
	}
	return strings.Join(p, ", ")
}

func (a *ContentApp) insertColumns() string {
	cols := a.primaryColumns()
	cols = append(cols,
		"data",
		"meta",
	)
	return strings.Join(cols, ", ")
}

func (a *ContentApp) insertPlaceholders() string {
	return placeholders(len(a.primaryColumns()) + 2)
}

func (a *ContentApp) ownerWhere() string {

	var where []string

	where = append(where, "namespace=?")

	for i := 0; i < a.tableConfig.RefSize; i++ {
		where = append(where,
			fmt.Sprintf("ref%d=?", i),
		)
	}

	return strings.Join(where, " AND ")
}

func (a *ContentApp) primaryWhere() string {
	return a.ownerWhere() + " AND id=?"
}

func (a *ContentApp) ownerArgs(
	namespace string,
	refIDs []string,
) []any {

	args := make([]any, 0, 1+len(refIDs))

	args = append(args, namespace)

	for _, ref := range refIDs {
		args = append(args, ref)
	}

	return args
}

func (a *ContentApp) primaryArgs(
	namespace string,
	refIDs []string,
	id string,
) []any {

	args := a.ownerArgs(namespace, refIDs)
	args = append(args, id)

	return args
}

func (a *ContentApp) selectColumns() string {

	cols := []string{
		"event_id",
		"namespace",
	}

	cols = append(cols, a.refColumns()...)

	cols = append(cols,
		"id",
		"data",
		"meta",
	)

	return strings.Join(cols, ", ")
}
