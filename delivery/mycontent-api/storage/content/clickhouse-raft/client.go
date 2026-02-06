package clickhouseraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	raft_runner "github.com/desain-gratis/common/lib/raft/runner"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/client"
	"github.com/lni/dragonboat/v4/statemachine"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var _ content.Repository = &mycontentClient{}

var (
	ErrNotReady = errors.New("raft not ready")
)

type mycontentClient struct {
	tableName string
	DHost     *dragonboat.NodeHost
	Sess      *client.Session
	ReplicaID uint64
}

func NewStorageClient(ctx context.Context, tableName string) *mycontentClient {
	raftCtx := raft_runner.GetRaftContext(ctx)
	return &mycontentClient{
		tableName: tableName,
		DHost:     raftCtx.DHost,
		Sess:      raftCtx.DHost.GetNoOPSession(raftCtx.ShardID),
		ReplicaID: raftCtx.ReplicaID,
	}
}

func (c *mycontentClient) Post(ctx context.Context, namespace string, refIDs []string, ID string, data content.Data) (content.Data, error) {
	var validate map[string]any
	err := json.Unmarshal(data.Data, &validate)
	if err != nil {
		// opinionated
		return content.Data{}, fmt.Errorf("expected json mycontent data payload: %v", string(data.Data))
	}

	err = json.Unmarshal(data.Meta, &validate)
	if err != nil {
		// opinionated
		return content.Data{}, fmt.Errorf("expected json mycontent meta payload: %v", string(data.Meta))
	}

	wrap := map[string]any{
		"command": "gratis.desain.mycontent.post",
		"value": DataWrapper{
			Table:     c.tableName,
			Namespace: namespace,
			RefIDs:    refIDs,
			ID:        ID,
			Data:      data.Data, // raw json
			Meta:      data.Meta,
		},
		"replica_id": c.ReplicaID,
	}

	resp, err := c.publishToRaft(ctx, wrap)
	if err != nil {
		return content.Data{}, err
	}

	var parsedRaft DataWrapper
	err = json.Unmarshal(resp, &parsedRaft)
	if err != nil {
		return content.Data{}, fmt.Errorf("failed to parse raft response: %w '%v'", err, string(resp))
	}

	return content.Data{
		Namespace: parsedRaft.Namespace,
		RefIDs:    parsedRaft.RefIDs,
		ID:        parsedRaft.ID,
		Data:      parsedRaft.Data,
		Meta:      parsedRaft.Meta,
		EventID:   parsedRaft.EventID,
	}, nil
}

// Get daya by owner ID
func (c *mycontentClient) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]content.Data, error) {
	resp, err := c.queryLocal(ctx, QueryMyContent{
		Table:     c.tableName,
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
	})
	if err != nil {
		return nil, err
	}

	chanResp, ok := resp.(QueryMyContentResponse)
	if !ok {
		return nil, fmt.Errorf("server error: not query my content response: %T %v", resp, resp)
	}

	// for get, we store them all into memory
	result := make([]content.Data, 0)
	for data := range chanResp {
		result = append(result, *data)
	}

	return result, nil
}

// Delete specific ID data. If no data, MUST return error
func (c *mycontentClient) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (content.Data, error) {
	placeholder := json.RawMessage("{}")

	wrap := map[string]any{
		"command": "gratis.desain.mycontent.delete",
		"value": DataWrapper{
			Table:     c.tableName,
			Namespace: namespace,
			RefIDs:    refIDs,
			ID:        ID,
			Data:      placeholder,
			Meta:      placeholder,
		},
	}

	resp, err := c.publishToRaft(ctx, wrap)
	// TODO: find better way to parse data back and forth & error handling between client and raft app
	if err != nil {
		return content.Data{}, fmt.Errorf("failed to publish delete message to raft: %w", err)
	}

	var parsedRaft content.Data
	err = json.Unmarshal(resp, &parsedRaft)
	if err != nil {
		return content.Data{}, fmt.Errorf("failed to parse message from raft: %w (%v)", err, string(resp))
	}

	return parsedRaft, nil
}

// Stream Get data
func (c *mycontentClient) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan content.Data, error) {
	return nil, fmt.Errorf("not implemented yet")
}

func (c *mycontentClient) publishToRaft(ctx context.Context, msg any) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal msg to raft: %w (%v)", err, string(data))
	}

	var attempts int
	var res statemachine.Result
	for range 3 {
		attempts++
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res, err = c.DHost.SyncPropose(ctx, c.Sess, data)
		if err == nil {
			cancel()
			break
		}
		cancel()
		time.Sleep(500 * time.Millisecond * time.Duration(attempts*2))
	}

	if attempts >= 3 {
		return nil, fmt.Errorf("maximum number of attempt (3) reached: %w (%w)", err, ErrNotReady)
	}

	if res.Value > 0 {
		return nil, fmt.Errorf("got error from raft: '%v'", string(res.Data))
	}

	return res.Data, nil
}

func (c *mycontentClient) queryLocal(ctx context.Context, msg any) (any, error) {
	var res any
	var errg error
	for range 3 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		tmpRes, err := c.DHost.SyncRead(ctx, c.Sess.ShardID, msg)
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
