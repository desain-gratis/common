package runneretcd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	topicImpl "github.com/desain-gratis/common/lib/notifier/impl"
	dgraft "github.com/desain-gratis/common/lib/raft"
	"github.com/spf13/viper"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.etcd.io/etcd/client/pkg/v3/types"
	"go.etcd.io/etcd/server/v3/etcdserver/api/rafthttp"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	stats "go.etcd.io/etcd/server/v3/etcdserver/api/v2stats"
	"go.etcd.io/etcd/server/v3/storage/wal"
	"go.etcd.io/etcd/server/v3/storage/wal/walpb"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"go.uber.org/zap"
)

func RunWithConfig(cfgPath string, app dgraft.ApplicationV2) (context.Context, chan string, error) {
	cfg, err := readEtcdRaftConfig(cfgPath)
	if err != nil {
		return nil, nil, err
	}

	cluster := cfg.GetStringSlice("cluster")
	id := cfg.GetInt("id")
	join := cfg.GetBool("join")

	// proposeC := make(chan string)
	// defer close(proposeC)
	// confChangeC := make(chan *raftpb.ConfChange)
	// defer close(confChangeC)

	rw, err := startRaft(
		fmt.Sprintf("raftexample-%d-snap", id),
		fmt.Sprintf("raftexample-%d", id), // todo: configurable
		id,
		cluster,
		join,
		app,
	)
	if err != nil {
		return nil, nil, err
	}

	proposeOut := make(chan string)

	go func() {
		for msg := range proposeOut {
			rw.proposeC <- msg
		}
	}()

	return dgraft.WithRaftContext(context.Background(), rw), proposeOut, nil
}

// snpadir --> DATA snapdir
// waldir --> raft wal dir & raft snapshot dir
func startRaft(snapdir, waldir string, id int, peers []string, join bool, app dgraft.ApplicationV2) (*RaftContext, error) {
	ctx := context.Background()

	if !fileutil.Exist(snapdir) {
		if err := os.Mkdir(snapdir, 0o750); err != nil {
			log.Fatalf("raftexample: cannot create dir for snapshot %v (%v)", snapdir, err)
		}
	}

	logger := zap.NewExample()

	// Snapshotter used to store data (but we will only use for metadata to keep the size small[?])
	snapshotter := snap.New(logger, snapdir)

	// Wal snapshot + App snapshot -- loadSnapshot()
	// actually its the app snapshot.
	snapshot := &raftpb.Snapshot{}
	walExist := wal.Exist(waldir)
	if walExist {
		walSnaps, err := wal.ValidSnapshotEntries(logger, waldir)
		if err != nil {
			log.Fatalf("raftexample: error listing snapshots (%v)", err)
		}
		// load from DATA (KV) SNAPSHOT folder / snapdir dalam bentuk raftpb.Snapshot (kan dia bisa ada banyak)
		// dia di match dengan data walSnaps index & term nya match (many to many)
		_snapshot, err := snapshotter.LoadNewestAvailable(walSnaps) // THIS KV SNAPSHOT DATA is obtaine inside latest walsnap.
		if err != nil && !errors.Is(err, snap.ErrNoSnapshot) {
			log.Fatalf("raftexample: error loading snapshot (%v)", err)
		}

		snapshot = _snapshot // apparently KV snapshot loaded here aswell; so, ngga "hanya" WAL snapshot, tapi mengandung kv
		// dan lebih tepatnya lagi, term dan index & metadata segalanya ngikutin si KV.
	}

	// openWAL
	if !walExist {
		if err := os.Mkdir(waldir, 0o750); err != nil {
			log.Fatalf("raftexample: cannot create dir for wal (%v)", err)
		}

		w, err := wal.Create(logger, waldir, nil)
		if err != nil {
			log.Fatalf("raftexample: create wal error (%v)", err)
		}
		w.Close()
	}

	// snapshot related  related is "ready" here.
	// but we need the wal entries as well, before we can put it in storage

	// invariant up to here: WAL folder will always be there.
	// actually openning the WAL, previously only "peeking" and looking up for metadata

	walsnapForReading := walpb.Snapshot{} // for actually reading the wal entries (above code is only for the KV snapshot related logic)
	if snapshot.GetMetadata() != nil {
		walsnapForReading.Index, walsnapForReading.Term = snapshot.Metadata.Index, snapshot.Metadata.Term
	}

	log.Printf("loading WAL at term %d and index %d", walsnapForReading.GetTerm(), walsnapForReading.GetIndex())
	w, err := wal.Open(logger, waldir, &walsnapForReading)
	if err != nil {
		log.Fatalf("raftexample: error loading wal (%v)", err)
	}

	// actually read all, especially the "ENTS"
	_, st, ents, err := w.ReadAll()
	if err != nil {
		log.Fatalf("raftexample: failed to read WAL (%v)", err)
	}

	// Prepare the memory storage structure of the raft
	raftStorage := raft.NewMemoryStorage()
	if snapshot != nil {
		// notice that, it's only the "metadata"
		raftStorage.ApplySnapshot(snapshot) // walsnapshot nya di apply; artinya dimasukan kedalam core raft storage (in-memory log/journal nya)
	}

	raftStorage.SetHardState(st) // prepare to be called by the raft framework ( whatis hard state and why it's only in the wal)
	raftStorage.Append(ents)     // apply the entries (not only the "metadata"), I assume this entries are the entries after the walsnapshot

	// "replayWAL" selesai :)
	// artinya, storage sudah punya "walsnapshot" (aka metadata) utk
	// dan udah punya entries
	// siap diapakai oleh framework raft.

	// raft storage ngga punya informasi mengenai KV snapshot nya saat "start" ini.

	// kasih sinyal snapshotter ready
	// snapshotter ready, artinya KV udah bisa populate (?)
	// the snapshotter will be used in kv
	// rc.snapshotterReady <- rc.snapshotter

	rpeers := make([]raft.Peer, len(peers))
	for i := range rpeers {
		rpeers[i] = raft.Peer{ID: uint64(i + 1)}
	}
	c := &raft.Config{
		ID:                        uint64(id),
		ElectionTick:              10,
		HeartbeatTick:             1,
		Storage:                   raftStorage,
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
	}

	lastAppliedIndex, err := app.InitV2(ctx)
	if err != nil {
		return nil, err
	}

	c.Applied = lastAppliedIndex

	// start the raft node

	var raftNode raft.Node
	if walExist || join {
		raftNode = raft.RestartNode(c)
	} else {
		raftNode = raft.StartNode(c, rpeers)
	}

	// TODO: refactor this part
	// prepare the transport
	transport := &rafthttp.Transport{
		Logger:      logger,
		ID:          types.ID(id),
		ClusterID:   0x1000,
		Raft:        &transportListener{node: raftNode},
		ServerStats: stats.NewServerStats("", ""),
		LeaderStats: stats.NewLeaderStats(logger, strconv.Itoa(id)),
		ErrorC:      make(chan error),
	}
	transport.Start()

	for i := range peers { // right now start first and then add..
		if i+1 != id {
			transport.AddPeer(types.ID(i+1), []string{peers[i]})
		}
	}

	commitC := make(chan *commit)
	errorC := make(chan error)
	snapshotterReady := make(chan *snap.Snapshotter, 1)

	rc := &RaftContext{
		id:    uint64(id),
		peers: peers,
		// join:  join,

		raftStorage: raftStorage, // duplicate, but necessary since it's implementation is used inside (not only on the raft)
		wal:         w,           // used in internal process
		snapshotter: snapshotter, // needed as well hehe

		// input
		proposeC:    make(chan string),
		confChangeC: make(<-chan *raftpb.ConfChange),

		stopc:     make(chan struct{}),
		httpstopc: make(chan struct{}),
		httpdonec: make(chan struct{}),

		node:             raftNode,
		transport:        transport,
		commitC:          commitC,
		errorC:           errorC,
		snapshotterReady: snapshotterReady,

		getSnapshotData: func() ([]byte, error) {
			return []byte("this is a snapshot"), nil
		},

		ApplyTopic: topicImpl.NewStandardTopic(),
	}

	go func() {
		for commit := range commitC {
			for _, s := range commit.data {
				// log.Printf("index=%v term=%v data=%v\n", *s.Index, *s.Term, string(s.Data))
				var entry dgraft.EntryV2
				err := json.Unmarshal(s.Data, &entry)
				if err != nil {
					// bad data
					continue
				}

				// overwrite
				entry.Index = *s.Index
				entry.Term = *s.Term

				result, err := app.OnUpdateV2(ctx, entry)
				resultWrapper := &dgraft.ResultV2{
					Value:          0,
					Data:           result,
					Error:          err,
					SubscriptionID: entry.SubscriptionID,
				}
				err = rc.ApplyTopic.Broadcast(ctx, resultWrapper)
				if err != nil {
					log.Printf("should fatal")
				}
			}
			close(commit.applyDoneC)
		}
	}()

	go rc.serveTransport()
	go rc.serveRaft()

	return rc, nil
}

func readEtcdRaftConfig(cfgFile string) (cfg *viper.Viper, err error) {
	f, err := os.Open(cfgFile)
	if err != nil {
		log.Fatal("failed to open file %v", cfgFile)
	}

	v := viper.New()

	v.SetConfigType("yaml")
	err = v.ReadConfig(f)
	if err != nil {
		log.Fatal("failed to read dragonboat config %v %v", cfgFile, err)
	}

	log.Printf("reading config: %v", cfgFile)

	return v, nil
}
