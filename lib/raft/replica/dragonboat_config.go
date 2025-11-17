package replica

import (
	"context"
	"encoding/json"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type DragonboatConfig struct {
	ReplicaID    uint64                  `json:"replica_id"`
	RaftAddress  string                  `json:"raft_address"`
	WALDir       string                  `json:"wal_dir"`
	NodehostDir  string                  `json:"nodehost_dir"`
	DeploymentID uint64                  `json:"deployment_id"`
	Shard        map[string]*shardConfig `json:"shard"`
}

type shardConfig struct {
	ShardID    uint64           `json:"shard_id"`
	ReplicaID  uint64           `json:"replica_id"`
	Alias      string           `json:"alias"`
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	Config     json.RawMessage  `json:"config"`
	ClickHouse ClickHouseConfig `json:"clickhouse"`
	Bootstrap  map[int]string   `json:"bootstrap"`
}

func initDragonboatConfig(_ context.Context) (cfg config2, err error) {
	cfgFile := "/etc/dragonboat.yaml"
	dc := os.Getenv("DC")
	if dc != "" {
		cfgFile = dc
	}

	f, err := os.Open(cfgFile)
	if err != nil {
		log.Panic().Msgf("failed to open file %v", cfgFile)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(f)
	if err != nil {
		log.Panic().Msgf("failed to read config %v %v", cfgFile, err)
	}

	log.Info().Msgf("reading config: %v", cfgFile)
	var c config2
	err = viper.Unmarshal(&c)
	if err != nil {
		log.Panic().Msgf("failed to unmarshal config %v %v", cfgFile, err)
	}

	log.Info().Msgf("config: %+v", c)

	return c, nil
}

type config2 struct {
	Host    HostConfig                `mapstructure:"host"`
	Replica map[string]*ReplicaConfig `mapstructure:"replica"`
}

type HostConfig struct {
	ReplicaID    uint64           `mapstructure:"replica_id"`
	RaftAddress  string           `mapstructure:"raft_address"`
	WALDir       string           `mapstructure:"wal_dir"`
	NodehostDir  string           `mapstructure:"nodehost_dir"`
	DeploymentID uint64           `mapstructure:"deployment_id"`
	ClickHouse   ClickHouseConfig `mapstructure:"clickhouse"`
	Peer         map[int]string   `mapstructure:"peer"`
}

type ReplicaConfig struct {
	shardID   uint64
	replicaID uint64

	Bootstrap bool   `yaml:"bootstrap"`
	ID        string `yaml:"id"`
	Alias     string `yaml:"alias"`
	Type      string `yaml:"type"`
	Config    string `yaml:"config"`
}
