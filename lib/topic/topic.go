package topic

func init() {
	// initialize raft
}

type Subscription interface {
}

type Topic interface {
	Publish(data []byte) ([]byte, error)
	PublishAsnyc(data []byte) error
	Subscribe() Subscription
	Query()
}

var topicMap map[string]Topic

func Get(topic string) Topic {
	return topicMap[topic]
}
