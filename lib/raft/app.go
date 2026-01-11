package raft

import (
	"context"
	"errors"

	sm "github.com/lni/dragonboat/v4/statemachine"
)

type Entry struct {
	*sm.Entry
	Index   uint64
	Command string
	Value   []byte
}

var ErrUnsupported = errors.ErrUnsupported

type Result sm.Result

type OnAfterApply func() (Result, error)

// Application represents a dragonboat state machine application
type Application interface {
	// Init
	Init(ctx context.Context) error

	// PrepareApply is to prepare for update scoped resource
	PrepareUpdate(ctx context.Context) (context.Context, context.CancelFunc, error)

	// OnUpdate but before apply
	// OnUpdate(ctx context.Context, e Entry) OnAfterApply

	//make it easier for everyone..
	OnUpdate(ctx context.Context, e Entry) (OnAfterApply, error)

	// Apply to place the code to commit to disk or "Sync"
	Apply(ctx context.Context) error

	// Lookup
	Lookup(ctx context.Context, key interface{}) (interface{}, error)
}
