package runner

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/desain-gratis/common/lib/notifier"
	noifier_impl "github.com/desain-gratis/common/lib/notifier/impl"
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

	ID        string
	Alias     string
	Type      string
	AppConfig T
}

var dhost *dragonboat.NodeHost
var cfg config2
var cfg3 config3

var topic notifier.Topic = noifier_impl.NewStandardTopic()

var globalClickhouse driver.Conn
var namespace string

type config3 struct {
	nhConfig    config.NodeHostConfig
	host        *dragonboat.NodeHost
	replica     map[uint64]*ReplicaConfig
	replicaByID map[string]ReplicaConfig
}

func Init() error {
	cfgFile := "/etc/dragonboat.yaml"
	return InitWithConfigFile(cfgFile)
}

func Close() error {
	if dhost != nil {
		defer dhost.Close()
		for _, repl := range cfg.Replica {
			err := dhost.StopShard(repl.ShardID)
			if err != nil {
				log.Warn().Msgf("error stopping shard %v: %v", repl.ShardID, err)
			}
		}
	}
	return nil
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

// TODO: refactor maxxing replica runner API

func WithClickhouseStorage(
	address, username, password, database string) {

	conn := Connect(address, username, password, "")
	defer conn.Close()
	namespace = database

	if err := conn.Exec(context.Background(), "CREATE DATABASE IF NOT EXISTS `"+database+"`"); err != nil {
		log.Fatal().Msgf("failed to create replica DB in clickhouse err: %v", err)
	}

	globalClickhouse = Connect(address, username, password, database)
}

func ConfigureReplica(replica map[uint64]ReplicaConfig) {
	for shardID, shardConfig := range replica {
		cfg3.replica[shardID].ShardID = shardID
		cfg3.replicaByID[shardConfig.ID] = replica[shardID]
	}
}

func GetConfig() DragonboatConfig2 {
	return DragonboatConfig2(cfg)
}

func DHost() *dragonboat.NodeHost {
	return dhost
}

func notifyLeader(dhost *dragonboat.NodeHost, shardID uint64, replicaID uint64) func(raftio.LeaderInfo) {
	return func(info raftio.LeaderInfo) {
		if info.LeaderID == replicaID {
			a, i, u, e := dhost.GetLeaderID(shardID)
			log.Info().Msgf("i'm the leader for shard: %v | %v %v %v %v", shardID, a, i, u, e)
		}
	}
}

// type Command string

const Command_UpdateLeader = "update-leader"

// UpdateRequest common Update request to state machine
type UpdateRequest struct {
	CmdName Command         `json:"cmd_name"`
	CmdVer  uint64          `json:"cmd_version"`
	Data    json.RawMessage `json:"data"`
}

func convertID(x string) (uint64, error) {
	id, err := strconv.Atoi(x)
	if err != nil {
		return 0, err
	}

	return uint64(id), nil
}
