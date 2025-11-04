package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/julienschmidt/httprouter"

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
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	subs, err := c.topic.Subscribe(r.Context(), impl.NewStandardSubscriber(nil))
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
