package raft

import (
	"context"
	"errors"

	"github.com/lni/dragonboat/v4/raftio"
	sm "github.com/lni/dragonboat/v4/statemachine"
)

// maybe propose async as well
type Proposer interface {
	Propose(ctx context.Context, value []byte) (any, error)
}

type Command string

type Entry struct {
	*sm.Entry
	Index   uint64
	Command Command
	Value   []byte

	// Replica that triggered the update
	ReplicaID *uint64
}

const raftCtxKey = "raft-ctx"

func GetRaftContext(ctx context.Context) any {
	return ctx.Value(raftCtxKey)
}

func WithRaftContext(ctx context.Context, runner any) context.Context {
	return context.WithValue(ctx, raftCtxKey, runner)
}

type EntryV2 struct {
	Index          uint64 `json:"index"`
	Term           uint64 `json:"term"`
	SourceNodeID   uint64 `json:"source_node_id"`
	SubscriptionID string `json:"subscription_id"` // or subscription ID

	Data []byte `json:"data"`
}

var (
	ErrUnsupported = errors.ErrUnsupported
	ErrTermZero    = errors.New("Term zero")
)

type ResultV2 struct {
	// or code
	Value uint64

	SubscriptionID string

	// the payload
	Data  any
	Error error
}

type Result sm.Result

type OnAfterApply func() (Result, error)

// exploring etcd raft / make it simpler compared to Application
// or just EtcdApplication
type ApplicationV2 interface {
	// Return last applied index
	// todo might separate it
	InitV2(ctx context.Context) (uint64, error)

	// Simpler API for distributed state machine
	// If return error, we will acknowledge it as applied. If you don't want, just crash the state machine.
	OnUpdateV2(ctx context.Context, entry EntryV2) (any, error)
}

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

type EventLeaderUpdate raftio.LeaderInfo
