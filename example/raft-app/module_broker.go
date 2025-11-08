package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/coder/websocket"
	raftchat "github.com/desain-gratis/common/example/raft-app/src/app/impl/raft-chat"
	"github.com/desain-gratis/common/lib/notifier"
	notifier_impl "github.com/desain-gratis/common/lib/notifier/impl"
	"github.com/julienschmidt/httprouter"
	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/client"
	"github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

// An integration layer that can interacts with specified replica app
type chatAppIntegration struct {
	shardID   uint64
	replicaID uint64
	dhost     *dragonboat.NodeHost
	sess      *client.Session
}

type SubscriptionData struct {
	IDToken string
	Name    string
}

func (b *chatAppIntegration) Websocket(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	ctx := r.Context()
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{
			"http://localhost:*", "http://localhost",
			"https://chat.desain.gratis", "http://dxb-keenan.tailnet-ee99.ts.net",
			"https://dxb-keenan.tailnet-ee99.ts.net",
			"https://mb.desain.gratis",
		},
	})
	if err != nil {
		log.Error().Msgf("error accept %v", err)
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "super duper X")

	wsWg := r.Context().Value("ws-wg").(*sync.WaitGroup)
	wsWg.Add(1)
	defer wsWg.Done()

	lctx, lcancel := context.WithCancel(ctx)
	pctx, pcancel := context.WithCancel(context.Background())

	id := rand.Int()
	name := randomdata.SillyName() + " " + randomdata.LastName()

	var signedIn bool

	sessID := &SessID{
		ID:   id,
		Name: name,
	}

	// Reader goroutine, detect client connection close as well.
	go func() {
		// close both listener & publisher ctx if client is the one closing
		defer lcancel()
		defer pcancel()

		defer func() {
			if !signedIn {
				return
			}
			// notify i'm offline (raft)
			msg := map[string]any{
				"cmd_name": "notify-offline",
				"cmd_ver":  0,
				"data": map[string]any{
					"name": name,
					"id":   id,
				},
			}
			_, err := b.publishToRaft(pctx, msg)
			if err != nil {
				return
			}

			log.Info().Msgf("closed connection for: %v", name)
		}()

		for {
			t, payload, err := c.Read(pctx)
			if websocket.CloseStatus(err) > 0 {
				return
			}
			if err != nil {
				// log.Warn().Msgf("unknown error. closing connection: %v")
				return
			}

			if t == websocket.MessageBinary {
				log.Info().Msgf("cannot read la")
				continue
			}

			err = b.parseMessage(pctx, c, sessID, payload)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}

				log.Err(err).Msgf("unknown error. closing connection")
				return
			}
		}
	}()

	// notify my identity (local)
	msg := map[string]any{
		"evt_name": "identity",
		"data": map[string]any{
			"name": name,
			"id":   id,
		},
	}
	err = b.publishTextToWebsocket(pctx, c, msg)
	if err != nil {
		if errors.Is(err, context.Canceled) || pctx.Err() != nil || lctx.Err() != nil {
			return
		}
		// or warn..
		// log.Error().Msgf("error publish notify-online message %v", err)
		return
	}

	// simple protection (to state machine) against quick open-close connection
	time.Sleep(100 * time.Millisecond)
	if pctx.Err() != nil || lctx.Err() != nil {
		return
	}

	// subscribe until server closed / client closed
	subscribeCtx, cancelSubscribe := context.WithCancelCause(context.Background())

	// merge context
	go func() {
		select {
		case <-lctx.Done():
			cancelSubscribe(errors.New("server closed"))
		case <-pctx.Done():
			cancelSubscribe(errors.New("client closed"))
		}
	}()

	subscription, listenOffset, err := b.getSubscription(subscribeCtx, notifier_impl.NewStandardSubscriber(nil), name)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, dragonboat.ErrShardNotReady) {
			return
		}
		if errors.Is(err, dragonboat.ErrCanceled) {
			// log.Warn().Msgf("error get listener %v", err)
			return
		}

		log.Err(err).Msgf("error get listener %v", err)
		return
	}

	// notify my identity (local)
	msg = map[string]any{
		"evt_name": "chat-offset",
		"data": map[string]any{
			"offset": listenOffset,
		},
	}
	err = b.publishTextToWebsocket(pctx, c, msg)
	if err != nil {
		if errors.Is(err, context.Canceled) || pctx.Err() != nil || lctx.Err() != nil {
			return
		}
		// or warn..
		// log.Error().Msgf("error publish notify-online message %v", err)
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
	_, err = b.publishToRaft(pctx, msg)
	if err != nil {
		if errors.Is(err, context.Canceled) || pctx.Err() != nil || lctx.Err() != nil {
			return
		}

		// or warn
		log.Warn().Msgf("error publish notify-online message %v", err)
		return
	}

	signedIn = true

	// Loading last 1 days data..

	aDayBefore := time.Now().AddDate(0, 0, -1).Local().Truncate(time.Hour * 24)
	log.Info().Msgf("a day before: %v", aDayBefore.Format(time.RFC3339))
	err = b.queryLog(pctx, c, raftchat.QueryLog{
		ToOffset:     listenOffset,
		FromDatetime: &aDayBefore,
		Ctx:          pctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || pctx.Err() != err {
			return
		}

		log.Error().Msgf("error querying last log %v", err)
		return
	}

	for anymsg := range subscription.Listen() {
		if pctx.Err() != nil {
			break
		}

		msg, ok := anymsg.(raftchat.Event)
		if !ok {
			log.Error().Msgf("its not an event 😔 %T %+v", msg, msg)
			continue
		}

		err = b.publishTextToWebsocket(pctx, c, map[string]any{
			"evt_name":         msg.EvtName,
			"table":            msg.EvtTable,
			"evt_id":           msg.EvtID,
			"server_timestamp": msg.ServerTimestamp,
			"data":             json.RawMessage(msg.Data),
		})
		if err != nil && websocket.CloseStatus(err) == -1 {
			if pctx.Err() != nil {
				return
			}
			// log.Warn().Msgf("err listen to notifier event: %v %v", err, string(data))
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
		if errors.Is(err, context.Canceled) {
			return
		}
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

func (b *chatAppIntegration) publishToRaft(ctx context.Context, msg any) ([]byte, error) {
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

func (b *chatAppIntegration) publishTextToWebsocket(ctx context.Context, wsconn *websocket.Conn, msg any) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
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

// getSubscription to the state machine topic, and then start listening for publish
func (b *chatAppIntegration) getSubscription(ctx context.Context, csfn notifier.CreateSubscription, name string) (
	notifier.Listener, uint64, error) {
	rctx, c := context.WithTimeout(ctx, 5*time.Second)
	defer c()

	// 1. get & register local instance of the subscription, but not yet received any event
	v, err := b.dhost.SyncRead(rctx, b.shardID, raftchat.Subscribe{})
	if err != nil {
		return nil, 0, err
	}

	// abort if contxet deadline
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	topic, ok := v.(notifier.Topic)
	if !ok {
		return nil, 0, errors.New("not notifier")
	}

	subscription, err := topic.Subscribe(ctx, csfn)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, 0, err
		}
		log.Err(err).Msgf("LOH PAK BU")
		return nil, 0, err
	}

	// 2. ask state-machine to start receiving data
	data, err := json.Marshal(raftchat.StartSubscriptionData{
		SubscriptionID: subscription.ID(),
		ReplicaID:      b.replicaID,
		Debug:          name,
	})
	if err != nil {
		return nil, 0, err
	}

	payload, _ := json.Marshal(raftchat.UpdateRequest{
		CmdName: raftchat.Command_StartSubscription,
		Data:    data,
	})

	ctx2, c2 := context.WithTimeout(ctx, 5*time.Second)
	defer c2()

	result, err := b.dhost.SyncPropose(ctx2, b.sess, payload)
	if err != nil {
		return nil, 0, err
	}

	// receive the listener offset

	startListenIdx := result.Value

	return subscription, startListenIdx, nil
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

func (b *chatAppIntegration) parseMessage(pctx context.Context, wsconn *websocket.Conn, sessID *SessID, payload []byte) error {
	var cmd Command
	if err := json.Unmarshal(payload, &cmd); err != nil {
		// ignore
	}

	switch cmd.Type {
	case "query-log":
		var qlog raftchat.QueryLog
		if err := json.Unmarshal(cmd.Data, &qlog); err != nil {
			return err
		}
		qlog.Ctx = pctx
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

	_, err := b.publishToRaft(pctx, msg)
	if err != nil {
		log.Error().Msgf("error propose %v", err)
		return err
	}
	return nil
}

func (b *chatAppIntegration) queryLog(pctx context.Context, wsconn *websocket.Conn, qlog raftchat.QueryLog) error {
	ctx, c := context.WithTimeout(pctx, 5*time.Second)
	defer c()

	q, err := b.dhost.SyncRead(ctx, b.shardID, qlog)
	if err != nil {
		return err
	}
	logstream, ok := q.(chan raftchat.Event)
	if !ok {
		return errors.New("it's not an event")
	}

	log.Info().Msgf("logstream acquired") // todo: investigate why nyangkut

	defer log.Info().Msgf("logstream released")
	for msg := range logstream {
		if pctx.Err() != err {
			return nil
		}

		d := map[string]any{
			"evt_name":         msg.EvtName,
			"table":            msg.EvtTable,
			"evt_id":           msg.EvtID,
			"server_timestamp": msg.ServerTimestamp,
			"data":             json.RawMessage(msg.Data),
		}

		err = b.publishTextToWebsocket(pctx, wsconn, d)
		if err != nil {
			return err
		}
	}

	return nil
}

// https://vitaneri.com/posts/check-for-nil-interface-in-go
func checkNilInterface(i interface{}) bool {
	iv := reflect.ValueOf(i)
	if !iv.IsValid() {
		return true
	}
	switch iv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Func, reflect.Interface:
		return iv.IsNil()
	default:
		return false
	}
}
