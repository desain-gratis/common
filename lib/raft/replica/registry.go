package replica

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/config"
	"github.com/lni/dragonboat/v4/raftio"
	"github.com/rs/zerolog/log"
)

type Config[T any] struct {
	internal *ReplicaConfig

	Host      *dragonboat.NodeHost
	ShardID   uint64
	ReplicaID uint64

	ID               string
	Alias            string
	Type             string
	ClickHouseConfig ClickHouseConfig
	AppConfig        T
}

type ClickHouseConfig struct {
	Address string `yaml:"address"`
}

var dhost *dragonboat.NodeHost
var cfg config2
var listener = &raftListener{
	ShardListener: make(map[uint64]func(info raftio.LeaderInfo)),
}

func Init() error {
	cfgFile := "/etc/dragonboat.yaml"
	return InitWithConfigFile(cfgFile)
}

func InitWithConfigFile(cfgFile string) error {
	ncfg, err := initDragonboatConfigWithFile(context.Background(), cfgFile)
	if err != nil {
		return err
	}

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

	cfg.ReplicaByID = make(map[string]ReplicaConfig)

	for i, shardConfig := range cfg.Replica {
		log.Debug().Msgf("configuring sm: %v", shardConfig.ID)
		shardID, err := convertID(i)
		if err != nil {
			log.Error().Msgf("err id %v", i)
			continue
		}

		cfg.Replica[i].ShardID = shardID
		cfg.Replica[i].ReplicaID = ncfg.Host.ReplicaID

		cfg.ReplicaByID[shardConfig.ID] = *cfg.Replica[i]
	}

	return nil
}

func GetConfig() DragonboatConfig2 {
	return DragonboatConfig2(cfg)
}

func DHost() *dragonboat.NodeHost {
	return dhost
}

func SusbcribeLeadershipEvent(shardID, replicaID uint64) {
	// todo: add lock etc.
	listener.ShardListener[shardID] = notifyLeader(dhost, shardID, replicaID)
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

type Command string

const Command_UpdateLeader = "update-leader"

// UpdateRequest common Update request to state machine
type UpdateRequest struct {
	CmdName Command         `json:"cmd_name"`
	CmdVer  uint64          `json:"cmd_version"`
	Data    json.RawMessage `json:"data"`
}
