package runneretcd

import (
	"context"

	"go.etcd.io/etcd/server/v3/etcdserver/api/rafthttp"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
)

var _ rafthttp.Raft = &transportListener{}

type transportListener struct {
	node raft.Node
}

func (tl *transportListener) Process(ctx context.Context, m *raftpb.Message) error {
	return tl.node.Step(ctx, m)
}

func (tl *transportListener) IsIDRemoved(_ uint64) bool {
	return false
}

func (tl *transportListener) ReportUnreachable(id uint64) {
	tl.node.ReportUnreachable(id)
}

func (tl *transportListener) ReportSnapshot(id uint64, status raft.SnapshotStatus) {
	tl.node.ReportSnapshot(id, status)
}
