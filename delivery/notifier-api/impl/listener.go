package impl

import (
	"context"
	"fmt"

	notifierapi "github.com/desain-gratis/common/delivery/notifier-api"
)

var _ notifierapi.Subscription = &listener{}

type listener struct {
	closed      bool
	ch          chan any
	exitMessage any
	async       bool
}

func (c *listener) Listen(ctx context.Context) <-chan any {
	subscribeChan := make(chan any)
	c.ch = subscribeChan

	go func() {
		defer close(subscribeChan)

		<-ctx.Done()

		c.closed = true
		if c.exitMessage != nil {
			subscribeChan <- c.exitMessage
		}
	}()

	return c.ch
}

func (c *listener) Publish(_ context.Context, msg any) error {
	if c.closed {
		return fmt.Errorf("closed")
	}

	if c.ch == nil {
		return fmt.Errorf("no listener")
	}

	if c.async {
		go func() {
			c.ch <- msg
		}()
	} else {
		c.ch <- msg
	}

	return nil
}
