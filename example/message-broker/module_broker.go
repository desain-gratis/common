package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/coder/websocket"
	notifierapi "github.com/desain-gratis/common/example/message-broker/src/log-api"
	"github.com/desain-gratis/common/example/message-broker/src/log-api/impl/replicated"
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
	sess      *client.Session
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

	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()

	v, err := b.dhost.SyncPropose(ctx, b.sess, payload)
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
	notifier, _, err := b.getListener("csv")
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
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"http://localhost:*", "http://localhost", "https://chat.desain.gratis", "http://dxb-keenan.tailnet-ee99.ts.net", "https://dxb-keenan.tailnet-ee99.ts.net"},
	})
	if err != nil {
		log.Error().Msgf("error accept %v", err)
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "super duper X")

	wsWg := r.Context().Value("ws-wg").(*sync.WaitGroup)
	wsWg.Add(1)
	defer wsWg.Done()

	lctx, lcancel := context.WithCancel(r.Context().Value("app-ctx").(context.Context))
	pctx, pcancel := context.WithCancel(context.Background())

	id := rand.Int()
	name := randomdata.SillyName() + " " + randomdata.LastName()

	sessID := &SessID{
		ID:   id,
		Name: name,
	}
	// Reader goroutine, detect client connection close as well.
	go func() {
		// close listener & publisher ctx
		defer lcancel()
		defer pcancel()

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
			_, err := b.publishToRaft(pctx, b.sess, msg)
			if err != nil {
				return
			}
		}()

		for {
			t, payload, err := c.Read(pctx)
			if websocket.CloseStatus(err) > 0 {
				log.Info().Msgf("closing connection..")
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

			err = b.parseMessage(pctx, c, b.sess, sessID, payload)
			if err != nil {
				log.Err(err).Msgf("unknown error. closing connection")
				return
			}
		}
	}()

	// tail chat log
	notifier, chatOffset, err := b.getListener(name)
	if err != nil {
		log.Error().Msgf("error get listener %v", err)
		return
	}

	// todo: defer query unsubscribe to avoid nyangkut
	// investigate nyangkut case when add (but it's not gotten to subscriber in Topic)
	// bisa ngirim, tetapi gak dapet message jadinya..

	// todo:
	// ada juga kasus connected, but disconnected (when starting up)

	// notify my identity (local)
	msg := map[string]any{
		"evt_name": "identity",
		"data": map[string]any{
			"name":   name,
			"id":     id,
			"offset": chatOffset,
		},
	}
	err = b.publishTextToWebsocket(pctx, c, msg)
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
	_, err = b.publishToRaft(pctx, b.sess, msg)
	if err != nil {
		log.Error().Msgf("error publish notify-online message %v", err)
		return
	}

	// Loading last 7 dayds data..

	aWeekBefore := time.Now().AddDate(0, 0, 7)
	err = b.queryLog(pctx, c, replicated.QueryLog{
		CurrentOffset: chatOffset,
		FromDateTime:  &aWeekBefore,
	})
	if err != nil {
		log.Error().Msgf("error querying last log %v", err)
		return
	}

	for msg := range notifier.Listen(lctx) {
		data, err := json.Marshal(msg)
		if err != nil {
			log.Err(err).Msgf("marshal feel %v", msg)
			continue
		}

		err = c.Write(pctx, websocket.MessageText, data)
		if err != nil && websocket.CloseStatus(err) == -1 {
			log.Err(err).Msgf("err listen to notifier event %v", string(data))
			return
		}
	}

	// if we cannot publish anymore, return immediately
	if err := pctx.Err(); err != nil {
		return
	}

	// else, send goodbye message
	d := map[string]any{
		"evt_name": "listen-server-closed",
		"evt_ver":  0,
		"data":     "Server closed.",
	}

	err = b.publishTextToWebsocket(pctx, c, d)
	if err != nil && websocket.CloseStatus(err) == -1 {
		log.Err(err).Msgf("failed to send message")
		return
	}

	err = c.Close(websocket.StatusNormalClosure, "super duper X")
	if err != nil && websocket.CloseStatus(err) == -1 {
		log.Err(err).Msgf("failed to close websocket connection normally")
		return
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
		res, err = b.dhost.SyncPropose(ctx, b.sess, data)
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

func (b *broker) getListener(name string) (notifierapi.Listener, uint64, error) {
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
		Debug:          name,
	})
	if err != nil {
		return nil, 0, err
	}

	payload, _ := json.Marshal(sm_topic.UpdateRequest{
		CmdName: sm_topic.Command_StartSubscription,
		Data:    data,
	})

	ctx2, c2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer c2()

	result, err := b.dhost.SyncPropose(ctx2, b.sess, payload)
	if err != nil {
		return nil, 0, err
	}

	return l, result.Value, nil
}

type Command struct {
	Type    string
	Version uint32
	Data    json.RawMessage
}
type SessID struct {
	ID   int
	Name string
}

func (b *broker) parseMessage(pctx context.Context, wsconn *websocket.Conn, raftSess *client.Session, sessID *SessID, payload []byte) error {
	var cmd Command
	if err := json.Unmarshal(payload, &cmd); err != nil {
		// ignore
	}

	switch cmd.Type {
	case "query-log":
		var qlog replicated.QueryLog
		if err := json.Unmarshal(cmd.Data, &qlog); err != nil {
			return err
		}
		return b.queryLog(pctx, wsconn, qlog)
	case "send-chat":
	}

	// so it's not breaking client
	ppp, _ := json.Marshal(string(payload))
	msg := map[string]any{
		"cmd_name": "publish-message",
		"cmd_ver":  0,
		"data": map[string]any{
			"name": sessID.Name,
			"id":   sessID.ID,
			"data": json.RawMessage(ppp),
		},
	}

	_, err := b.publishToRaft(pctx, raftSess, msg)
	if err != nil {
		log.Error().Msgf("error propose %v", err)
		return err
	}
	return nil
}

func (b *broker) queryLog(pctx context.Context, wsconn *websocket.Conn, qlog replicated.QueryLog) error {
	ctx, c := context.WithTimeout(pctx, 5*time.Second)
	defer c()

	q, err := b.dhost.SyncRead(ctx, b.shardID, qlog)
	if err != nil {
		return err
	}
	logstream, ok := q.(chan replicated.Log)
	if !ok {
		return errors.New("it's not a log")
	}

	log.Info().Msgf("logstream acquired") // todo: investigate why nyangkut

	defer log.Info().Msgf("logstream released")
	for msg := range logstream {
		d := map[string]any{
			"evt_name":         "echo",
			"evt_ver":          0,
			"table":            "chat_log",
			"server_timestamp": msg.ServerTimestamp,
			"evt_id":           msg.EventID,
			"data":             json.RawMessage(msg.Data),
		}

		err = b.publishTextToWebsocket(pctx, wsconn, d)
		if err != nil {
			return err
		}
	}

	return nil
}
