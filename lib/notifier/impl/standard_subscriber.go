package impl

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/lib/notifier"
)

const (
	listenTimeOut   = 100 * time.Millisecond
	listenQueueSize = 24000
)

var _ notifier.Subscription = &standardSubscriber{}

var (
	ErrClosed     = errors.New("closed")
	ErrNotStarted = errors.New("not started")
)

type standardSubscriber struct {
	id        string
	started   atomic.Bool
	closed    atomic.Bool
	listened  atomic.Bool
	listenCh  chan any
	receiveCh chan any
}

func NewStandardSubscriber(filterOutFn func(any) bool) notifier.CreateSubscription {
	return func(ctx context.Context, id string) notifier.Subscription {
		c := &standardSubscriber{
			id:        id,
			listenCh:  make(chan any, listenQueueSize),
			receiveCh: make(chan any),
		}

		if filterOutFn == nil {
			filterOutFn = func(a any) bool {
				return false
			}
		}

		log.Info().Msgf("subscription member: created %v", id)

		// main listener
		go func() {
			wg := sync.WaitGroup{}
			defer func() {
				wg.Wait()
				close(c.listenCh)
				log.Info().Msgf("subscription member: closed properly %v", id)
			}()

			for {
				select {
				case <-ctx.Done():
					log.Info().Msgf("subscription member: closing %v cause: %v", id, context.Cause(ctx))

					c.closed.Store(true)
					close(c.receiveCh)

					return
				case <-time.After(listenTimeOut):
					if c.listened.Load() {
						continue
					}

					log.Info().Msgf("subscription member: listen timed out %v", id)

					c.closed.Store(true)
					close(c.receiveCh)

					return
				case msg := <-c.receiveCh:
					if filterOutFn(msg) {
						continue
					}
					c.listenCh <- msg
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
	c.listened.Store(true)
	return c.listenCh
}

func (c *standardSubscriber) Publish(msg any) error {
	if c.closed.Load() {
		return ErrClosed
	}

	if !c.started.Load() {
		return ErrNotStarted
	}

	// we do not reject based on !c.listened,
	// we want to queue messages after publisher Start() them

	// maybe we can add statistics eg. number of publishhed messages..

	c.receiveCh <- msg

	return nil
}
