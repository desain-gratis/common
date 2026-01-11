package clickhouseraft

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	raft_runner "github.com/desain-gratis/common/lib/raft/runner"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/client"
	"github.com/lni/dragonboat/v4/statemachine"
)

var _ content.Repository = &mycontentClient{}

type mycontentClient struct {
	tableName string
	DHost     *dragonboat.NodeHost
	Sess      *client.Session
}

func NewStorageClient(ctx context.Context, tableName string) *mycontentClient {
	raftCtx := raft_runner.GetRaftContext(ctx)
	return &mycontentClient{
		tableName: tableName,
		DHost:     raftCtx.DHost,
		Sess:      raftCtx.DHost.GetNoOPSession(raftCtx.ShardID),
	}
}

func (c *mycontentClient) Post(ctx context.Context, namespace string, refIDs []string, ID string, data content.Data) (content.Data, error) {
	var validate map[string]any
	err := json.Unmarshal(data.Data, &validate)
	if err != nil {
		// opinionated
		return content.Data{}, fmt.Errorf("expected json mycontent payload: %v")
	}

	err = json.Unmarshal(data.Meta, &validate)
	if err != nil {
		// opinionated
		return content.Data{}, fmt.Errorf("expected json mycontent payload: %v")
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
	}

	resp, err := c.publishToRaft(ctx, wrap)
	if err != nil {
		return content.Data{}, err
	}

	var parsedRaft DataWrapper
	err = json.Unmarshal(resp, &parsedRaft)
	if err != nil {
		return content.Data{}, err
	}

	return content.Data{
		Namespace: parsedRaft.Namespace,
		RefIDs:    parsedRaft.RefIDs,
		ID:        parsedRaft.ID,
		Data:      parsedRaft.Data,
		Meta:      parsedRaft.Meta,
	}, nil
}

// Get daya by owner ID
func (c *mycontentClient) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]content.Data, error) {
	return nil, fmt.Errorf("not implemented yet")
}

// Delete specific ID data. If no data, MUST return error
func (c *mycontentClient) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (content.Data, error) {
	resp, err := c.publishToRaft(ctx, map[string]any{
		"command": "gratis.desain.mycontent.delete",
		"data": DataWrapper{
			Table:     c.tableName,
			Namespace: namespace,
			RefIDs:    refIDs,
			ID:        ID,
		},
	})
	if err != nil {
		return content.Data{}, err
	}

	var parsedRaft content.Data
	err = json.Unmarshal(resp, &parsedRaft)
	if err != nil {
		return content.Data{}, err
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
		return nil, err
	}

	var res statemachine.Result
	for range 3 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res, err = c.DHost.SyncPropose(ctx, c.Sess, data)
		if err == nil {
			cancel()
			break
		}
		cancel()
		time.Sleep(500 * time.Millisecond)
	}

	return res.Data, nil
}

func (c *mycontentClient) queryLocal(ctx context.Context, msg any) (any, error) {
	var res any
	for range 3 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		tmpRes, err := c.DHost.SyncRead(ctx, c.Sess.ShardID, msg)
		if err == nil {
			res = tmpRes
			cancel()
			break
		}
		cancel()
		time.Sleep(500 * time.Millisecond)
	}

	return res, nil
}
