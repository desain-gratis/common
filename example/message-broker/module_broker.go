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
	"github.com/lni/dragonboat/v4/client"
	"github.com/lni/dragonboat/v4/statemachine"
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
	notifier, _, err := b.getListener()
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
	appCtx := r.Context().Value("app-ctx").(context.Context)
	// ender := r.Context().Value("ender").(chan struct{})

	// select {
	// case _, closed := <-ender:
	// 	if closed {
	// 		// reject new new connection if server already closing..
	// 		return
	// 	}
	// default:
	// }

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"http://localhost:*", "https://chat.desain.gratis"},
	})
	if err != nil {
		log.Error().Msgf("error accept %v", err)
		return
	}
	defer func() {
		err := c.CloseNow()
		if err != nil {
			log.Err(err).Msgf("failed to close websocket")
		}
		log.Info().Msgf("successfully closed ws connection")
	}()

	wsCtx, cancel := context.WithCancel(appCtx)

	id := rand.Int()
	sess := b.dhost.GetNoOPSession(b.shardID)

	name := randomdata.SillyName() + " " + randomdata.LastName()

	// Reader goroutine
	go func() {
		defer cancel()

		defer func() {
			// notify i'm offline (raft)
			msg := map[string]any{
				"cmd_name": "notify-offline",
				"cmd_ver":  0,
				"data": map[string]any{
					"name": name,
					"id":   id,
				},
			}
			_, err := b.publishToRaft(wsCtx, sess, msg)
			if err != nil {
				return
			}

		}()

		for {
			t, payload, err := c.Read(wsCtx)
			if websocket.CloseStatus(err) > 0 {
				log.Info().Msgf("closing connection")
				return
			}
			if err != nil {
				log.Err(err).Msgf("unknown error. closing connection")
				return
			}

			if t == websocket.MessageBinary {
				log.Info().Msgf("cannot read la")
				continue
			}

			ppp, _ := json.Marshal(string(payload))

			msg := map[string]any{
				"cmd_name": "publish-message",
				"cmd_ver":  0,
				"data": map[string]any{
					"name": name,
					"id":   id,
					"data": json.RawMessage(ppp),
				},
			}

			_, err = b.publishToRaft(wsCtx, sess, msg)
			if err != nil {
				log.Error().Msgf("error propose %v", err)
				continue
			}
		}
	}()

	// tail chat log
	notifier, chatOffset, err := b.getListener()
	if err != nil {
		log.Error().Msgf("error get listener %v", err)
		return
	}

	// notify my identity (local)
	msg := map[string]any{
		"evt_name": "identity",
		"data": map[string]any{
			"name":   name,
			"id":     id,
			"offset": chatOffset,
		},
	}
	err = b.publishTextToWebsocket(wsCtx, c, msg)
	if err != nil {
		log.Error().Msgf("error publish notify-online message %v", err)
		return
	}

	// notify i'm online to raft
	msg = map[string]any{
		"cmd_name": "notify-online",
		"cmd_ver":  0,
		"data": map[string]any{
			"name": name,
			"id":   id,
		},
	}
	_, err = b.publishToRaft(wsCtx, sess, msg)
	if err != nil {
		log.Error().Msgf("error publish notify-online message %v", err)
		return
	}

	for msg := range notifier.Listen(wsCtx) {
		data, err := json.Marshal(msg)
		if err != nil {
			log.Err(err).Msgf("marshal feel %v", msg)
			continue
		}

		c.Write(wsCtx, websocket.MessageText, data)
	}

	log.Info().Msgf("Did i get up?")

	// server close
	d := map[string]any{
		"evt_name": "listen-server-closed",
		"evt_ver":  0,
		"data":     "Server closed.",
	}

	err = b.publishTextToWebsocket(wsCtx, c, d)
	if err != nil {
		log.Error().Msgf("oyoyoyoy... %v %v", err, "Server closed")
	}

	err = c.Close(websocket.StatusNormalClosure, "super duper X")
	if err != nil {
		log.Err(err).Msgf("failed to close websocket connection normally")
	}

	log.Info().Msgf("websocket connection closed")
}

func (b *broker) publishToRaft(ctx context.Context, sess *client.Session, msg any) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	var res statemachine.Result
	for range 3 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res, err = b.dhost.SyncPropose(ctx, sess, data)
		if err == nil {
			cancel()
			break
		}
		cancel()
		time.Sleep(500 * time.Millisecond)
	}

	return res.Data, nil
}

func (b *broker) publishTextToWebsocket(ctx context.Context, wsconn *websocket.Conn, msg any) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	err = wsconn.Write(ctx, websocket.MessageText, payload)
	if err != nil {
		return err
	}

	return nil
}

func (b *broker) getListener() (notifierapi.Listener, uint64, error) {
	sess := b.dhost.GetNoOPSession(b.shardID)

	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()

	// 1. get & register local instance of the subscription
	v, err := b.dhost.SyncRead(ctx, b.shardID, sm_topic.QuerySubscribe{})
	if err != nil {
		return nil, 0, err
	}

	l, ok := v.(notifierapi.Listener)
	if !ok {
		return nil, 0, errors.New("not notifier")
	}

	// 2. start consuming data from the subscription
	data, err := json.Marshal(sm_topic.StartSubscriptionData{
		SubscriptionID: l.ID(),
		ReplicaID:      b.replicaID,
	})
	if err != nil {
		return nil, 0, err
	}

	payload, _ := json.Marshal(sm_topic.UpdateRequest{
		CmdName: sm_topic.Command_StartSubscription,
		Data:    data,
	})

	result, err := b.dhost.SyncPropose(ctx, sess, payload)
	if err != nil {
		return nil, 0, err
	}

	return l, result.Value, nil
}

type Message struct {
	Type string
	Data json.RawMessage
}

type QueryChat struct {
	CurrentOffset uint64
}

func parseMessage(payload []byte) error {
	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	switch msg.Type {
	case "chat":
		// send data to raft
		// return immediately ()
	case "query-chat":
		var queryChat QueryChat
		if err := json.Unmarshal(msg.Data, &queryChat); err != nil {
			return err
		}
		// query, get iterator, write to web socket for each entry
	}

	return errors.New("message type not supported")
}
