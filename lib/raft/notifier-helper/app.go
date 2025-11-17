package notifierhelper

import (
	"errors"

	"github.com/desain-gratis/common/lib/notifier"
	"github.com/desain-gratis/common/lib/notifier/impl"
	"github.com/rs/zerolog/log"
)

const (
	appName = "base_app"
)

var (
	ErrDifferentReplica = errors.New("different replica")
	ErrTopicNotFound    = errors.New("topic not found")
)

type TopicRegistry map[string]notifier.Topic

func NewTopicRegistry(topic map[string]notifier.Topic) TopicRegistry {
	return TopicRegistry(topic)
}

// StartSubscription should be put inside raft.Application OnUpdate method
func (n TopicRegistry) StartSubscription(currentReplica, currentOffset uint64, request StartSubscriptionRequest) error {
	if request.ReplicaID != currentReplica {
		log.Debug().Msgf("not the requested replica requested: %v got %v", request.ReplicaID, currentReplica)
		return nil
	}

	topic, ok := n[request.Topic]
	if !ok {
		return ErrTopicNotFound
	}

	subs, err := topic.GetSubscription(request.SubscriptionID)
	if err != nil {
		if !errors.Is(err, impl.ErrNotFound) {
			log.Err(err).Msgf("err get subs %v: %v %v", err, request.SubscriptionID, request)
		}
		return nil
	}

	log.Info().Msgf("starting local subscriber: %v %v", subs.ID(), request)
	subs.Start()

	return nil
}
