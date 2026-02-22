package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/desain-gratis/common/lib/raft"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/config"
	"github.com/rs/zerolog/log"
)

type ReplicaConfig struct {
	ShardID   uint64
	ReplicaID uint64

	Bootstrap bool   `yaml:"bootstrap"`
	ID        string `yaml:"id"`
	Alias     string `yaml:"alias"`
	Type      string `yaml:"type"`
	Config    string `yaml:"config"`
}

func Context[T any](ctx context.Context, appID string) (context.Context, error) {
	cfg := GetConfig()

	sc, ok := cfg.ReplicaByID[appID]
	if !ok {
		return nil, fmt.Errorf("replica config not found in context for app ID: %v", appID)
	}

	var t T
	err := json.Unmarshal([]byte(sc.Config), &t)
	if err != nil {
		log.Warn().Msgf("failed to marshal config")
	}

	// Pass everything via context to make the API clean
	ctx = context.WithValue(ctx, contextKey, RaftContext{
		ID:        sc.ID,
		ShardID:   sc.ShardID,
		ReplicaID: sc.ReplicaID,
		Type:      sc.Type,
		AppConfig: t,
		DHost:     DHost(),

		// internal state
		isBootstrap: sc.Bootstrap,

		// can add more as required
	})

	return ctx, nil
}

func RunReplica[T any](ctx context.Context, appID string, app raft.Application) (context.Context, error) {
	cfg := GetConfig()

	sc, ok := cfg.ReplicaByID[appID]
	if !ok {
		return nil, fmt.Errorf("replica config not found for app ID: %v", appID)
	}

	shardID := sc.ShardID
	replicaID := cfg.Host.ReplicaID

	SusbcribeLeadershipEvent(shardID, replicaID)

	ctx, err := Context[T](ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to build raft replica context: %v", err)
	}

	return ctx, Run(ctx, app)
}

func ForEachReplica[T any](appType string, f func(ctx context.Context) error) {
	cfg := GetConfig()

	for _, sc := range cfg.Replica {
		if sc.Type != appType {
			continue
		}

		shardID := sc.ShardID
		replicaID := cfg.Host.ReplicaID

		SusbcribeLeadershipEvent(shardID, replicaID)

		var t T
		err := json.Unmarshal([]byte(sc.Config), &t)
		if err != nil {
			log.Warn().Msgf("failed to marshal config")
		}

		// Pass everything via context to make the API clean
		ctx := context.Background()
		ctx = context.WithValue(ctx, contextKey, RaftContext{
			ID:        sc.ID,
			ShardID:   sc.ShardID,
			ReplicaID: sc.ReplicaID,
			Type:      sc.Type,
			AppConfig: t,
			DHost:     DHost(),

			// internal state
			isBootstrap: sc.Bootstrap,

			// can add more as required
		})

		// configuration related context (private to this library)
		// ctx = context.WithValue(ctx, chConnKey, )

		f(ctx)
	}
}

func Run(ctx context.Context, app raft.Application) error {
	dhost := DHost()

	cfg := GetConfig()
	raftCtx, err := GetRaftContext(ctx)
	if err != nil {
		return err
	}

	database := fmt.Sprintf("%v_%v_%v", raftCtx.ID, raftCtx.ShardID, raftCtx.ReplicaID)

	createClickhouseDB(cfg.Host.ClickHouse.Address, database)

	fn := newBaseDiskSM(cfg.Host.ClickHouse.Address, database, app)

	var target map[uint64]dragonboat.Target
	if raftCtx.isBootstrap {
		target = getPeer(cfg.Host.Peer)
	}

	join := len(target) == 0

	err = dhost.StartOnDiskReplica(target, join, fn, config.Config{
		ShardID:            raftCtx.ShardID,
		ReplicaID:          raftCtx.ReplicaID,
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

func getPeer(x map[int]string) map[uint64]dragonboat.Target {
	y := make(map[uint64]dragonboat.Target)
	for replicaID, peerRaftAddress := range x {
		y[uint64(replicaID)] = dragonboat.Target(peerRaftAddress)
	}

	return y
}
