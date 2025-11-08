package statemachine

import (
	"encoding/json"
)

type Metadata struct {
	// AppliedIndex is the raft applied index
	AppliedIndex *uint64 `json:"applied_index,omitempty"`
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

	return &metadata, nil
}

func serializeMetadata(metadata *Metadata) ([]byte, error) {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	return payload, nil
}
