package impl

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"sync"

	notifierapi "github.com/desain-gratis/common/delivery/log-api"
	"github.com/rs/zerolog/log"
)

var _ notifierapi.Topic = &topic{}

// topic
type topic struct {
	listener map[uint64]notifierapi.Subscription
	lock     *sync.RWMutex
	csf      CreateSubscription
}

var (
	ErrNotFound   = errors.New("not found")
	ErrInvalidKey = errors.New("invalid key")
)

type CreateSubscription func(id string) notifierapi.Subscription

// NewTopic create a new topic with create subscription function
func NewTopic(csf CreateSubscription) notifierapi.Topic {
	return &topic{
		listener: make(map[uint64]notifierapi.Subscription),
		csf:      csf,
		lock:     &sync.RWMutex{},
	}
}

func getKey(uid uint64) string {
	idstr := strconv.FormatUint(uid, 10)
	return idstr
}

func (s *topic) Subscribe() (notifierapi.Subscription, error) {
	id := rand.Uint64() // change id impl to avoid conflict if needed

	subs := s.csf(getKey(id))

	s.lock.Lock()
	s.listener[id] = subs
	s.lock.Unlock()

	// todo: add timeout if there is no active listener to close the listener.

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
		wg.Add(1)
		go func(k uint64, l notifierapi.Subscription) {
			defer wg.Done()
			err := listener.Publish(ctx, message)
			if err != nil {
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
