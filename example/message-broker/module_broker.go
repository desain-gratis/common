package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	notifierapi "github.com/desain-gratis/common/delivery/log-api"
	sm_topic "github.com/desain-gratis/common/delivery/log-api/impl/replicated"
	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v4"
	"github.com/rs/zerolog/log"
)

type broker struct {
	shardID   uint64
	replicaID uint64
	dhost     *dragonboat.NodeHost
}

func (b *broker) GetTopic(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// print list of topic
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "success %v", p.ByName("topic"))
	// print topic metadta
}

func (b *broker) Publish(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// post message to topic
	payload, _ := io.ReadAll(r.Body)

	sess := b.dhost.GetNoOPSession(b.shardID)

	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()

	v, err := b.dhost.SyncPropose(ctx, sess, payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error cuk: %v", err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "success: %v (value: %v)", string(v.Data), v.Value)
}

func (b *broker) Tail(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// tail topic log

	sess := b.dhost.GetNoOPSession(b.shardID)

	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()

	// 1. get & register local instance of the subscription
	v, err := b.dhost.SyncRead(ctx, b.shardID, sm_topic.QuerySubscribe{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "sync read error: %v", err)
		return
	}

	l, ok := v.(notifierapi.Listener)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "not a listener error: %v", err)
		return
	}

	// 2. start consuming data from the subscription
	data, err := json.Marshal(sm_topic.StartSubscriptionData{
		SubscriptionID: l.ID(),
		ReplicaID:      b.replicaID,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error: %v", err)
		return
	}
	payload, _ := json.Marshal(sm_topic.UpdateRequest{
		CmdName: sm_topic.Command_StartSubscription,
		Data:    data,
	})

	_, err = b.dhost.SyncPropose(ctx, sess, payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "lapolizia: %v", err)
		return
	}

	notifier, ok := v.(notifierapi.Listener)
	if !ok {
		http.Error(w, "notifier noT?", http.StatusInternalServerError)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	log.Info().Msgf("LISTENING...")

	w.WriteHeader(http.StatusAccepted)
	w.Header().Add("Content-Type", "text/plain") // so that browser can render them properly

	// can save state here (eg. store last received msg)
	for msg := range notifier.Listen(r.Context()) {
		// client (FE) can do this:
		// client can store G --> last applied
		// if received tail, store the latest applied info as G
		// if received hello, check if we already have G, if yes ignore.
		// if no, we not yet receive tail.. we latest applied info as G.
		// G is used to query logs before the tail;
		// or if we were to use a Key-Value storage snapshot, G is used to
		// query the message between latest applied snapshot to G inclusive.
		data, err := json.Marshal(msg)
		if err != nil {
			log.Err(err).Msgf("marshal feel %v", msg)
			continue
		}

		_, err = fmt.Fprintf(w, "%v\n", string(data))
		if err != nil {
			log.Err(err).Msgf("cannot write uhuy")
			return
		}

		flusher.Flush() // can use ticker to flush every x millis...
	}
}
