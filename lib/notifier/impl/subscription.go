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
	listenTimeOut = 100 * time.Millisecond
)

var _ notifier.Subscription = &subscription{}

var (
	ErrClosed         = errors.New("closed")
	ErrListenerEmpty  = errors.New("no listener")
	ErrNotStarted     = errors.New("not started")
	ErrListenTimedOut = errors.New("listen timed out")
)

// TODO: major refactor this
type subscription struct {
	id         string
	started    atomic.Bool
	closed     atomic.Bool
	listened   atomic.Bool
	ch         chan any
	listenChan chan any

	exitMessage any
}

func NewSubscription(requestCtx, appCtx context.Context, id string, exitMessage any, filterOutFn func(any) bool) *subscription {
	// add go routine to Close this subscription
	// if it's not listened up immediately after certain time (eg. 2 seconds)
	c := &subscription{
		id:          id,
		exitMessage: exitMessage,
		ch:          make(chan any, 20000), // currently arbitrary
		listenChan:  make(chan any),
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
			close(c.ch)
			log.Info().Msgf("subscription member: closed properly %v", id)
		}()

		for {
			select {
			case <-appCtx.Done():
				log.Info().Msgf("subscription member: closing (server stop) %v", id)

				c.closed.Store(true)
				close(c.listenChan)

				if !checkNilInterface(c.exitMessage) {
					wg.Add(1)
					go func() {
						defer wg.Done()
						c.ch <- c.exitMessage
					}()
				}
				return
			case <-requestCtx.Done():
				log.Info().Msgf("subscription member: closing (client stop listening) %v", id)

				c.closed.Store(true)
				close(c.listenChan)

				return
			case <-time.After(listenTimeOut):
				if c.listened.Load() {
					continue
				}

				log.Info().Msgf("subscription member: listen timed out %v", id)

				c.closed.Store(true)
				close(c.listenChan)

				return
			case msg := <-c.listenChan:
				if filterOutFn(msg) {
					continue
				}
				c.ch <- msg
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
	return c.ch
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

	c.listenChan <- msg

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
