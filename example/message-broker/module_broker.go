package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/coder/websocket"
	notifierapi "github.com/desain-gratis/common/example/message-broker/src/log-api"
	sm_topic "github.com/desain-gratis/common/example/message-broker/src/log-api/impl/replicated"
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
}

func (b *broker) Publish(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// post message to topic
	payload, _ := io.ReadAll(r.Body)

	// since we can publish with Tail / connection, and are not web socket, we can use JWT for determining identity..
	// TODO: validate JWT to obtain sender identity (that are created during Tail [TODO] as well
	// parse jwt, and then modify the payload.
	// since we can publish outside the stream connection... / from anywhere.

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
	notifier, err := b.getListener(w)
	if err != nil {
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

type SubscriptionData struct {
	IDToken string
	Name    string
}

func (b *broker) Websocket(w http.ResponseWriter, r *http.Request, p httprouter.Params) {

	name := randomdata.SillyName() + " " + randomdata.LastName()
	id := rand.Int()

	sess := b.dhost.GetNoOPSession(b.shardID)

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"http://localhost:*"},
	})
	if err != nil {
		log.Error().Msgf("error accept %v", err)
		return
	}
	defer c.CloseNow()

	// tail topic log
	notifier, err := b.getListener(w)
	if err != nil {
		log.Error().Msgf("error get listener %v", err)
		return
	}

	// after listening, start input reader goroutine
	cccc, cancer := context.WithCancel(r.Context())
	defer cancer()

	go func() {
		for {
			t, payload, err := c.Read(cccc)
			if err != nil {
				log.Info().Msgf("reader bye bye")
				return
			}

			if t == websocket.MessageBinary {
				log.Info().Msgf("cannot read la")
				continue
			}

			ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
			defer c()

			ppp, _ := json.Marshal(string(payload))

			d := map[string]any{
				"cmd_name": "publish-message",
				"cmd_ver":  0,
				"data": map[string]any{
					"name": name,
					"id":   id,
					"data": json.RawMessage(ppp),
				},
			}

			data, err := json.Marshal(d)
			if err != nil {
				log.Info().Msgf("invalid jsonk %v", string(payload))
				continue
			}

			_, err = b.dhost.SyncPropose(ctx, sess, data)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "error cuk: %v", err)
				return
			}
		}
	}()

	// notify my identity (local)
	idd, _ := json.Marshal(map[string]any{
		"evt_name": "identity",
		"data": map[string]any{
			"name": name,
			"id":   id,
		},
	})
	c.Write(r.Context(), websocket.MessageText, idd)

	// notify i'm online (raft)
	d := map[string]any{
		"cmd_name": "notify-online",
		"cmd_ver":  0,
		"data": map[string]any{
			"name": name,
			"id":   id,
		},
	}

	data, err := json.Marshal(d)
	if err != nil {
		return
	}

	ctx, cca := context.WithTimeout(context.Background(), 5*time.Second)
	defer cca()

	_, err = b.dhost.SyncPropose(ctx, sess, data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error cuk: %v", err)
		return
	}

	for msg := range notifier.Listen(r.Context()) {
		data, err := json.Marshal(msg)
		if err != nil {
			log.Err(err).Msgf("marshal feel %v", msg)
			continue
		}

		c.Write(r.Context(), websocket.MessageText, data)
	}

	log.Info().Msgf("closing..")
	c.Close(websocket.StatusNormalClosure, "")
	log.Info().Msgf("miroslav..")
}

func (b *broker) getListener(w http.ResponseWriter) (notifierapi.Listener, error) {
	sess := b.dhost.GetNoOPSession(b.shardID)

	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()

	// 1. get & register local instance of the subscription
	v, err := b.dhost.SyncRead(ctx, b.shardID, sm_topic.QuerySubscribe{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "sync read error: %v", err)
		return nil, err
	}

	l, ok := v.(notifierapi.Listener)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "not a listener error: %v", err)
		return nil, errors.New("not notifier")
	}

	// 2. start consuming data from the subscription
	data, err := json.Marshal(sm_topic.StartSubscriptionData{
		SubscriptionID: l.ID(),
		ReplicaID:      b.replicaID,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error: %v", err)
		return nil, err
	}
	payload, _ := json.Marshal(sm_topic.UpdateRequest{
		CmdName: sm_topic.Command_StartSubscription,
		Data:    data,
	})

	_, err = b.dhost.SyncPropose(ctx, sess, payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "lapolizia: %v", err)
		return nil, err
	}

	return l, nil
}
