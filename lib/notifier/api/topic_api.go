package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/lib/notifier"
	"github.com/desain-gratis/common/lib/notifier/impl"
)

type api struct {
	topic     notifier.Topic
	transform func(v any) any
}

func NewTopicAPI(topic notifier.Topic, transform func(v any) any) *api {
	return &api{topic: topic, transform: transform}
}

func (c *api) Metrics(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	stat, ok := c.topic.(notifier.Metric)
	if !ok {
		http.Error(w, "topic implementation does not support metric query", http.StatusInternalServerError)
		return
	}

	payload, err := json.Marshal(stat.GetMetric())
	if err != nil {
		http.Error(w, "failed to parse metric", http.StatusInternalServerError)
		return
	}

	w.Write(payload)
}

// http helper to listen notify event; all event will be listened
// NOTE: interesting that Google chrome:
// 1. if we not yet press enter, already started the connection.
// 2. if we open multiple, they do not create new (it seems reusing the old connection?)
// .   WORKAROUND: add random values to the URL param
func (c *api) Tail(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	c.TailFilterOut(nil)(w, r, p)
}

func (c *api) TailFilterOut(filterFunc func(msg any) bool) func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		subs, err := c.topic.Subscribe(r.Context(), impl.NewStandardSubscriber(filterFunc))
		if err != nil {
			http.Error(w, "failed to subscribe to topic", http.StatusInternalServerError)
			return
		}

		subs.Start()

		for msg := range subs.Listen() {
			if c.transform != nil {
				msg = c.transform(msg)
			}

			_, err := fmt.Fprintf(w, "%v\n", msg)
			if err != nil {
				return
			}

			flusher.Flush() // can use ticker to flush every x millis...
		}
	}
}

// http util helper to publish directly
// for debugging only, only support consumer inside the same process
func (c *api) Publish(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	c.topic.Broadcast(r.Context(), b)
}

// originPatterns:
//
//	[]string{
//		"http://localhost:*", "http://localhost",
//		"https://chat.desain.gratis", "http://dxb-keenan.tailnet-ee99.ts.net",
//		"https://dxb-keenan.tailnet-ee99.ts.net",
//		"https://mb.desain.gratis",
//	}
func (apii *api) Websocket(appCtx context.Context, originPatterns []string, filterFunc func(msg any) bool) func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// TODO: later you can create your own hander;
	// this one is just for convenience
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		ctx := r.Context()

		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: originPatterns,
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

		// Reader goroutine, detect client connection close as well.
		go func() {
			// close both listener & publisher ctx if client is the one closing
			defer lcancel()
			defer pcancel()

			for {
				_, _, err := c.Read(pctx)
				if websocket.CloseStatus(err) > 0 {
					return
				}
				if err != nil {
					return
				}

				// if t == websocket.MessageBinary {
				// 	log.Info().Msgf("cannot read la")
				// 	continue
				// }

				// parse message; but in our case, it's only read

				// err = b.parseMessage(pctx, c, sessID, payload)
				// if err != nil {
				// 	if errors.Is(err, context.Canceled) {
				// 		return
				// 	}

				// 	log.Err(err).Msgf("unknown error. closing connection")
				// 	return
				// }
			}
		}()

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

		subscription, err := apii.topic.Subscribe(subscribeCtx, impl.NewStandardSubscriber(filterFunc))
		if err != nil {
			return
		}

		for anymsg := range subscription.Listen() {
			if pctx.Err() != nil {
				break
			}

			err = publishTextToWebsocket(pctx, c, anymsg)
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

		err = publishTextToWebsocket(pctx, c, map[string]string{"message": "bye bye"}) // todo:..
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
}

func publishTextToWebsocket(ctx context.Context, wsconn *websocket.Conn, msg any) error {
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
