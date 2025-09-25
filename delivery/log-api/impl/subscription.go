package impl

import (
	"context"
	"errors"
	"reflect"
	"time"

	notifierapi "github.com/desain-gratis/common/delivery/log-api"
)

var _ notifierapi.Subscription = &subscription{}

var (
	ErrListenerClosed = errors.New("closed")
	ErrListenerEmpty  = errors.New("no listener")
	ErrNotStarted     = errors.New("not started")
	ErrListenTimedOut = errors.New("listen timed out")
)

type subscription struct {
	id          string
	started     bool
	closed      bool
	ch          chan any
	exitMessage any
	async       bool
	listen      bool
	timeout     bool
}

func NewSubscription(id string, async bool, bufferSize uint32, exitMessage any, listenTimeout time.Duration) *subscription {
	// add go routine to Close this subscription
	// if it's not listened up immediately after certain time (eg. 2 seconds)
	s := &subscription{
		async:       async,
		exitMessage: exitMessage,
		ch:          make(chan any, bufferSize),
		id:          id,
	}

	if listenTimeout > 0 {
		go func() {
			time.Sleep(listenTimeout)
			s.timeout = true
		}()
	}

	return s
}
func (c *subscription) ID() string {
	return c.id
}

func (c *subscription) Start() {
	c.started = true
}

func (c *subscription) Listen(ctx context.Context) <-chan any {
	go func() {
		defer close(c.ch)

		<-ctx.Done()
		c.closed = true
		if !checkNilInterface(c.exitMessage) {
			c.ch <- c.exitMessage
		}
	}()

	c.listen = true

	return c.ch
}

func (c *subscription) Publish(_ context.Context, msg any) error {
	if c.closed {
		return ErrListenerClosed
	}

	if !c.started {
		return ErrNotStarted
	}

	if c.timeout && !c.listen {
		return ErrListenTimedOut
	}

	if c.async {
		go func(msg any) {
			c.ch <- msg
		}(msg)
	} else {
		c.ch <- msg
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
