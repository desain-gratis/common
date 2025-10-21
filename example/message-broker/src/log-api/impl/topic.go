package impl

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	notifierapi "github.com/desain-gratis/common/example/message-broker/src/log-api"
	"github.com/rs/zerolog/log"
)

var _ notifierapi.Topic = &topic{}

// topic
type topic struct {
	listener map[uint64]notifierapi.Subscription
	lock     *sync.RWMutex
	Csf      CreateSubscription
}

var (
	ErrNotFound   = errors.New("not found")
	ErrInvalidKey = errors.New("invalid key")
)

type CreateSubscription func(id string) notifierapi.Subscription

var _ notifierapi.Topic = &topic{}

// NewTopic create a new topic with create subscription function
func NewTopic() *topic {
	return &topic{
		listener: make(map[uint64]notifierapi.Subscription),
		lock:     &sync.RWMutex{},
	}
}

func getKey(uid uint64) string {
	idstr := strconv.FormatUint(uid, 10)
	return idstr
}

func (s *topic) Subscribe() (notifierapi.Subscription, error) {
	id := rand.Uint64() // change id impl to avoid conflict if needed

	subs := s.Csf(getKey(id))

	s.lock.Lock()
	s.listener[id] = subs
	s.lock.Unlock()

	go func(id uint64) {
		time.Sleep(4 * time.Second)
		if subs.IsListening() {
			return
		}

		log.Error().Msgf("listen timed out")
		s.lock.Lock()
		defer s.lock.Unlock()
		delete(s.listener, id)
	}(id)

	return s.listener[id], nil
}

func (s *topic) GetSubscription(id string) (notifierapi.Subscription, error) {
	iduint, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return nil, ErrInvalidKey
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	l, ok := s.listener[iduint]
	if !ok {
		return nil, ErrNotFound
	}

	return l, nil
}

func (s *topic) RemoveSubscription(id string) error {
	iduint, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return ErrInvalidKey
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.listener, iduint)

	return nil
}

func (s *topic) Broadcast(ctx context.Context, message any) error {
	delKey := make([]uint64, 0)

	wg := new(sync.WaitGroup)
	for key, listener := range s.listener {
		// if !listener.IsListening() {
		// 	continue
		// }
		log.Info().Msgf("listener: %v gets it.", key)

		wg.Add(1)
		go func(k uint64, l notifierapi.Subscription) {
			defer wg.Done()
			err := listener.Publish(ctx, message)
			// todo: refactor here
			if err != nil && !errors.Is(err, ErrNotStarted) {
				log.Err(err).Msgf("error during publish.. I delete: %v", key)
				delKey = append(delKey, key)
			}
		}(key, listener)
	}
	wg.Wait()

	s.lock.Lock()
	defer s.lock.Unlock()

	for _, key := range delKey {
		log.Info().Msgf("deletion: %v", key)
		delete(s.listener, key)
	}

	return nil
}
