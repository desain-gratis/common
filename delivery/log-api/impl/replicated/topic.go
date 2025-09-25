package replicated

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	logapi "github.com/desain-gratis/common/delivery/log-api"
	notifierapi "github.com/desain-gratis/common/delivery/log-api"
	"github.com/lni/dragonboat/v4"
)

var _ notifierapi.Topic = &replicatedTopic{}

// replicatedTopic using dragonboat library
type replicatedTopic struct {
	shardID   uint64
	replicaID uint64
	dhost     *dragonboat.NodeHost
}

type DragonboatShardConfig struct {
	ShardID   uint64
	ReplicaID uint64
}

// New extends the default topic with replication ability using dragonboat raft library
func New(dhost *dragonboat.NodeHost, shardID uint64, replicaID uint64) notifierapi.Topic {
	return &replicatedTopic{
		shardID:   shardID,
		replicaID: replicaID,
		dhost:     dhost,
	}
}

func (r *replicatedTopic) Subscribe() (logapi.Subscription, error) {
	sess := r.dhost.GetNoOPSession(r.shardID)

	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()

	// 1. get & register local instance of the subscription
	v, err := r.dhost.SyncRead(ctx, r.shardID, QuerySubscribe{})
	if err != nil {
		return nil, err
	}

	l, ok := v.(notifierapi.Subscription)
	if !ok {
		return nil, err
	}

	// 2. start consuming data from the subscription
	data, err := json.Marshal(StartSubscriptionData{
		SubscriptionID: l.ID(),
		ReplicaID:      r.replicaID,
	})
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(UpdateRequest{
		CmdName: Command_StartSubscription,
		Data:    data,
	})

	_, err = r.dhost.SyncPropose(ctx, sess, payload)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (r *replicatedTopic) GetSubscription(id string) (logapi.Subscription, error) {
	return nil, errors.New("not supported")
}

func (r *replicatedTopic) RemoveSubscription(id string) error {
	return errors.New("not supported")
}

func (r *replicatedTopic) Broadcast(ctx context.Context, message any) error {
	payload, ok := message.([]byte)
	if !ok {
		return errors.New("should be []byte")
	}

	sess := r.dhost.GetNoOPSession(r.shardID)

	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()

	_, err := r.dhost.SyncPropose(ctx, sess, payload)
	if err != nil {
		return fmt.Errorf("%w: %w", errors.New("failed propose to raft sm"), err)
	}

	// todo: parses result

	return nil
}
