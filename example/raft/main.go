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
)

const (
	// shard ID for config
	defaultShardID uint64 = 128
	datadir        string = ".rjs" // raft job scheduler
)

func main() {
	var replicaID uint64
	var httpAddr string
	var raftAddr string
	var bootstrap string
	flag.Uint64Var(&replicaID, "replica-id", 1, "replica id for this host")
	flag.StringVar(&httpAddr, "http-addr", "", "http bind adddress <host>:<port>")
	flag.StringVar(&raftAddr, "raft-addr", "", "bind adddress <host>:<port>")
	flag.StringVar(&bootstrap, "bootstrap", "", "comma separated of replica's [<id>:<address>:<port>] pair for cluster config")

	flag.Parse()

	nhc := config.NodeHostConfig{
		WALDir:         path.Join(datadir, strconv.FormatUint(replicaID, 10), "raft", "wal"),
		NodeHostDir:    path.Join(datadir, strconv.FormatUint(replicaID, 10), "raft", "etc"),
		RTTMillisecond: 200,
		RaftAddress:    raftAddr,
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

	join := len(bootstrapServerByReplicaID) == 0

	if !join {
		// add ourself as part of the cluster
		bootstrapServerByReplicaID[replicaID] = raftAddr
	}

	nh, err := dragonboat.NewNodeHost(nhc)
	if err != nil {
		log.Panicln("err dragon rising", err)
		return
	}

	repc := config.Config{
		ReplicaID:          replicaID,
		ShardID:            defaultShardID,
		ElectionRTT:        10,
		HeartbeatRTT:       1,
		CheckQuorum:        true,
		SnapshotEntries:    100, // if snapshot is not implemented can crash
		CompactionOverhead: 5,
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

	raftSession := nh.GetNoOPSession(defaultShardID)

	// http handler

	router := httprouter.New()

	enableAuthAPI(router, nh, raftSession, info)

	// start http endopoint
	server := http.Server{
		Addr:         httpAddr,
		Handler:      router,
		ReadTimeout:  time.Duration(10) * time.Second,
		WriteTimeout: time.Duration(10) * time.Second,
	}

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
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

	// log.Info().Msgf("Serving at %v..\n", address)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		// Error starting or closing listener:
		// log.Fatal().Msgf("HTTP server ListendAndServe: %v", err)
	}

	<-finished
	log.Println("bye~ 👋🏼")
}
