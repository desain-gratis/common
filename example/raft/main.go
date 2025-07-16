package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"time"

	raftenabledapi "github.com/desain-gratis/common/delivery/raft-enabled-api"
	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/config"
	"github.com/lni/dragonboat/v4/logger"
	"github.com/lni/dragonboat/v4/raftio"
)

const (
	// shard ID for config
	defaultShardID uint64 = 128
	datadir        string = ".rjs" // raft job scheduler
)

var leaderUpdate = make(chan leaderInfo, 12312)

var _ raftio.IRaftEventListener = &raftListener{}

type raftListener struct{}

type leaderInfo struct {
	replicaID uint64
	leaderID  uint64
	shardID   uint64
	term      uint64
}

func (r *raftListener) LeaderUpdated(info raftio.LeaderInfo) {
	if info.LeaderID == 0 {
		log.Printf(" no la policia :( %+v\n", info)
		return
	}
	log.Printf("leader updated!: %+v\n", info)
	leaderUpdate <- leaderInfo{
		replicaID: info.ReplicaID,
		leaderID:  info.LeaderID,
		shardID:   info.ShardID,
		term:      info.Term,
	}
}

func init() {
	logger.GetLogger("dragonboat").SetLevel(logger.DEBUG)
}

func main() {
	finished := make(chan struct{})
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)

	defer func() {
		log.Println("bye~ 👋🏼")
	}()

	var replicaID uint64
	var httpAddr string
	var raftAddr string
	var bootstrap string
	var joinFlag bool
	flag.Uint64Var(&replicaID, "replica-id", 1, "replica id for this host")
	flag.StringVar(&httpAddr, "http-addr", "", "http bind adddress <host>:<port>")
	flag.StringVar(&raftAddr, "raft-addr", "", "bind adddress <host>:<port>")
	flag.StringVar(&bootstrap, "bootstrap", "", "comma separated of replica's [<id>:<address>:<port>] pair for cluster config")
	flag.BoolVar(&joinFlag, "join", false, "comma separated of replica's [<id>:<address>:<port>] pair for cluster config")

	flag.Parse()

	nhc := config.NodeHostConfig{
		WALDir:            path.Join(datadir, strconv.FormatUint(replicaID, 10), "raft", "wal"),
		NodeHostDir:       path.Join(datadir, strconv.FormatUint(replicaID, 10), "raft", "etc"),
		RTTMillisecond:    50,
		RaftAddress:       raftAddr,
		RaftEventListener: &raftListener{},
	}

	bootstrapServerByReplicaID := make(map[uint64]dragonboat.Target)
	// process
	tokens := strings.Split(bootstrap, ",")
	for _, token := range tokens {
		if token == "" {
			continue
		}
		st := strings.Split(token, ":")
		replicaIDstr := st[0]
		otherReplicaID, err := strconv.ParseUint(replicaIDstr, 10, 64)
		if err != nil {
			log.Panicf("invalid replica id: %v. not an uint64\n", replicaIDstr)
		}

		othersRaftAddr := strings.Join(st[1:], ":")

		if replicaID == otherReplicaID || raftAddr == othersRaftAddr {
			log.Panicf("conflicted replica ID: %v OR bind addr: %v\n", replicaID, raftAddr)
		}

		bootstrapServerByReplicaID[otherReplicaID] = dragonboat.Target(
			othersRaftAddr,
		)
	}

	join := len(bootstrapServerByReplicaID) == 0 && joinFlag

	if !join {
		// add ourself as part of the cluster
		bootstrapServerByReplicaID[replicaID] = raftAddr
	}

	nh, err := dragonboat.NewNodeHost(nhc)
	if err != nil {
		log.Panicln("err dragon rising", err)
		return
	}
	defer nh.Close()

	repc := config.Config{
		ReplicaID:          replicaID,
		ShardID:            defaultShardID,
		ElectionRTT:        10,
		HeartbeatRTT:       1,
		CheckQuorum:        true,
		SnapshotEntries:    100, // if snapshot is not implemented can crash
		CompactionOverhead: 5,
		WaitReady:          true,
	}

	info := map[string]any{
		"replica_id":     replicaID,
		"shard_id":       defaultShardID,
		"join":           join,
		"bootstrap_info": bootstrapServerByReplicaID,
		"http-addr":      httpAddr,
		"raft-addr":      raftAddr,
	}

	if err := nh.StartOnDiskReplica(
		bootstrapServerByReplicaID,
		join,
		raftenabledapi.NewDiskKV,
		repc,
	); err != nil { // config needs to be always be same as first init ..?
		fmt.Fprintf(os.Stderr, "failed to add cluster, %v\n", err)
		os.Exit(1)
	}

	router := httprouter.New()

	haveLeader := new(bool)
	enableAuthAPI(router, nh, info, haveLeader)

	// start http endopoint
	server := http.Server{
		Addr:         httpAddr,
		Handler:      router,
		ReadTimeout:  time.Duration(10) * time.Second,
		WriteTimeout: time.Duration(10) * time.Second,
	}

	go func() {
		for {
			<-raftenabledapi.Updatechan
		}
	}()

	go func() {
		defer close(finished)
		<-sigint

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// log.Info().Msgf("Shutting down HTTP server..")
		if err := server.Shutdown(ctx); err != nil {
			// eg. timeout
			// log.Err(err).Msgf("HTTP server Shutdown")
		}
		// log.Info().Msgf("Stopped serving new connections.")
	}()

	// leadership update loop
	go func() {
		for {
			// wait get leader?
			log.Println("Listening for leadership update..") // only for one shard
			info := <-leaderUpdate
			if info.leaderID == 0 {
				log.Println("No leader. LAPOLOCIAA")
				*haveLeader = false
				continue
			}

			retryCount := 0
			for {
				leaderID, term, valid, err := nh.GetLeaderID(defaultShardID)
				if valid {
					log.Println("LEADER-SAMA", leaderID, term, valid, err)
					*haveLeader = true
					break
				}
				retryCount++
				time.Sleep(time.Duration(retryCount) * 50 * time.Millisecond)
			}
		}
	}()

	// log.Info().Msgf("Serving at %v..\n", address)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		// Error starting or closing listener:
		// log.Fatal().Msgf("HTTP server ListendAndServe: %v", err)
	}

	<-finished
}
