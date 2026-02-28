package runner

import (
	"context"
	"time"

	"github.com/desain-gratis/common/lib/notifier"
	"github.com/desain-gratis/common/lib/notifier/impl"
	"github.com/desain-gratis/common/lib/raft"
)

// "replicaID" that maps shard UID
func WaitReady(ctx context.Context) (raft.EventLeaderUpdate, notifier.Subscription, error) {
	raftCtx, err := GetRaftContext(ctx)
	if err != nil {
		return raft.EventLeaderUpdate{}, nil, ErrRaftContextNotFound
	}

	sub, err := topic.Subscribe(ctx, impl.NewStandardSubscriber(func(a any) bool {
		elu, ok := a.(raft.EventLeaderUpdate)
		if !ok {
			return true
		}
		if elu.ReplicaID != raftCtx.ReplicaID || elu.ShardID != raftCtx.ShardID {
			return true
		}
		return false
	}))

	// make sure that we don't lose message;
	// we can always check for duplicate by comparing the "term"
	sub.Start()

	var attempts int
	for {
		attempts++
		leaderID, term, ready, err := raftCtx.DHost.GetLeaderID(raftCtx.ShardID)
		if err == nil && ready {
			if term == 0 {
				return raft.EventLeaderUpdate{}, nil, raft.ErrTermZero
			}
			return raft.EventLeaderUpdate{
				ShardID:   raftCtx.ShardID,
				ReplicaID: raftCtx.ReplicaID,
				Term:      term,
				LeaderID:  leaderID,
			}, sub, nil
		}
		time.Sleep(100 * time.Millisecond * time.Duration(1<<attempts))
	}
}
