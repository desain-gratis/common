package base

import (
	"context"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	"github.com/rs/zerolog/log"
)

var _ mycontent.Usecase[mycontent.VersionedData] = &VersionedHandler[mycontent.VersionedData]{}

type VersionedHandler[T mycontent.VersionedData] struct {
	*Handler[T] // extends
}

func NewVersioned[T mycontent.VersionedData](
	repo content.Repository,
	expectedRefSize int,
) *VersionedHandler[T] {
	return &VersionedHandler[T]{
		Handler: &Handler[T]{
			repo:            repo,
			expectedRefSize: expectedRefSize,
		},
	}
}

// Extend the Get
func (c *VersionedHandler[T]) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]T, error) {
	ds, err := c.get(ctx, namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	result := make([]T, 0, len(ds))
	for _, d := range ds {
		parsedResult, err := Parse[T](d.Data)
		if err != nil {
			log.Error().Msgf("Should not happend")
			continue
		}

		// parsedResult.WithVersion(d.Version)
		parsedResult.WithEventID(d.EventID)

		result = append(result, parsedResult)
	}

	return result, nil
}
