package replica

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/raftio"
	"github.com/rs/zerolog/log"
)

func getPeer(x map[int]string, cfg config2) map[uint64]dragonboat.Target {
	y := make(map[uint64]dragonboat.Target)
	for replicaID, peerRaftAddress := range x {
		y[uint64(replicaID)] = dragonboat.Target(peerRaftAddress)
	}

	// add our own self address
	if x != nil {
		y[cfg.Host.ReplicaID] = cfg.Host.RaftAddress
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
