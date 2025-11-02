package impl

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/lib/notifier"
)

var _ notifier.Subscription = &subscription{}

var (
	ErrListenerClosed = errors.New("closed")
	ErrListenerEmpty  = errors.New("no listener")
	ErrNotStarted     = errors.New("not started")
	ErrListenTimedOut = errors.New("listen timed out")
)

// TODO: major refactor this
type subscription struct {
	id          string
	started     bool
	closed      bool
	ch          chan any
	exitMessage any
	ctx         context.Context
	listenCtx   context.Context
	listenChan  chan any
}

func NewSubscription(requestCtx, appCtx context.Context, id string, exitMessage any, filterOutFn func(any) bool) *subscription {
	// add go routine to Close this subscription
	// if it's not listened up immediately after certain time (eg. 2 seconds)
	c := &subscription{
		id:          id,
		ctx:         requestCtx,
		exitMessage: exitMessage,
		ch:          make(chan any),
		listenChan:  make(chan any),
	}

	if filterOutFn == nil {
		filterOutFn = func(a any) bool {
			return false
		}
	}

	log.Info().Msgf("subscription member: created %v", id)

	go func() {
		wg := sync.WaitGroup{}
		defer func() {
			wg.Wait()
			close(c.ch)
			log.Info().Msgf("subscription member: closed properly %v", id)
		}()
		for {
			select {
			case <-appCtx.Done(): // app close
				log.Info().Msgf("subscription member: closing (server stop) %v", id)
				c.closed = true
				close(c.listenChan)
				if !checkNilInterface(c.exitMessage) {
					wg.Add(1)
					go func() {
						defer wg.Done()
						c.ch <- c.exitMessage
					}()
				}
			case <-requestCtx.Done(): // client close
				log.Info().Msgf("subscription member: closing (stop listening for publish) %v", id)
				c.closed = true
				close(c.listenChan)
				return
			case msg := <-c.listenChan:
				if filterOutFn(msg) {
					continue
				}
				// published message
				// definitely can queue up, to make sure no messages are lost. (and can join together by event ID)
				wg.Add(1)
				go func() {
					defer wg.Done()
					c.ch <- msg
				}()
			}
		}
	}()

	return c
}

func (c *subscription) ID() string {
	return c.id
}

func (c *subscription) Start() {
	c.started = true
}

func (c *subscription) Listen() <-chan any {
	return c.ch
}

func (c *subscription) Publish(msg any) error {
	if c.closed {
		return ErrListenerClosed
	}

	if !c.started {
		return ErrNotStarted
	}

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
