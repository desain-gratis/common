package impl

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	notifierapi "github.com/desain-gratis/common/example/message-broker/src/log-api"
	"github.com/rs/zerolog/log"
)

var _ notifierapi.Subscription = &subscription{}

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
	async       bool
	listen      bool
	timeout     bool
	stop        func()
	ctx         context.Context
	listenCtx   context.Context
	listenChan  chan any
}

func NewSubscription(ctx, serverCtx context.Context, id string, async bool, bufferSize uint32, exitMessage any, listenTimeout time.Duration, stop func()) *subscription {
	// add go routine to Close this subscription
	// if it's not listened up immediately after certain time (eg. 2 seconds)
	c := &subscription{
		async:       async,
		exitMessage: exitMessage,
		ch:          make(chan any),
		id:          id,
		ctx:         ctx,
		listenChan:  make(chan any),
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
			case <-serverCtx.Done(): // app close
				log.Info().Msgf("subscription member: closing (server stop) %v", id)
				if !checkNilInterface(c.exitMessage) {
					wg.Add(1)
					go func() {
						defer wg.Done()
						c.ch <- c.exitMessage
					}()
				}
			case <-ctx.Done(): // client close
				log.Info().Msgf("subscription member: closing (stop listening for publish) %v", id)
				c.closed = true
				close(c.listenChan)
				return
			case msg := <-c.listenChan: // published message
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

func (c *subscription) IsListening() bool {
	return c.listen
}

func (c *subscription) Listen(ctx context.Context) <-chan any {
	c.listenCtx = ctx
	c.listen = true

	return c.ch
}

func (c *subscription) Publish(msg any) error {
	if c.closed {
		return ErrListenerClosed
	}

	if !c.started {
		return ErrNotStarted
	}

	if c.timeout && !c.listen {
		return ErrListenTimedOut
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
