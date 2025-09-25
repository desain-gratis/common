package smregistry

import (
	"encoding/json"

	"github.com/lni/dragonboat/v4"
	"github.com/lni/dragonboat/v4/statemachine"
	"github.com/rs/zerolog/log"
)

type SMConfigTyped[T any] struct {
	ID        uint64
	Name      string
	ShardID   uint64
	ReplicaID uint64
}

func Register[T any](r *dragonboatRegistry, fsmType string, fn StateMachineFunction[T]) {
	r.registry[fsmType] = func(dhost *dragonboat.NodeHost, smConfig SMConfig2, appConfig any) statemachine.CreateStateMachineFunc {
		cfg, ok := appConfig.(T)
		if !ok {
			log.Panic().Msgf("panic")
		}

		return fn(dhost, smConfig, cfg)
	}

	r.cfgParser[fsmType] = func(rm json.RawMessage) (any, error) {
		var t T

		if rm == nil {
			x := new(T)
			return *x, nil
		}

		err := json.Unmarshal(rm, &t)
		if err != nil {
			return nil, err
		}

		return t, nil
	}
}
