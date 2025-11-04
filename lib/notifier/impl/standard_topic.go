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

var _ notifier.Topic = &standardTopic{}

// topic
type standardTopic struct {
	listener map[uint64]notifier.Subscription
	lock     *sync.RWMutex
}

var (
	ErrNotFound   = errors.New("not found")
	ErrInvalidKey = errors.New("invalid key")
)

// NewTopic create a new topic with create subscription function
func NewTopic() *standardTopic {
	return &standardTopic{
		listener: make(map[uint64]notifier.Subscription),
		lock:     &sync.RWMutex{},
	}
}

func getKey(uid uint64) string {
	idstr := strconv.FormatUint(uid, 10)
	return idstr
}

// TODO: maybe add appCtx as well
func (s *standardTopic) Subscribe(ctx context.Context, csf notifier.CreateSubscription) (notifier.Subscription, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	id := rand.Uint64() // change id impl to avoid conflict if needed

	subs := csf(ctx, getKey(id))

	log.Info().Msgf("topic: created new %v", id)

	s.lock.Lock()
	s.listener[id] = subs
	s.lock.Unlock()

	// unregister once the ctx has done
	go func(id uint64) {
		_ = <-ctx.Done()
		s.lock.Lock()
		defer s.lock.Unlock()
		delete(s.listener, id)
		log.Info().Msgf("topic: closed properly %v", id)
	}(id)

	return s.listener[id], nil
}

func (s *standardTopic) GetSubscription(id string) (notifier.Subscription, error) {
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

func (s *standardTopic) RemoveSubscription(id string) error {
	iduint, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return ErrInvalidKey
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.listener, iduint)

	return nil
}

func (s *standardTopic) Broadcast(ctx context.Context, message any) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for key, listener := range s.listener {
		err := listener.Publish(message)
		if err != nil && !errors.Is(err, ErrNotStarted) {
			log.Err(err).Msgf("error during publish.. I delete: %v", key)
		}
	}

	return nil
}

// Metric to support metrics query
func (s *standardTopic) Metric() any {
	var subscriberCount int
	func() {
		s.lock.RLock()
		defer s.lock.RUnlock()
		subscriberCount = len(s.listener)
	}()

	return map[string]any{
		"n_subscription": subscriberCount,
		"type":           "standard_topic",
	}
}
