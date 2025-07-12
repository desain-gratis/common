package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

func enableAuthAPI(router *httprouter.Router, nh *dragonboat.NodeHost, info map[string]any, haveLeader *bool) {
	sess := nh.GetNoOPSession(defaultShardID)
	router.POST("/auth/gsi", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		id, term, valid, err := nh.GetLeaderID(defaultShardID)

		info["leader_id"] = id
		info["leader_term"] = term
		info["leader_valid"] = valid
		info["leader_err"] = err

		info["our_id"] = nh.ID()

		if nhregis, ok := nh.GetNodeHostRegistry(); ok {
			info["nhregis_nshards"] = nhregis.NumOfShards()
			if meta, ok := nhregis.GetMeta("test"); ok {
				info["meta"] = meta
			}
			if si, ok := nhregis.GetShardInfo(defaultShardID); ok {
				info["si_config_change_index"] = si.ConfigChangeIndex
				info["si_leader_id"] = si.LeaderID
				info["si_replicas"] = si.Replicas
				info["si_shard_id"] = si.ShardID
				info["si_term"] = si.Term
			}
		}

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			payload = []byte(`hehe`)
			log.Error().Msgf("failed to read body")
		}

		ses := nh.GetNoOPSession(defaultShardID)

		res, err := nh.SyncPropose(ctx, ses, payload)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			info["message"] = "err: " + err.Error()
			result, _ := json.Marshal(info)
			w.Write(result)
			return
		}

		w.WriteHeader(http.StatusOK)
		info["message"] = fmt.Sprintf("SUCCESS: %v (%v)", string(res.Data), res.Value)
		result, _ := json.Marshal(info)
		w.Write(result)
	})

	router.GET("/ingfo", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		id, term, valid, err := nh.GetLeaderID(defaultShardID)

		info["leader_id"] = id
		info["leader_term"] = term
		info["leader_valid"] = valid
		info["leader_err"] = err

		info["our_id"] = nh.ID()

		if nhregis, ok := nh.GetNodeHostRegistry(); ok {
			info["nhregis_nshards"] = nhregis.NumOfShards()
			if meta, ok := nhregis.GetMeta("test"); ok {
				info["meta"] = meta
			}
			if si, ok := nhregis.GetShardInfo(defaultShardID); ok {
				info["si_config_change_index"] = si.ConfigChangeIndex
				info["si_leader_id"] = si.LeaderID
				info["si_replicas"] = si.Replicas
				info["si_shard_id"] = si.ShardID
				info["si_term"] = si.Term
			}
		}

		res, err := nh.SyncRead(ctx, defaultShardID, []byte("echo"))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			info["message"] = "err: " + err.Error()
			result, _ := json.Marshal(info)
			w.Write(result)
			return
		}

		w.WriteHeader(http.StatusOK)
		info["message"] = res.(string)
		result, _ := json.Marshal(info)
		w.Write(result)
	})

	router.GET("/member", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// this will block until we have leader
		if !*haveLeader {
			w.WriteHeader(404)
			w.Write([]byte("no leader goodbye!"))
			return
		}

		retry := 0

		var result statemachine.Result
		var err error
		for {
			ctx, c := context.WithTimeout(r.Context(), 1*time.Second)
			result, err = nh.SyncPropose(ctx, sess, []byte("hello world!"))
			c()
			if err == nil {
				break
			}

			if err != dragonboat.ErrTimeout && err != dragonboat.ErrShardNotReady {
				break
			}

			retry++
			if retry > 3 {
				break
			}
			time.Sleep(time.Duration(retry) * 10 * time.Millisecond)
		}
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte(fmt.Sprintf("nooo~! la politzia ... %v", err)))
			return
		}
		w.Write([]byte(result.Data))
	})

	router.GET("/gossip", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		cfg := nh.NodeHostConfig()
		payload, _ := json.Marshal(cfg.Gossip)
		w.Write([]byte(string(payload)))
	})

}

func printMembership(ctx context.Context, nh *dragonboat.NodeHost) (string, error) {

	// ses := nh.GetNoOPSession(defaultShardID)
	var m *dragonboat.Membership
	var err error

	ctx, c := context.WithTimeout(ctx, 2000*time.Millisecond)
	defer c()
	m, err = nh.SyncGetShardMembership(ctx, defaultShardID)

	return fmt.Sprintf("MEMBERZIP %+v %+v", m, err), nil
}
