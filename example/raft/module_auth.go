package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	idtokensigner "github.com/desain-gratis/common/delivery/auth-api/idtoken-signer"
	idtokenverifier "github.com/desain-gratis/common/delivery/auth-api/idtoken-verifier"
	plugin "github.com/desain-gratis/common/example/raft/app-auth"
	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/statemachine"
)

func enableAuthAPI(router *httprouter.Router, nh *dragonboat.NodeHost, info map[string]any, haveLeader *bool) {
	signerVerifier := idtokensigner.NewSimple(
		"raft-app",
		map[string]string{
			"key-v1": "85746{K=q's)",
		},
		"key-v1",
	)

	gsiAuth := idtokenverifier.GSIAuth("web client ID")
	appAuth := idtokenverifier.AppAuth(signerVerifier, "app")

	adminTokenBuilder := plugin.AdminAuthLogic(map[string]struct{}{"keenan.gebze@gmail.com": struct{}{}}, 1*30)
	userTokenBuilder := plugin.NewUserAuthLogic(nil, 8*60)

	router.GET("/auth/admin", gsiAuth(authapi.GetToken(adminTokenBuilder, signerVerifier)))
	router.GET("/auth/user/gsi", gsiAuth(authapi.GetToken(userTokenBuilder, signerVerifier)))

	router.GET("/member-only", appAuth(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {}))

	sess := nh.GetNoOPSession(defaultShardID)
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
}
