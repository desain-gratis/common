package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/config"
	"github.com/lni/dragonboat/v4/raftio"
	"github.com/rs/zerolog/log"

	logapi "github.com/desain-gratis/common/delivery/log-api"
	logapi_impl "github.com/desain-gratis/common/delivery/log-api/impl"
	logapi_sm_topic_impl "github.com/desain-gratis/common/delivery/log-api/impl/state-machine/topic"
)

type topicData struct {
	name      string
	shardID   uint64
	replicaID uint64
}

func enableBroadcaster(cfg DragonboatConfig) (*dragonboat.NodeHost, map[string]topicData) {
	listener := &raftListener{
		ShardListener: make(map[uint64]func(info raftio.LeaderInfo)),
	}

	host, err := dragonboat.NewNodeHost(config.NodeHostConfig{
		RaftAddress:       cfg.RaftAddress,
		WALDir:            cfg.WALDir,
		NodeHostDir:       cfg.NodehostDir,
		RTTMillisecond:    100,
		DeploymentID:      cfg.DeploymentID,
		RaftEventListener: listener,
	})
	if err != nil {
		log.Panic().Msgf("init nodehost %v", err)
	}

	mapping := map[string]topicData{}

	for i, fsm := range cfg.FSM {
		shardID, err := convertID(i)
		if err != nil {
			log.Error().Msgf("err id %v", i)
			continue
		}

		target := convertRaftAddress(fsm.Bootstrap, cfg) // todo: add our own address
		join := len(target) == 0
		log.Info().Msgf("target: %v join: %v", target, join)

		listener.ShardListener[shardID] = func(info raftio.LeaderInfo) {
			if info.LeaderID == cfg.ReplicaID {
				a, i, u, e := host.GetLeaderID(shardID)
				log.Info().Msgf("i'm the leader for shard: %v | %v %v %v %v", shardID, a, i, u, e)
				log.Info().Msgf("proposing to the state machine...")

				info, _ := json.Marshal(info)
				d, _ := json.Marshal(logapi_sm_topic_impl.UpdateRequest{
					CmdName: logapi_sm_topic_impl.Command_UpdateLeader,
					CmdVer:  0,
					Data:    info,
				})

				sess := host.GetNoOPSession(shardID)
				ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
				res, err := host.SyncPropose(ctx, sess, d)
				c()
				if err != nil {
					log.Error().Msgf("error propose: %v", err)
					return
				}

				l, _ := host.GetLogReader(1)
				l.NodeState()

				log.Info().Msgf("result: %v", string(res.Data))

				return
			} else {
				// sync propose as well..
			}
		}

		exitMsg := "server is closed, bye byee 🫰🏽💕 see u 🥹"
		broker := logapi_impl.NewTopic(func(key string) logapi.Subscription {
			return logapi_impl.NewSubscription(key, true, 0, &exitMsg, 2*time.Second)
		})

		smf := logapi_sm_topic_impl.New(broker)

		err = host.StartReplica(target, join, smf, config.Config{
			ShardID:            shardID,
			ReplicaID:          cfg.ReplicaID,
			HeartbeatRTT:       1,
			CheckQuorum:        true,
			ElectionRTT:        10,
			SnapshotEntries:    10, // todo: set to 0. let manual snapshot by cron by calling request snapshot.
			CompactionOverhead: 5,
		})
		if err != nil {
			log.Panic().Msgf("start replica: %v", err)
		}

		mapping[fsm.Name] = topicData{
			name:      fsm.Name,
			shardID:   shardID,
			replicaID: cfg.ReplicaID,
		}
	}

	return host, mapping
}

func convertRaftAddress(x map[string]int, cfg DragonboatConfig) map[uint64]dragonboat.Target {
	y := make(map[uint64]dragonboat.Target)
	for addr, id := range x {
		y[uint64(id)] = dragonboat.Target(addr)
	}

	// add our own address
	if x != nil {
		y[cfg.ReplicaID] = cfg.RaftAddress
	}

	return y
}

func convertID(x string) (uint64, error) {
	id, err := strconv.Atoi(x)
	if err != nil {
		return 0, err
	}

	return uint64(id), nil
}
