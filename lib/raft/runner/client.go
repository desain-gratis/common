package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/desain-gratis/common/lib/raft"
	"github.com/lni/dragonboat/v4"
	dclient "github.com/lni/dragonboat/v4/client"
	"github.com/lni/dragonboat/v4/statemachine"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var (
	ErrRaftContextNotFound = errors.New("raft context not found")
	ErrNotReady            = errors.New("raft not ready")
)

type Client struct {
	dHost     *dragonboat.NodeHost
	sess      *dclient.Session
	replicaID uint64
}

func NewClient(ctx context.Context) (*Client, error) {
	raftCtx, err := GetRaftContext(ctx)
	if err != nil {
		return nil, ErrRaftContextNotFound
	}
	return &Client{
		dHost:     raftCtx.DHost,
		sess:      raftCtx.DHost.GetNoOPSession(raftCtx.ShardID),
		replicaID: raftCtx.ReplicaID,
	}, nil
}

func (c *Client) Publish(ctx context.Context, command raft.Command, msg any) ([]byte, uint64, error) {
	value, err := json.Marshal(msg)
	if err != nil {
		return nil, 1, fmt.Errorf("failed to marshal msg to raft: %w (%v)", err, msg)
	}

	cmd := Command{
		Command:   command,
		ReplicaID: &c.replicaID,
		Value:     value,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, 1, fmt.Errorf("failed to marshal msg to raft: %w (%v)", err, string(data))
	}

	var attempts int
	var res statemachine.Result
	for range 3 {
		attempts++
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res, err = c.dHost.SyncPropose(ctx, c.sess, data)
		if err == nil {
			cancel()
			break
		}
		cancel()
		time.Sleep(500 * time.Millisecond * time.Duration(attempts*2))
	}

	if attempts >= 3 {
		return nil, 1, fmt.Errorf("maximum number of attempt (3) reached: %w (%w)", err, ErrNotReady)
	}

	// TODO: consider move this error handling to specific usecase (outside of the library)
	if res.Value > 0 {
		return nil, res.Value, fmt.Errorf("raft app: '%v'", string(res.Data))
	}

	return res.Data, 0, nil
}

func (c *Client) Query(ctx context.Context, msg any) (any, error) {
	var res any
	var errg error
	for range 3 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		tmpRes, err := c.dHost.SyncRead(ctx, c.sess.ShardID, msg)
		if err == nil {
			res = tmpRes
			errg = nil
			cancel()
			break
		}
		errg = err
		cancel()
		if errors.Is(err, content.ErrInvalidKey) || errors.Is(err, content.ErrNotFound) {
			return nil, err
		}

		time.Sleep(500 * time.Millisecond)
	}

	return res, errg
}
