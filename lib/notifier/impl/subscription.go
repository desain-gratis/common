package impl

import (
	"context"
	"errors"
	"reflect"
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

var _ notifier.Subscription = &subscription{}

var (
	ErrClosed     = errors.New("closed")
	ErrNotStarted = errors.New("not started")
)

type subscription struct {
	id        string
	started   atomic.Bool
	closed    atomic.Bool
	listened  atomic.Bool
	listenCh  chan any
	receiveCh chan any

	exitMessage any
}

func NewSubscription(requestCtx, appCtx context.Context, id string, exitMessage any, filterOutFn func(any) bool) *subscription {
	c := &subscription{
		id:          id,
		listenCh:    make(chan any, listenQueueSize),
		receiveCh:   make(chan any),
		exitMessage: exitMessage,
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
			case <-appCtx.Done():
				log.Info().Msgf("subscription member: closing (server stop) %v", id)

				c.closed.Store(true)
				close(c.receiveCh)

				if !checkNilInterface(c.exitMessage) {
					wg.Add(1)
					go func() {
						defer wg.Done()
						c.listenCh <- c.exitMessage
					}()
				}
				return
			case <-requestCtx.Done():
				log.Info().Msgf("subscription member: closing (client stop listening) %v", id)

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

func (c *subscription) ID() string {
	return c.id
}

// Start allows the control of exact listen time
func (c *subscription) Start() {
	c.started.Store(true)
}

func (c *subscription) Listen() <-chan any {
	c.listened.Store(true)
	return c.listenCh
}

func (c *subscription) Publish(msg any) error {
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
