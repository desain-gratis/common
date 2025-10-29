package topic

import "context"

type OnAfterCommit func() ([]byte, error)

type TopicWriter interface {
	// PrepareSchema
	PrepareSchema(ctx context.Context) error

	// PrepareApply is to prepare for update scoped resource
	PrepareUpdate(ctx context.Context) (context.Context, error)

	// OnUpdate but before apply
	OnUpdate(ctx context.Context, data []byte) OnAfterCommit

	// Apply to place the code to commit to disk or "Sync"
	Apply(ctx context.Context) error
}
