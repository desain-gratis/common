package main

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/rs/zerolog/log"
)

type DragonboatConfig struct {
	ReplicaID    uint64               `json:"replica_id"`
	RaftAddress  string               `json:"raft_address"`
	WALDir       string               `json:"wal_dir"`
	NodehostDir  string               `json:"nodehost_dir"`
	DeploymentID uint64               `json:"deployment_id"`
	FSM          map[string]FSMConfig `json:"fsm"`
}

type FSMConfig struct {
	Alias     string         `json:"alias"`
	Name      string         `json:"name"`
	FSMID     string         `json:"fsm_id"`
	Bootstrap map[string]int `json:"bootstrap"`
}

func initDragonboatConfig(ctx context.Context, cfgFile string) (cfg DragonboatConfig, err error) {
	f, err := os.Open(cfgFile)
	if err != nil {
		log.Panic().Msgf("failed to open file %v", cfgFile)
	}

	payload, err := io.ReadAll(f)
	if err != nil {
		log.Panic().Msgf("failed to read file %v", cfgFile)
	}

	err = json.Unmarshal(payload, &cfg)
	if err != nil {
		log.Panic().Msgf("failed to parse JSON %v", cfgFile)
	}

	return cfg, nil
}
