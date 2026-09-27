package impl

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/desain-gratis/common/lib/notifier"
)

const (
	listenQueueSize = 1024
)

var _ notifier.Subscription = &standardSubscriber{}

var (
	ErrClosed     = errors.New("closed")
	ErrNotStarted = errors.New("not started")
)

type standardSubscriber struct {
	id        string
	started   atomic.Bool
	listenCh  chan any
	receiveCh chan any
	done      chan struct{}
}

func NoOp(a any) bool {
	return false
}

// Use t to make sure you have topic before you can subscribe!
// Need to find a way to make sure this one have TOPIC!!
// maybe add param or receiver later...
func NewStandardSubscriber(filterOutFn func(any) bool) notifier.CreateSubscription {
	return func(ctx context.Context, id string) notifier.Subscription {
		c := &standardSubscriber{
			id:        id,
			listenCh:  make(chan any, listenQueueSize),
			receiveCh: make(chan any),
			done:      make(chan struct{}),
		}

		if filterOutFn == nil {
			filterOutFn = NoOp
		}

		// log.Info().Msgf("subscription member: created %v", id)

		// main listener
		go func() {
			defer func() {
				close(c.listenCh)
			}()

			for {
				select {
				case <-ctx.Done():
					c.close()
					return

				case msg := <-c.receiveCh:
					if filterOutFn(msg) {
						continue
					}

					select {
					case c.listenCh <- msg:
					case <-c.done:
						return
					}
				}
			}
		}()

		return c
	}
}

func (c *standardSubscriber) ID() string {
	return c.id
}

// Start allows the control of exact listen time
func (c *standardSubscriber) Start() {
	c.started.Store(true)
}

func (c *standardSubscriber) Listen() <-chan any {
	return c.listenCh
}

func (c *standardSubscriber) Publish(msg any) error {
	if !c.started.Load() {
		return ErrNotStarted
	}

	select {
	case c.receiveCh <- msg:
		return nil

	case <-c.done:
		return ErrClosed
	}
}

func (c *standardSubscriber) close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}
