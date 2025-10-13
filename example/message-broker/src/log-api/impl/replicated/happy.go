package replicated

import (
	"context"

	sm "github.com/lni/dragonboat/v4/statemachine"
)

type OnAfterApply func() (sm.Result, error)

type Happy interface {

	// PrepareApply is to prepare for update scoped resource
	PrepareUpdate(ctx context.Context) (context.Context, error)

	// OnUpdate but before apply
	OnUpdate(ctx context.Context, e sm.Entry) OnAfterApply

	// Apply to place the code to commit to disk or "Sync"
	Apply(ctx context.Context) error

	// Lookup
	Lookup(ctx context.Context, key interface{}) (interface{}, error)
}
