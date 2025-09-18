package dragonboat

import "encoding/json"

type Query string
type Evt string

const (
	Query_Subscribe Query = "subscribe"
)

type Command struct {
	CmdName string          `json:"cmd_name"`
	CmdVer  uint64          `json:"cmd_version"`
	Data    json.RawMessage `json:"data"`
}

type Event struct {
	EvtName string          `json:"evt_name"`
	EvtVer  uint64          `json:"evt_version"`
	EvtID   uint64          `json:"evt_id"` // offset
	Data    json.RawMessage `json:"data"`
}
