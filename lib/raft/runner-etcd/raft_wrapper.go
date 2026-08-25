package runneretcd

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/desain-gratis/common/lib/notifier"
	dgraft "github.com/desain-gratis/common/lib/raft"
	"go.etcd.io/etcd/client/pkg/v3/types"
	"go.etcd.io/etcd/server/v3/etcdserver/api/rafthttp"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/etcd/server/v3/storage/wal"
	"go.etcd.io/etcd/server/v3/storage/wal/walpb"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"google.golang.org/protobuf/proto"
)

var snapshotCatchUpEntriesN uint64 = 10000

type RaftContext struct {
	// todo: maybe put in config
	id       uint64
	peers    []string
	bindAddr string

	// todo: temporary (can use better pattern)
	httpstopc chan struct{}
	httpdonec chan struct{}

	node      raft.Node
	transport *rafthttp.Transport

	raftStorage   *raft.MemoryStorage
	wal           *wal.WAL
	confState     *raftpb.ConfState
	snapshotIndex uint64
	appliedIndex  uint64

	proposeC    chan string               // proposed messages (k,v)
	confChangeC <-chan *raftpb.ConfChange // proposed cluster config changes
	commitC     chan<- *commit            // entries committed to log (k,v)
	errorC      chan<- error              // errors from raft session
	stopc       chan struct{}             // signals proposal channel closed

	snapshotter      *snap.Snapshotter
	snapshotterReady chan *snap.Snapshotter // signals when snapshotter is ready

	getSnapshotData func() ([]byte, error)

	ApplyTopic notifier.Topic
}

type commit struct {
	data       []*raftpb.Entry
	applyDoneC chan<- struct{}
}

func (rc *RaftContext) Propose(ctx context.Context, value []byte) (any, error) {
	data := dgraft.EntryV2{
		SourceNodeID: rc.id,
		Data:         value,
		// id term later
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	err = rc.node.Propose(ctx, payload)
	if err != nil {
		return nil, err
	}

	// rc.ApplyTopic

	return nil, nil
}

func (rc *RaftContext) serveRaft() {
	snap, err := rc.raftStorage.Snapshot()
	if err != nil {
		panic(err)
	}
	rc.confState = snap.Metadata.ConfState
	rc.snapshotIndex = snap.Metadata.GetIndex()
	rc.appliedIndex = snap.Metadata.GetIndex()

	defer rc.wal.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// send proposals over raft
	go func() {
		confChangeCount := uint64(0)

		for rc.proposeC != nil && rc.confChangeC != nil {
			select {
			case prop, ok := <-rc.proposeC:
				if !ok {
					rc.proposeC = nil
				} else {
					// blocks until accepted by raft state machine
					_ = rc.node.Propose(context.Background(), []byte(prop))
				}

			case cc, ok := <-rc.confChangeC:
				if !ok {
					rc.confChangeC = nil
				} else {
					confChangeCount++
					cc.Id = &confChangeCount
					rc.node.ProposeConfChange(context.TODO(), cc)
				}
			}
		}
		// client closed channel; shutdown raft if not already
		close(rc.stopc)
	}()

	// event loop on raft state machine updates
	for {
		select {
		case <-ticker.C:
			rc.node.Tick()

		// store raft entries to wal, then publish over commit channel
		case rd := <-rc.node.Ready():
			// 1. SAVE IN ADVANCE (TO DISK)
			if !raft.IsEmptySnap(rd.Snapshot) {
				// Save snapshot from raft (not from our initialization code) to disk immediately.
				rc.saveSnapshotToDisk(rd.Snapshot)
			}
			var hs *raftpb.HardState
			if !raft.IsEmptyHardState(rd.HardState) {
				hs = rd.HardState
			}
			rc.wal.Save(hs, rd.Entries)

			// 2. LOAD TO STORAGE
			if !raft.IsEmptySnap(rd.Snapshot) {
				// Load the snapshot to the application
				rc.raftStorage.ApplySnapshot(rd.Snapshot)

				// kv must be un sync w. snapshot in me storage
				rc.loadSnapshot(rd.Snapshot)
			}
			rc.raftStorage.Append(rd.Entries) // entnries

			// 3. SEND
			rc.transport.Send(rc.updateMsgSnap(rd.Messages))

			// 4. APPLY
			applyDoneC, ok := rc.publishEntries(rc.entriesToApply(rd.CommittedEntries))
			if !ok {
				rc.stop()
				return
			}
			rc.maybeTriggerSnapshot(applyDoneC)
			rc.node.Advance()

		case err := <-rc.transport.ErrorC:
			rc.writeError(err)
			return

		case <-rc.stopc:
			rc.stop()
			return
		}
	}

	// During processing of app message,
	// the app can always write-write-write-write asynchronously. INSERT to `log` table.`
	// but during scheduled time / during snapshot, we should query that `log` table, get the latest log/index written,
	// and use that as snapshot metadata.
	//
	// i think this is more efficient than using fsynced synchronous write for each message..
	// since anyway, the data already written inside the WAL.
	// we just need to replay the WAL correctly by using the actual `log` stored in our application side logic.

	// The same pattern can (and should) be applied to non clickhouse DB as well.

	// By the same logic, during initial run, we need to check whats the oldest log index inside our DB;
	// And if it's not from beginning, what to do (eg. might need to get from peers [?])
	// Where we can get one?

	// This might be redundant with the WALS...  bet its OK lets keep it separate at least for now
	// Using existing WAL implementation let's the library more flexible,
	// eg. if no need to use clickhouse, its ok. If we re-implement wal using clickhouse then its coupled to that...
	// so wal dir default impl is the way to go.
}

// TODO: use our pattern
// allow communicate to each server
func (rc *RaftContext) serveTransport() {
	url, err := url.Parse(rc.peers[rc.id-1]) // todo: use rc.bindAddr
	if err != nil {
		log.Fatalf("raftexample: Failed parsing URL (%v)", err)
	}

	// https://stackoverflow.com/questions/63676241/how-to-set-setkeepaliveperiod-on-a-tls-conn
	// 3 minutes as per the raftexample
	lc := net.ListenConfig{KeepAlive: 3 * time.Minute}
	ln, err := lc.Listen(context.Background(), "tcp", url.Host) // TODO: use proper context
	// log.Println("HELLO ORLDFREND: ", url.Host)
	// ln, err := newStoppableListener(url.Host, rc.httpstopc)
	if err != nil {
		log.Fatalf("raftexample: Failed to listen rafthttp (%v)", err)
	}
	srv := &http.Server{Handler: rc.transport.Handler()}

	err = srv.Serve(ln)
	if err != nil {
		log.Fatalf("raftexample: Failed to serve rafthttp (%v)", err)
	}

	select {
	case <-rc.httpstopc:
	default:
		log.Fatalf("raftexample: Failed to serve rafthttp (%v)", err)
	}

	log.Println("LOH KENAPA DICLOSE BAPACK!")
	close(rc.httpdonec)
}

func (rc *RaftContext) saveSnapshotToDisk(snap *raftpb.Snapshot) error {
	walSnap := walpb.Snapshot{
		Index:     snap.Metadata.Index,
		Term:      snap.Metadata.Term,
		ConfState: snap.Metadata.ConfState,
	}
	// save the snapshot file before writing the snapshot to the wal.
	// This makes it possible for the snapshot file to become orphaned, but prevents
	// a WAL snapshot entry from having no corresponding snapshot file.
	if err := rc.snapshotter.SaveSnap(snap); err != nil {
		return err
	}
	if err := rc.wal.SaveSnapshot(&walSnap); err != nil {
		return err
	}
	return rc.wal.ReleaseLockTo(snap.Metadata.GetIndex())
}

func (rc *RaftContext) loadSnapshot(snapshotToSave *raftpb.Snapshot) {
	if raft.IsEmptySnap(snapshotToSave) {
		return
	}

	log.Printf("publishing snapshot at index %d", rc.snapshotIndex)
	defer log.Printf("finished publishing snapshot at index %d", rc.snapshotIndex)

	if snapshotToSave.Metadata.GetIndex() <= rc.appliedIndex {
		log.Fatalf("snapshot index [%d] should > progress.appliedIndex [%d]", snapshotToSave.Metadata.GetIndex(), rc.appliedIndex)
	}
	rc.commitC <- nil // trigger kvstore to load snapshot

	rc.confState = snapshotToSave.Metadata.ConfState
	rc.snapshotIndex = snapshotToSave.Metadata.GetIndex()
	rc.appliedIndex = snapshotToSave.Metadata.GetIndex()
}

// When there is a `raftpb.EntryConfChange` after creating the snapshot,
// then the confState included in the snapshot is out of date. so We need
// to update the confState before sending a snapshot to a follower.
func (rc *RaftContext) updateMsgSnap(ms []*raftpb.Message) []*raftpb.Message {
	// no need to create new pointer no...
	var messages []*raftpb.Message
	for i := 0; i < len(ms); i++ {
		if ms[i].GetType() == raftpb.MsgSnap {
			ms[i].Snapshot.Metadata.ConfState = rc.confState
		}
		messages = append(messages, ms[i])
	}
	return messages
}

// publishEntries writes committed log entries to commit channel and returns
// whether all entries could be published.
func (rc *RaftContext) publishEntries(ents []*raftpb.Entry) (<-chan struct{}, bool) {
	if len(ents) == 0 {
		return nil, true
	}

	data := make([]*raftpb.Entry, 0, len(ents))
	for i := range ents {
		switch ents[i].GetType() {
		case raftpb.EntryNormal:
			if len(ents[i].Data) == 0 {
				// ignore empty messages
				break
			}
			data = append(data, ents[i])
		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := proto.Unmarshal(ents[i].Data, &cc); err != nil {
				log.Fatalf("failed to unmarshal conf change: %v", err)
			}
			rc.confState = rc.node.ApplyConfChange(&cc)
			switch cc.GetType() {
			case raftpb.ConfChangeAddNode:
				if len(cc.Context) > 0 {
					rc.transport.AddPeer(types.ID(cc.GetNodeId()), []string{string(cc.Context)})
				}
			case raftpb.ConfChangeRemoveNode:
				if cc.GetNodeId() == uint64(rc.id) {
					log.Println("I've been removed from the cluster! Shutting down.")
					return nil, false
				}
				rc.transport.RemovePeer(types.ID(cc.GetNodeId()))
			}
		}
	}

	var applyDoneC chan struct{}

	if len(data) > 0 {
		applyDoneC = make(chan struct{}, 1)
		select {
		case rc.commitC <- &commit{data, applyDoneC}:
		case <-rc.stopc:
			return nil, false
		}
	}

	// after commit, update appliedIndex
	rc.appliedIndex = ents[len(ents)-1].GetIndex()

	return applyDoneC, true
}

func (rc *RaftContext) entriesToApply(ents []*raftpb.Entry) (nents []*raftpb.Entry) {
	if len(ents) == 0 {
		return ents
	}
	firstIdx := ents[0].GetIndex()
	if firstIdx > rc.appliedIndex+1 {
		log.Fatalf("first index of committed entry[%d] should <= progress.appliedIndex[%d]+1", firstIdx, rc.appliedIndex)
	}
	if rc.appliedIndex-firstIdx+1 < uint64(len(ents)) {
		nents = ents[rc.appliedIndex-firstIdx+1:]
	}
	return nents
}

var defaultSnapshotCount uint64 = 20

func (rc *RaftContext) maybeTriggerSnapshot(applyDoneC <-chan struct{}) {
	if rc.appliedIndex-rc.snapshotIndex <= defaultSnapshotCount {
		return
	}

	// wait until all committed entries are applied (or server is closed)
	if applyDoneC != nil {
		select {
		case <-applyDoneC:
		case <-rc.stopc:
			return
		}
	}

	log.Printf("start snapshot [applied index: %d | last snapshot index: %d]", rc.appliedIndex, rc.snapshotIndex)
	data, err := rc.getSnapshotData()
	if err != nil {
		log.Panic(err)
	}

	// take a "photo" snapshot of the storage as source of truth; used by the raft framework
	snap, err := rc.raftStorage.CreateSnapshot(rc.appliedIndex, rc.confState, data)
	if err != nil {
		panic(err)
	}

	if err := rc.saveSnapshotToDisk(snap); err != nil {
		panic(err)
	}

	compactIndex := uint64(1)
	if rc.appliedIndex > snapshotCatchUpEntriesN {
		compactIndex = rc.appliedIndex - snapshotCatchUpEntriesN
	}
	if err := rc.raftStorage.Compact(compactIndex); err != nil {
		if !errors.Is(err, raft.ErrCompacted) {
			panic(err)
		}
	} else {
		log.Printf("compacted log at index %d", compactIndex)
	}

	rc.snapshotIndex = rc.appliedIndex
}

// stop closes http, closes all channels, and stops raft.
func (rc *RaftContext) stop() {
	log.Println("KENEAPADISOPT?")
	rc.stopHTTP()
	close(rc.commitC)
	close(rc.errorC)
	rc.node.Stop()
}

func (rc *RaftContext) stopHTTP() {
	rc.transport.Stop()
	close(rc.httpstopc)
	<-rc.httpdonec
}

func (rc *RaftContext) writeError(err error) {
	rc.stopHTTP()
	close(rc.commitC)
	rc.errorC <- err
	close(rc.errorC)
	rc.node.Stop()
}
