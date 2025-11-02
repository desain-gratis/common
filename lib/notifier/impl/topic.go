package impl

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/lib/notifier"
)

var _ notifier.Topic = &topic{}

// topic
type topic struct {
	listener map[uint64]notifier.Subscription
	lock     *sync.RWMutex
	Csf      CreateSubscription
}

var (
	ErrNotFound   = errors.New("not found")
	ErrInvalidKey = errors.New("invalid key")
)

type CreateSubscription func(ctx context.Context, id string) notifier.Subscription

var _ notifier.Topic = &topic{}

// NewTopic create a new topic with create subscription function
func NewTopic() *topic {
	return &topic{
		listener: make(map[uint64]notifier.Subscription),
		lock:     &sync.RWMutex{},
	}
}

func getKey(uid uint64) string {
	idstr := strconv.FormatUint(uid, 10)
	return idstr
}

// TODO: maybe add appCtx as well
func (s *topic) Subscribe(ctx context.Context) (notifier.Subscription, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	id := rand.Uint64() // change id impl to avoid conflict if needed

	subs := s.Csf(ctx, getKey(id))

	log.Info().Msgf("topic: created new %v", id)

	s.lock.Lock()
	s.listener[id] = subs
	s.lock.Unlock()

	go func(id uint64) {
		_ = <-ctx.Done()

		s.lock.Lock()
		defer s.lock.Unlock()
		delete(s.listener, id)
		log.Info().Msgf("topic: closed properly %v", id)
	}(id)

	return s.listener[id], nil
}

func (s *topic) GetSubscription(id string) (notifier.Subscription, error) {
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

	func() {
		s.lock.RLock()
		defer s.lock.RUnlock()
		for key, listener := range s.listener {
			err := listener.Publish(message)
			// todo: refactor here
			if err != nil && !errors.Is(err, ErrNotStarted) {
				log.Err(err).Msgf("error during publish.. I delete: %v", key)
				delKey = append(delKey, key)
			}
		}
	}()

	s.lock.Lock()
	defer s.lock.Unlock()

	for _, key := range delKey {
		log.Info().Msgf("deletion: %v", key)
		delete(s.listener, key)
	}

	return nil
}
