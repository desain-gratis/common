package raftenabledapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v3"
	"github.com/lni/dragonboat/v3/config"
)

type dragonboatImpl struct {
	nh *dragonboat.NodeHost
}

func NewDragonboatReplica(nh *dragonboat.NodeHost) *dragonboatImpl {
	return nil
}

type initializeRequest struct {
	Password       string `json:"password"`
	GSIClientID    string `json:"gsi_client_id"`
	GSIAdminEmails string `json:"gsi_admin_emails"`
	JWTSigningKey  string `json:"jwt_signing_key"`

	InitialMember map[uint64]dragonboat.Target `json:"initial_member"`
}

func (d *dragonboatImpl) StartReplica(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}

	var req initializeRequest
	err = json.Unmarshal(payload, &req)
	if err != nil {
		return
	}

	rc := config.Config{
		ReplicaID: 0,
		ShardID:   0,
	}

	err = d.nh.StartOnDiskReplica(req.InitialMember, false, NewDiskKV, rc)
	if err != nil {
		return
	}
}
