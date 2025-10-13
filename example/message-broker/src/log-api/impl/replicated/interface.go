package replicated

import "encoding/json"

type QuerySubscribe struct{}

type QueryChatFromOffset struct {
	// Offset represent the current chat offset; you get it by subscribing
	Offset uint64
}

type Command string
type EventName string

// UpdateRequest is a json serializable object to post request to the state machine
type UpdateRequest struct {
	CmdName Command         `json:"cmd_name"`
	CmdVer  uint64          `json:"cmd_version"`
	Data    json.RawMessage `json:"data"`
}

type UpdateResponse struct {
	Error   error  `json:"error,omitempty"`
	Message string `json:"message"`
}

type AddSubscriptionResponse struct {
	Error          error  `json:"error,omitempty"`
	SubscriptionID uint64 `json:"subscription_id"`
}

// Event represent serializable events emanating from the state machine
// published in the topic
type Event struct {
	EvtID   uint64    `json:"evt_id"` // offset
	EvtName EventName `json:"evt_name"`
	EvtVer  uint64    `json:"evt_version"`
	Data    any       `json:"data"`
}

type EventStartListener struct {
	LastAppliedIdx uint64 `json:"last_applied_idx"`
}

const (
	// Command_UpdateLeader a command to register a subscription
	Command_UpdateLeader Command = "update-leader"

	// Command_StartSubscription adds subscription to topic to listen to
	Command_StartSubscription Command = "start-subscription"

	// Command_PublishMessage publishes to the topic
	Command_PublishMessage Command = "publish-message"

	Command_NotifyOnline  Command = "notify-online"
	Command_NotifyOffline Command = "notify-offline"

	EventName_Echo          EventName = "echo"
	EventName_Identity      EventName = "identity"
	EventName_NotifyOnline  EventName = "notify-online"
	EventName_NotifyOffline EventName = "notify-offline"
)

type StartSubscriptionData struct {
	SubscriptionID string `json:"subscription_id"`
	ReplicaID      uint64 `json:"replica_id"`
}
