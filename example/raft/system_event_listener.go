package main

import (
	"log"

	"github.com/lni/dragonboat/v4/raftio"
)

var Ready = make(chan string)

var _ raftio.ISystemEventListener = &seListener{}

type seListener struct{}

func (s *seListener) NodeHostShuttingDown() {
	log.Printf("NodeHostShuttingDown()")
}

func (s *seListener) NodeUnloaded(info raftio.NodeInfo) {
	log.Printf("NodeUnloaded() replica=%v shard=%v!\n", info.ReplicaID, info.ShardID)
}
func (s *seListener) NodeDeleted(info raftio.NodeInfo) {
	log.Printf("NodeDeleted() replica=%v shard=%v!\n", info.ReplicaID, info.ShardID)
}
func (s *seListener) NodeReady(info raftio.NodeInfo) {
	log.Printf("NodeReady() replica=%v shard=%v!\n", info.ReplicaID, info.ShardID)
	// Ready <- "IM READE"
}

func (s *seListener) MembershipChanged(info raftio.NodeInfo) {
	log.Printf("MembershipChanged() replica=%v shard=%v\n", info.ReplicaID, info.ShardID)
}

func (s *seListener) ConnectionEstablished(info raftio.ConnectionInfo) {
	log.Printf("ConnectionEstablished() addr=%v snap=%v\n", info.Address, info.SnapshotConnection)

}
func (s *seListener) ConnectionFailed(info raftio.ConnectionInfo)    {}
func (s *seListener) SendSnapshotStarted(info raftio.SnapshotInfo)   {}
func (s *seListener) SendSnapshotCompleted(info raftio.SnapshotInfo) {}
func (s *seListener) SendSnapshotAborted(info raftio.SnapshotInfo)   {}
func (s *seListener) SnapshotReceived(info raftio.SnapshotInfo)      {}
func (s *seListener) SnapshotRecovered(info raftio.SnapshotInfo)     {}
func (s *seListener) SnapshotCreated(info raftio.SnapshotInfo)       {}
func (s *seListener) SnapshotCompacted(info raftio.SnapshotInfo)     {}
func (s *seListener) LogCompacted(info raftio.EntryInfo)             {}
func (s *seListener) LogDBCompacted(info raftio.EntryInfo)           {}
