package runner

import (
	"context"

	"github.com/lni/dragonboat/v4/raftio"

	"github.com/desain-gratis/common/lib/raft"
)

var _ raftio.IRaftEventListener = &raftListener{}

type raftListener struct{}

func (r *raftListener) LeaderUpdated(info raftio.LeaderInfo) {
	if info.LeaderID == 0 || info.ShardID == 0 {
		return
	}

	topic.Broadcast(context.Background(), raft.EventLeaderUpdate(info))
}
