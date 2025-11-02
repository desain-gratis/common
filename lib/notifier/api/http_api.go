package notifier

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/desain-gratis/common/lib/notifier"
	"github.com/desain-gratis/common/lib/notifier/impl"
	"github.com/julienschmidt/httprouter"
)

type api struct {
	n         notifier.Topic
	transform func(v any) any
}

func NewDebugAPI(n notifier.Topic) *api {
	return &api{n: n}
}

// todo; make it streaming DSL
func (a *api) WithTransform(transform func(v any) any) *api {
	a.transform = transform
	return a
}

// http helper to listen notify event; all event will be listened
func (c *api) ListenHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	var chatSubscription = func(ctx context.Context, key string) notifier.Subscription {
		return impl.NewSubscription(ctx, r.Context(), key, "bye bye", nil)
	}

	subs, err := c.n.Subscribe(r.Context(), chatSubscription)
	if err != nil {
		http.Error(w, "failed to subscribe to topic", http.StatusInternalServerError)
		return
	}

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
func (c *api) PublishHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	c.n.Broadcast(r.Context(), b)
}
