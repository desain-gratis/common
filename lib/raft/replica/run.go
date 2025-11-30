package replica

import (
	"fmt"

	"github.com/desain-gratis/common/example/raft-app/src/conn/clickhouse"
	"github.com/desain-gratis/common/lib/raft"
	"github.com/desain-gratis/common/lib/raft/runner"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/config"
	"github.com/rs/zerolog/log"
)

func Run[T any](c Config[T], appName string, app raft.Application) error {
	database := fmt.Sprintf("%v_%v_%v", appName, c.ShardID, c.ReplicaID)
	clickhouse.CreateDB(c.ClickHouseConfig.Address, database)

	fn := runner.New(c.ClickHouseConfig.Address, database, app)

	var target map[uint64]dragonboat.Target
	if c.internal.Bootstrap {
		target = getPeer(cfg.Host.Peer, cfg)
	}

	join := len(target) == 0

	err := dhost.StartOnDiskReplica(target, join, fn, config.Config{
		ShardID:            c.internal.shardID,
		ReplicaID:          c.internal.replicaID,
		HeartbeatRTT:       1,
		CheckQuorum:        true,
		ElectionRTT:        10,
		SnapshotEntries:    0, // todo: set to 0. let manual snapshot by cron by calling request snapshot.
		CompactionOverhead: 5,
	})

	if err != nil {
		log.Panic().Msgf("start replica: %v", err)
	}

	return nil
}
