package runneretcd

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/desain-gratis/common/lib/notifier"
	"github.com/desain-gratis/common/lib/notifier/impl"
	dgraft "github.com/desain-gratis/common/lib/raft"
	"go.etcd.io/etcd/client/pkg/v3/types"
	"go.etcd.io/etcd/server/v3/etcdserver/api/rafthttp"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/etcd/server/v3/storage/wal"
	"go.etcd.io/etcd/server/v3/storage/wal/walpb"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// TODO: IMPLEMENT RAFT STORAGE PROPERLY FOR BADGER, THIS IS JUST A QUICK HACK
// We can  start by "extending" the *raft.MemoryStorage
// Meanwhile, for POC we use this hack.
var defaultSnapshotCount uint64 = 10000

var snapshotCatchUpEntriesN uint64 = 10000

type RaftContext struct {
	// todo: maybe put in config
	id       uint64
	peers    []string
	bindAddr string

	config *raft.Config

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
	commitC     chan *commit              // entries committed to log (k,v)
	leaderC     chan *leaderChange
	errorC      chan<- error  // errors from raft session
	stopc       chan struct{} // signals proposal channel closed

	snapshotter      *snap.Snapshotter
	snapshotterReady chan *snap.Snapshotter // signals when snapshotter is ready

	getSnapshotData func() ([]byte, error)

	ApplyTopic notifier.Topic
	app        dgraft.ApplicationV2

	Logger *zap.Logger
}

type commit struct {
	data       []*raftpb.Entry
	applyDoneC chan<- struct{}
}

type leaderChange struct {
	Ctx      context.Context
	Term     uint64
	LeaderID uint64
}

// Maybe we rename to ProposeSync
// TODO as well, use better encoding eg. proto type.
func (rc *RaftContext) Propose(ctx context.Context, value any) (any, error) {
	subID := strconv.FormatUint(rand.Uint64(), 10)

	sub, _ := rc.ApplyTopic.Subscribe(ctx, impl.NewStandardSubscriber(func(a any) bool {
		// filter out other message
		msg, ok := a.(*dgraft.ResultV2)
		return !ok || msg.SubscriptionID != subID
	}))

	reqPayload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	// entry wrapper

	data := dgraft.EntryV2{
		SourceNodeID:   rc.id,
		Data:           reqPayload,
		SubscriptionID: subID,
	}

	// todo: use proto
	wrappedPayload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	sub.Start()

	var result *dgraft.ResultV2

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		msg := <-sub.Listen()

		r, ok := msg.(*dgraft.ResultV2)
		if !ok {
			return
		}
		result = r
	}()

	err = rc.node.Propose(ctx, wrappedPayload)
	if err != nil {
		return nil, err
	}

	wg.Wait()
	if result == nil && ctx.Err() != nil {
		return result, ctx.Err()
	}

	return result.Data, result.Error
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

	var lt leaderTracker

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
				// meaning: Save it inside the WAL that we have a 'snapshot entry' not necessary a full Database backup.
				rc.saveSnapshotToDisk(rd.Snapshot)
			}
			var hs *raftpb.HardState
			if !raft.IsEmptyHardState(rd.HardState) {
				hs = rd.HardState
			}
			rc.wal.Save(hs, rd.Entries)

			// 2. LOAD TO STORAGE (TO MEMORY; since it's using the inmemory implementation)
			if !raft.IsEmptySnap(rd.Snapshot) {
				// Load the snapshot to the application
				rc.raftStorage.ApplySnapshot(rd.Snapshot)

				// kv must be un sync w. snapshot in me storage
				rc.loadSnapshot(rd.Snapshot)
			}
			// 2a. Load entries (to memory as well)
			rc.raftStorage.Append(rd.Entries) // entries

			// 3. SEND
			rc.transport.Send(rc.updateMsgSnap(rd.Messages))

			// Leadership change event
			rc.detectLeadershipChange(&lt, &rd)

			// 4. APPLY
			applyDoneC, ok := rc.publishEntries(rc.entriesToApply(rd.CommittedEntries))
			if !ok {
				rc.stop()
				return
			}

			// todo: maybe better if we can  make it simpler later / make it per "applied" message;
			// no need to use channel.

			// see inside this method
			rc.maybeTriggerSnapshot(applyDoneC)

			// TODO: important, on after apply () should be made here.

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

func (rc *RaftContext) detectLeadershipChange(lt *leaderTracker, rd *raft.Ready) {
	// 1) New term voids leadership.
	if hs := rd.HardState; hs != nil && hs.GetTerm() != lt.term {
		lt.term = hs.GetTerm()
		lt.loseLeadership()
	}

	// 2) SoftState change: losing StateLeader or a new Lead voids leadership.
	if ss := rd.SoftState; ss != nil {
		if lt.isLeader && !(ss.Lead == rc.id && ss.RaftState == raft.StateLeader) {
			lt.loseLeadership()
		}
		lt.isLeader = ss.Lead == rc.id && ss.RaftState == raft.StateLeader
	}

	// 3) Fire exactly once per term, when an entry of MY term commits.
	//    The first such entry is what confirms leadership.
	if lt.isLeader && lt.firedTerm != lt.term {
		for _, e := range rd.CommittedEntries {
			if e.GetTerm() == lt.term {
				lt.firedTerm = lt.term
				lt.lctx, lt.lcancel = context.WithCancel(context.Background())
				rc.leaderC <- &leaderChange{Ctx: lt.lctx, Term: lt.term, LeaderID: rc.id}
				break
			}
		}
	}
}

func (lt *leaderTracker) loseLeadership() {
	lt.isLeader = false
	if lt.lcancel != nil {
		lt.lcancel()
		lt.lcancel = nil
	}
}

// TODO: use our pattern
// allow communicate to each server
func (rc *RaftContext) serveTransport() {
	if rc.bindAddr == "" {
		url, err := url.Parse(rc.peers[rc.id-1]) // todo: use rc.bindAddr
		if err != nil {
			log.Fatalf("raftexample: Failed parsing URL (%v)", err)
		}
		rc.bindAddr = url.Host
	}

	// https://stackoverflow.com/questions/63676241/how-to-set-setkeepaliveperiod-on-a-tls-conn
	// 3 minutes as per the etcd's contrib/raftexample
	lc := net.ListenConfig{KeepAlive: 3 * time.Minute}
	ln, err := lc.Listen(context.Background(), "tcp", rc.bindAddr) // TODO: use proper context
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

	close(rc.httpdonec)
}

// Apply is just like "saveSnapshotToDisk", but without any snapshotting logic
// It's only writes to WAL, not to the "snapshot"
func (rc *RaftContext) apply(index, term uint64, confState *raftpb.ConfState, _debugDeleteMe *raftpb.Snapshot) error {
	walSnap := walpb.Snapshot{
		Index:     &index,
		Term:      &term,
		ConfState: confState,
	}

	if _debugDeleteMe != nil && _debugDeleteMe.Metadata != nil {
		rc.Logger.Info("Refactor debug: ",
			zap.Any("confState-equal", _debugDeleteMe.Metadata.ConfState.Equivalent(confState) == nil),
			zap.Any("index-equal", _debugDeleteMe.Metadata.GetIndex() == index),
			zap.Any("term-equal", _debugDeleteMe.Metadata.GetTerm() == term),
		)
	}

	// notice the difference between this and saveSnapshotToDisk
	// this one doesn't store snapshot; since we append to wal on every message.

	// Since at init, we open the wal using a *walSnap pointer, we mark it use it as our "Apply" mechanism.
	// Notice that we don't do actual "snapshotting" logic (no accompanied snapshotter.SaveSnap )
	if err := rc.wal.SaveSnapshot(&walSnap); err != nil { // basically we "apply" here; using snap.Metadata.Index here.
		return err // wal both save "entries" & the commited index
	}

	return rc.wal.ReleaseLockTo(index)
}

// Called from Ready
func (rc *RaftContext) saveSnapshotToDisk(snap *raftpb.Snapshot) error {
	walSnap := walpb.Snapshot{
		Index:     snap.Metadata.Index,
		Term:      snap.Metadata.Term,
		ConfState: snap.Metadata.ConfState,
	}

	// save the snapshot file before writing the snapshot to the wal.
	// This makes it possible for the snapshot file to become orphaned, but prevents
	// a WAL snapshot entry from having no corresponding snapshot file.
	// TESTING COMMENT SNAPSHOT RELATED

	// if err := rc.snapshotter.SaveSnap(snap); err != nil {
	// 	return err
	// }

	if err := rc.wal.SaveSnapshot(&walSnap); err != nil {
		return err // wal both save "entries" & the commited index
	}

	return rc.wal.ReleaseLockTo(snap.Metadata.GetIndex()) // TESTING COMMENT SNAPSHOT RELATED
}

func (rc *RaftContext) loadSnapshot(snapshotToSave *raftpb.Snapshot) {
	if raft.IsEmptySnap(snapshotToSave) {
		return
	}

	if snapshotToSave.Metadata.GetIndex() <= rc.appliedIndex {
		log.Fatalf("snapshot index [%d] should > progress.appliedIndex [%d]", snapshotToSave.Metadata.GetIndex(), rc.appliedIndex)
	}

	// IMPORTANT: this one need to be implemented since can load the snapshot from DISK.
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
					rc.Logger.Info("I've been removed from the cluster! Shutting down.")
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

func (rc *RaftContext) maybeTriggerSnapshot(applyDoneC <-chan struct{}) {
	if rc.appliedIndex-rc.snapshotIndex <= defaultSnapshotCount {
		return
	}

	_, ok := rc.app.(dgraft.ApplicationV2WithSnapshot)
	if !ok {
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

	// take a "photo" snapshot of the storage as source of truth; used by the raft framework
	// It seems it's used by Raft in Snapshot() method. In our case, it also response back to us for saving externally.
	// important to remember: all node do create snapshot, not only the leader. (etcd also do that)
	// additionally, etcd server can do force snapshot locally for storage versioning backward compability
	// but we dont do it for our usecase (at least for near future)
	rc.Logger.Info("SNAPSHOT TRIGGERED", zap.Uint64("applied-index", rc.appliedIndex))

	// // TODO:
	// var buf bytes.Buffer
	// size, err := app.WriteSnapshot(&buf)
	// if err != nil {
	// 	log.Panic(err)
	// }

	// IMPORTANT THAT THIS WAS THE METADATA, SINCE IT WILL ALL BE WRITTEN INTO MEMORY.
	// We can PIPE the whole data somewhere else, and
	// SNAPSHOT APP METADATA
	smallDataSample := []byte(`{"message": "this is only a metadata, loaded all into memory. we can configure this later~"}`)
	verifyMyLogicTmp, err := rc.raftStorage.CreateSnapshot(rc.appliedIndex, rc.confState, smallDataSample) // test with nil. we don't have "snapshot"
	if err != nil {
		panic(err)
	}

	// IMPORTANT to remember:
	// "light" snap storage (not the full DB). not sure  if we needed; I'll comment out for now
	// if err := rc.snapshotter.SaveSnap(verifyMyLogicTmp); err != nil {
	// 	panic(err)
	// }

	// Using our refactored "apply" start ---

	// this is the actual "wal" commit index
	term, err := rc.raftStorage.Term(rc.appliedIndex)
	if err != nil {
		panic(err)
	}

	// re-use the apply. code is same
	if err := rc.apply(rc.appliedIndex, term, rc.confState, verifyMyLogicTmp); err != nil {
		panic(err)
	}

	// refactored "apply" end ---

	// I thnk this one is logic for "slow" follower for them to catch up entries

	compactIndex := uint64(1)
	if rc.appliedIndex > snapshotCatchUpEntriesN {
		compactIndex = rc.appliedIndex - snapshotCatchUpEntriesN
	}

	// this is for in memory , it's ok
	if err := rc.raftStorage.Compact(compactIndex); err != nil {
		if !errors.Is(err, raft.ErrCompacted) {
			panic(err)
		}
	} else {
		rc.Logger.Info("Compacted log", zap.Uint64("compact-index", compactIndex))
	}

	rc.snapshotIndex = rc.appliedIndex
}

// stop closes http, closes all channels, and stops raft.
func (rc *RaftContext) stop() {
	rc.Logger.Info("KENEAPADISTOP?")
	rc.stopHTTP()
	close(rc.commitC)
	close(rc.errorC)
	close(rc.leaderC)
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
	close(rc.leaderC)
}
