package impl

import (
	"context"
	"math/rand/v2"
	"sync"

	notifierapi "github.com/desain-gratis/common/delivery/notifier-api"
	"github.com/rs/zerolog/log"
)

type notifier struct {
	listener    map[uint32]chan<- any
	lock        *sync.RWMutex
	exitMessage any
}

// NewSimpleNotifier is the basic implementation only works for publisher/listener
// in the same process. Can be used for testing or in single server setup
// todo: move to impl folder
func NewSimpleNotifier(exitMessage any) notifierapi.Notifier {
	return &notifier{
		listener:    make(map[uint32]chan<- any),
		lock:        &sync.RWMutex{},
		exitMessage: exitMessage,
	}
}

// Publish or "broadcast"
func (c *notifier) Publish(ctx context.Context, message any) error {
	for _, listener := range c.listener {
		go func(v any) {
			listener <- v
		}(message)
	}

	return nil
}

func (c *notifier) Listen(ctx context.Context) <-chan any {
	id := rand.Uint32()
	subscribeChan := make(chan any)
	go func(id uint32) {
		defer func() {
			c.lock.Lock()
			defer c.lock.Unlock()
			delete(c.listener, id)
			close(subscribeChan)
			log.Debug().Msgf("  DELETED listener: %v", id)
		}()
		<-ctx.Done()

		if c.exitMessage != nil {
			subscribeChan <- c.exitMessage
		}
	}(id)

	c.lock.Lock()
	defer c.lock.Unlock()

	// TODO: improve data structure (this can conflict)
	c.listener[id] = subscribeChan
	log.Debug().Msgf("NEW listener: %v", id)

	return subscribeChan
}
