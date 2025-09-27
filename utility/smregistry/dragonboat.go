package smregistry

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/config"
	"github.com/lni/dragonboat/v4/raftio"
	"github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

type StateMachineFunction[T any] func(dhost *dragonboat.NodeHost, smConfig ShardConfig, appConfig T) statemachine.CreateStateMachineFunc
type stateMachineFunction func(dhost *dragonboat.NodeHost, smConfig ShardConfig, appConfig any) statemachine.CreateStateMachineFunc

type dragonboatRegistry struct {
	cfg         DragonboatConfig
	registry    map[string]stateMachineFunction
	cfgParser   map[string]func(json.RawMessage) (any, error)
	registryExt map[string]ExtensionFunction
}

func NewDragonboat(ctx context.Context, configPath string) (*dragonboatRegistry, error) {
	cfg, err := initDragonboatConfig(ctx, configPath)
	if err != nil {
		return nil, err
	}

	return &dragonboatRegistry{
		cfg:         cfg,
		registry:    make(map[string]stateMachineFunction),
		cfgParser:   make(map[string]func(json.RawMessage) (any, error)),
		registryExt: make(map[string]ExtensionFunction),
	}, nil
}

func (r *dragonboatRegistry) Start(ctx context.Context) {
	listener := &raftListener{
		ShardListener: make(map[uint64]func(info raftio.LeaderInfo)),
	}

	dhost, err := dragonboat.NewNodeHost(config.NodeHostConfig{
		RaftAddress:       r.cfg.RaftAddress,
		WALDir:            r.cfg.WALDir,
		NodeHostDir:       r.cfg.NodehostDir,
		RTTMillisecond:    100,
		DeploymentID:      r.cfg.DeploymentID,
		RaftEventListener: listener,
	})
	if err != nil {
		log.Panic().Msgf("init nodehost %v", err)
	}

	for i, shardConfig := range r.cfg.Shard {
		log.Info().Msgf("starting sm: %v", shardConfig.Name)
		shardID, err := convertID(i)
		if err != nil {
			log.Error().Msgf("err id %v", i)
			continue
		}

		shardConfig.ShardID = shardID
		shardConfig.ReplicaID = r.cfg.ReplicaID

		target := convertRaftAddress(shardConfig.Bootstrap, r.cfg) // todo: add our own address
		join := len(target) == 0
		log.Info().Msgf("target: %v join: %v", target, join)

		listener.ShardListener[shardID] = notifyLeader(dhost, shardID, r.cfg.ReplicaID)

		// let's use all in memory state machine first

		smf, ok := r.registry[shardConfig.Type]
		if !ok {
			log.Error().Msgf("type not found for smf: %v", shardConfig.Type)
			continue
		}

		appConfigParser, ok := r.cfgParser[shardConfig.Type]
		if !ok {
			log.Error().Msgf("type not found for parser: %v", shardConfig.Type)
			continue
		}

		appConfig, err := appConfigParser(shardConfig.Config)
		if err != nil {
			log.Error().Msgf("failed to read config not found %v", shardConfig.Type)
			continue
		}

		log.Info().Msgf("app config: %+v", appConfig)

		dragonboatSM := smf(dhost, shardConfig, appConfig)

		err = dhost.StartReplica(target, join, dragonboatSM, config.Config{
			ShardID:            shardID,
			ReplicaID:          r.cfg.ReplicaID,
			HeartbeatRTT:       1,
			CheckQuorum:        true,
			ElectionRTT:        10,
			SnapshotEntries:    10, // todo: set to 0. let manual snapshot by cron by calling request snapshot.
			CompactionOverhead: 5,
		})
		if err != nil {
			log.Panic().Msgf("start replica: %v", err)
		}

		// register others
		ext, ok := r.registryExt[shardConfig.Type]
		if !ok {
			log.Error().Msgf("type not found for ext: %v", shardConfig.Type)
			continue
		}
		ext(dhost, shardConfig)
	}
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

func notifyLeader(dhost *dragonboat.NodeHost, shardID uint64, replicaID uint64) func(raftio.LeaderInfo) {
	return func(info raftio.LeaderInfo) {
		if info.LeaderID == replicaID {
			a, i, u, e := dhost.GetLeaderID(shardID)
			log.Info().Msgf("i'm the leader for shard: %v | %v %v %v %v", shardID, a, i, u, e)
			log.Info().Msgf("proposing to the state machine...")

			info, _ := json.Marshal(info)
			d, _ := json.Marshal(UpdateRequest{
				CmdName: Command_UpdateLeader,
				CmdVer:  0,
				Data:    info,
			})

			sess := dhost.GetNoOPSession(shardID)
			ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
			res, err := dhost.SyncPropose(ctx, sess, d)
			c()
			if err != nil {
				log.Error().Msgf("error propose: %v", err)
				return
			}

			l, _ := dhost.GetLogReader(1)
			l.NodeState()

			log.Info().Msgf("result: %v", string(res.Data))

			return
		} else {
			// sync propose as well..
		}
	}
}

// standard type to communicate with the state machine

type Command string

const Command_UpdateLeader = "update-leader"

// UpdateRequest common Update request to state machine
type UpdateRequest struct {
	CmdName Command         `json:"cmd_name"`
	CmdVer  uint64          `json:"cmd_version"`
	Data    json.RawMessage `json:"data"`
}
