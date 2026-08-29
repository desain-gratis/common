package mycontent

import "context"

type dgCtxKey string

const (
	dgEntityVersion dgCtxKey = "dg-entity-version"
)

// To get entitiy with version, only useful if repository implementation respect this
// API might change
func WithEntityVersion(ctx context.Context, version uint64) context.Context {
	return context.WithValue(ctx, dgEntityVersion, version)
}

func GetEntityVersion(ctx context.Context) uint64 {
	ver, ok := ctx.Value(dgEntityVersion).(uint64)
	if !ok {
		return 0
	}
	return ver
}
