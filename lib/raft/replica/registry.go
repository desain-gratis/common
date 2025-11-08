package replica

import (
	"context"
	"encoding/json"

	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/config"
	"github.com/lni/dragonboat/v4/raftio"
	"github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

type Config[T any] struct {
	internal *ReplicaConfig

	Host      *dragonboat.NodeHost
	ShardID   uint64
	ReplicaID uint64

	ID        string
	Alias     string
	Type      string
	AppConfig T
}

func (c *Config[T]) StartOnDiskReplica(fn statemachine.CreateOnDiskStateMachineFunc) error {
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

func (c *Config[T]) StartReplica(fn statemachine.CreateStateMachineFunc) error {
	var target map[uint64]dragonboat.Target
	if c.internal.Bootstrap {
		target = getPeer(cfg.Host.Peer, cfg)
	}

	join := len(target) == 0

	err := dhost.StartReplica(target, join, fn, config.Config{
		ShardID:            c.internal.shardID,
		ReplicaID:          c.internal.replicaID,
		HeartbeatRTT:       1,
		CheckQuorum:        true,
		ElectionRTT:        10,
		SnapshotEntries:    10, // todo: set to 0. let manual snapshot by cron by calling request snapshot.
		CompactionOverhead: 5,
	})

	if err != nil {
		log.Panic().Msgf("start replica: %v", err)
	}

	return nil
}

var dhost *dragonboat.NodeHost
var cfg config2
var listener = &raftListener{
	ShardListener: make(map[uint64]func(info raftio.LeaderInfo)),
}

func Init() error {
	ncfg, err := initDragonboatConfig(context.Background())
	if err != nil {
		return err
	}

	log.Info().Msgf("%+v", ncfg)

	nhost, err := dragonboat.NewNodeHost(config.NodeHostConfig{
		RaftAddress:       ncfg.Host.RaftAddress,
		WALDir:            ncfg.Host.WALDir,
		NodeHostDir:       ncfg.Host.NodehostDir,
		RTTMillisecond:    100,
		DeploymentID:      ncfg.Host.DeploymentID,
		RaftEventListener: &raftListener{},
	})
	if err != nil {
		log.Panic().Msgf("init nodehost %v", err)
	}

	dhost = nhost
	cfg = ncfg

	for i, shardConfig := range cfg.Replica {
		log.Info().Msgf("configuring sm: %v", shardConfig.ID)
		shardID, err := convertID(i)
		if err != nil {
			log.Error().Msgf("err id %v", i)
			continue
		}

		cfg.Replica[i].shardID = shardID
		cfg.Replica[i].replicaID = cfg.Host.ReplicaID
	}

	return nil
}

func ForEachType[T any](appType string, f func(config Config[T]) error) {
	for _, sc := range cfg.Replica {
		if sc.Type != appType {
			continue
		}

		shardID := sc.shardID

		listener.ShardListener[shardID] = notifyLeader(dhost, shardID, cfg.Host.ReplicaID)

		c := Config[T]{
			internal:  sc,
			Host:      dhost,
			ShardID:   sc.shardID,
			ReplicaID: cfg.Host.ReplicaID,
			ID:        sc.ID,
			Alias:     sc.Alias,
			Type:      sc.Type,
		}

		var t T
		err := json.Unmarshal([]byte(sc.Config), &t)
		if err != nil {
			log.Error().Msgf("failed to read config")
			continue
		}

		c.AppConfig = t

		f(c)
	}
}
