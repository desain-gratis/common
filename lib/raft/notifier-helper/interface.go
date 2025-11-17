package notifierhelper

type StartSubscriptionRequest struct {
	SubscriptionID string `json:"subscription_id"`
	ReplicaID      uint64 `json:"replica_id"`
	Topic          string `json:"topic"`
	Debug          string `json:"debug"`
}
