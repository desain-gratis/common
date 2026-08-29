package badgerraft

import (
	"context"
	"log"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	runneretcd "github.com/desain-gratis/common/lib/raft/runner-etcd"
)

// badgerRaftRepo extend the base that allows it to be interfaced with raft
type badgerRaftRepo struct {
	content.Repository
	TableName   string
	raftContext *runneretcd.RaftContext // specific
}

type Command struct {
	Name    string `json:"name"`
	Version string `json:"version"`

	TableName string       `json:"table_name"`
	Namespace string       `json:"namespace"`
	RefIDs    []string     `json:"ref_ids"`
	ID        string       `json:"id"`
	Data      content.Data `json:"data"`
}

func (c *badgerRaftRepo) Post(ctx context.Context, namespace string, refIDs []string, ID string, data content.Data) (content.Data, error) {
	cmd := Command{
		Name:      "post",
		Version:   "v1",
		TableName: c.TableName,
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
		Data:      data,
	}

	result, err := c.raftContext.Propose(ctx, cmd)
	if err != nil {
		return content.Data{}, err
	}

	data, _ = result.(content.Data) // because no additional info, ,we can return the request data only

	return data, nil
}

func (c *badgerRaftRepo) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (content.Data, error) {
	cmd := Command{
		Name:      "delete",
		Version:   "v1",
		TableName: c.TableName,
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
	}

	result, err := c.raftContext.Propose(ctx, cmd)
	if err != nil {
		return content.Data{}, err
	}

	data, _ := result.(content.Data) // because no additional info, ,we can return the request data only

	log.Printf("DATANYA %v\n", string(data.Data))

	return data, nil
}
