package main

import (
	"github.com/lni/dragonboat/v4/raftio"
	"github.com/rs/zerolog/log"
)

var _ raftio.IRaftEventListener = &raftListener{}

type raftListener struct {
	ShardListener map[uint64]func(info raftio.LeaderInfo)
}

func (r *raftListener) LeaderUpdated(info raftio.LeaderInfo) {
	if info.LeaderID == 0 || info.ShardID == 0 {
		return
	}

	cb, ok := r.ShardListener[info.ShardID]
	if ok {
		cb(info)
	} else {
		log.Info().Msgf("why not found? %+v", info)
	}
}
