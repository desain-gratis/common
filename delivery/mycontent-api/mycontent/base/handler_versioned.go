package base

import (
	"context"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	"github.com/rs/zerolog/log"
)

var _ mycontent.VersionedUsecase[mycontent.Data] = &VersionedHandler[mycontent.Data]{}

type VersionedHandler[T mycontent.Data] struct {
	Handler[T]
	repo content.VersionedRepository
}

func NewVersioned[T mycontent.Data](
	repo content.VersionedRepository,
) *VersionedHandler[T] {
	// TODO: add validation
	return &VersionedHandler[T]{
		Handler: Handler[T]{repo: repo},
		repo:    repo,
	}
}

func (c *VersionedHandler[T]) GetByVersion(ctx context.Context, namespace string, refIDs []string, ID string, version uint64) (T, error) {
	var t T
	d, err := c.repo.GetByVersion(ctx, namespace, refIDs, ID, version)
	if err != nil {
		return t, err
	}

	parsedResult, err := Parse[T](d.Data)
	if err != nil {
		log.Error().Msgf("Should not happend")
		return t, err
	}

	// repository responsble to specify it inside their ID
	parsedResult.WithID(d.ID)
	if ver := d.Version; ver != nil && *ver > 0 {
		parsedResult.WithVersion(*ver)
	}

	return parsedResult, nil
}

func (c *VersionedHandler[T]) GetAllVersion(ctx context.Context, namespace string, refIDs []string, ID string) ([]T, error) {
	ds, err := c.repo.GetAllVersion(ctx, namespace, refIDs, ID)
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

		// repository responsble to specify it inside their ID
		parsedResult.WithID(d.ID)
		if ver := d.Version; ver != nil && *ver > 0 {
			parsedResult.WithVersion(*ver)
		}

		result = append(result, parsedResult)
	}

	return result, nil
}
