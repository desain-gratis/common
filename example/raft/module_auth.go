package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/client"
)

func enableAuthAPI(router *httprouter.Router, nh *dragonboat.NodeHost, ses *client.Session, info map[string]any) {
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

		res, err := nh.SyncPropose(ctx, ses, []byte(`{"type":"echo", "payload": "assalamualaikum"}`))
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

}
