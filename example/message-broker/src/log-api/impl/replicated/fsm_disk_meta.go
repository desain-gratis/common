package replicated

import (
	"encoding/json"

	"github.com/rs/zerolog/log"
)

type Metadata struct {
	// AppliedIndex is the raft applied index
	AppliedIndex *uint64 `json:"applied_index,omitempty"`

	// ChatLog_EvtIndex_Counter serial counter
	ChatLog_EvtIndex_Counter *uint64 `json:"chat_log__evt_index__counter,omitempty"`
}

func deserializeMetadata(payload []byte) (*Metadata, error) {
	var metadata Metadata

	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &metadata); err != nil {
			return nil, err
		}
	}

	// initialize default value.
	// mostly uses reference for easier modifications

	if metadata.AppliedIndex == nil {
		var appliedIndex uint64
		metadata.AppliedIndex = &appliedIndex
	}

	if metadata.ChatLog_EvtIndex_Counter == nil {
		var counter uint64
		metadata.ChatLog_EvtIndex_Counter = &counter
	}

	log.Info().Msgf("METADATA %+v", *metadata.AppliedIndex)

	return &metadata, nil
}

func serializeMetadata(metadata *Metadata) ([]byte, error) {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	return payload, nil
}
