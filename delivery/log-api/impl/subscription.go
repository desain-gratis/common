package impl

import (
	"context"
	"errors"
	"time"

	notifierapi "github.com/desain-gratis/common/delivery/log-api"
)

var _ notifierapi.Subscription = &subscription{}

var (
	ErrListenerClosed = errors.New("closed")
	ErrListenerEmpty  = errors.New("no listener")
)

type subscription struct {
	closed      bool
	ch          chan any
	exitMessage any
	async       bool
	listen      bool
}

func NewSubscription(async bool, bufferSize uint32, exitMessage any, listenTimeout time.Duration) *subscription {
	// add go routine to Close this subscription
	// if it's not listened up immediately after certain time (eg. 2 seconds)
	s := &subscription{
		async:       async,
		exitMessage: exitMessage,
		ch:          make(chan any, bufferSize),
	}

	if listenTimeout > 0 {
		go func() {
			time.Sleep(listenTimeout)
			s.Close()
		}()
	}

	return s
}

func (c *subscription) Listen(ctx context.Context) <-chan any {
	go func() {
		defer close(c.ch)

		<-ctx.Done()
		c.listen = false
		c.Close()
	}()

	c.listen = true

	return c.ch
}

func (c *subscription) Close() {
	if c.listen {
		// once listened, the subscription can only be closed by
		// expiring/cancelling the context in Listen method
		return
	}

	c.closed = true
	if c.exitMessage != nil {
		c.ch <- c.exitMessage
	}
}

func (c *subscription) Publish(_ context.Context, msg any) error {
	if c.closed {
		return ErrListenerClosed
	}

	if c.ch == nil {
		return ErrListenerEmpty
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
