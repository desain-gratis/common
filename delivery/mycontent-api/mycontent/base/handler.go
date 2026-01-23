package base

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

var _ mycontent.Usecase[mycontent.Data] = &Handler[mycontent.Data]{}

type Handler[T mycontent.Data] struct {
	repo            content.Repository
	expectedRefSize int // TODO: change to just size (eg. expected refIDs size)
}

func New[T mycontent.Data](
	repo content.Repository,
	expectedRefSize int,
) *Handler[T] {
	return &Handler[T]{
		repo:            repo,
		expectedRefSize: expectedRefSize,
	}
}

// Post (create new or overwrite) resource here
func (c *Handler[T]) Post(ctx context.Context, data T, meta any) (T, error) {
	var t T
	err := data.Validate()
	if err != nil {
		return t, err
	}

	if data.Namespace() == "" {
		return t, fmt.Errorf("%w: namespace cannot be empty", mycontent.ErrValidation)
	}

	if !isValid(data.RefIDs()) && len(filterEmpty(data.RefIDs())) != c.expectedRefSize {
		return t, fmt.Errorf("%w: complete reference must be provided during post", mycontent.ErrValidation)
	}

	// delivery --- up to here -----

	// if create new and no id, assign new id
	id := data.ID()
	if id == "" {
		_id := uuid.New()
		id = _id.String()
	}
	data.WithID(id)

	// created that empty, assign new created date
	date := data.CreatedTime()
	if date.Equal(time.Time{}) {
		data.WithCreatedTime(time.Now())
	}

	payload, errMarshal := json.Marshal(data)
	if errMarshal != nil {
		return t, err
	}

	metaPayload := []byte("{}")
	if meta != nil {
		metaPayload, errMarshal = json.Marshal(meta)
		if errMarshal != nil {
			return t, err
		}
	}

	result, err := c.repo.Post(ctx, data.Namespace(), data.RefIDs(), data.ID(), content.Data{
		Data: payload,
		Meta: metaPayload,
	})
	if err != nil {
		return t, fmt.Errorf("%w: %w during data storage", mycontent.ErrStorage, err)
	}

	parsedResult, err := Parse[T](result.Data)
	if err != nil {
		return t, err
	}

	return parsedResult, nil
}

// Get all of your resource for your user ID here
// Simple wrapper for repository
func (c *Handler[T]) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]T, error) {
	// 1. check if there is ID
	if ID != "" {
		if !isValid(refIDs) || len(filterEmpty(refIDs)) != c.expectedRefSize {
			result := make([]T, 0, 1)
			return result, fmt.Errorf(
				"%w: when ID is specified, all reference must be specified", mycontent.ErrValidation)
		}

		result := make([]T, 0, 1)

		d, err := c.repo.Get(ctx, namespace, refIDs, ID)
		if err != nil {
			return nil, err
		}

		if len(d) == 0 {
			return result, fmt.Errorf(
				"%w: id specified, but content not found", mycontent.ErrNotFound)
		}

		parsedResult, err := Parse[T](d[0].Data)
		if err != nil {
			return nil, err
		}

		result = append(result, parsedResult)
		return result, nil
	}

	// 2. check if there is main ref ID (without ID)
	if isValid(refIDs) {
		ds, err := c.repo.Get(ctx, namespace, filterEmpty(refIDs), "")
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

			result = append(result, parsedResult)
		}
		return result, nil
	}

	// 3. get by namespace
	ds, err := c.repo.Get(ctx, namespace, []string{}, "")
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

		result = append(result, parsedResult)
	}

	return result, nil
}

func (c *Handler[T]) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan T, error) {
	// 1. check if there is ID
	if ID != "" {
		if !isValid(refIDs) || len(filterEmpty(refIDs)) != c.expectedRefSize {
			return nil, fmt.Errorf(
				"%w: when ID is specified, all reference must be specified", mycontent.ErrValidation)
		}

		d, err := c.repo.Get(ctx, namespace, refIDs, ID)
		if err != nil {
			return nil, err
		}

		if len(d) == 0 {
			return nil, fmt.Errorf(
				"%w: id specified, but content not found", mycontent.ErrNotFound)
		}

		parsedResult, err := Parse[T](d[0].Data)
		if err != nil {
			return nil, err
		}

		result := make(chan T)

		go func() {
			defer close(result)
			result <- parsedResult
		}()

		return result, nil
	}

	// 2. check if there is main ref ID (without ID)
	if isValid(refIDs) {
		ds, err := c.repo.Stream(ctx, namespace, filterEmpty(refIDs), "")
		if err != nil {
			return nil, err
		}

		result := make(chan T)
		go func() {
			defer close(result)

			for d := range ds {
				parsedResult, err := Parse[T](d.Data)
				if err != nil {
					log.Error().Msgf("Should not happend")
					continue
				}
				result <- parsedResult
			}
		}()
		return result, nil
	}

	// 3. get by namespace
	ds, err := c.repo.Stream(ctx, namespace, []string{}, "")
	if err != nil {
		return nil, err
	}

	result := make(chan T)
	go func() {
		defer close(result)

		for d := range ds {
			parsedResult, err := Parse[T](d.Data)
			if err != nil {
				log.Error().Msgf("Should not happend")
				continue
			}
			result <- parsedResult
		}
	}()

	return result, nil
}

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (c *Handler[T]) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (t T, err error) {
	if !isValid(refIDs) && len(filterEmpty(refIDs)) != c.expectedRefSize {
		return t, fmt.Errorf("%w: complete reference must be provided during delete", mycontent.ErrValidation)
	}

	if namespace == "" || namespace == "*" {
		return t, fmt.Errorf("%w: namespace cannot be empty or '*' during delete", mycontent.ErrValidation)
	}

	// TODO user ID validation
	d, err := c.repo.Delete(ctx, namespace, refIDs, ID)
	if err != nil {
		var t T
		return t, fmt.Errorf("repository error: %w", err)
	}

	parsedResult, err := Parse[T](d.Data)
	if err != nil {
		return t, err
	}

	return parsedResult, nil
}

func Parse[T any](in []byte) (T, error) {
	var t T
	err := json.Unmarshal(in, &t)
	if err != nil {
		return t, err
	}
	return t, nil
}

func filterEmpty(arr []string) []string {
	result := make([]string, 0, len(arr))
	for _, ar := range arr {
		if ar != "" {
			result = append(result, ar)
		}
	}
	return result
}

func isValid(arr []string) bool {
	var notEmptyFound bool
	for i := len(arr) - 1; i >= 0; i-- {
		v := arr[i]
		if v == "" {
			if !notEmptyFound {
				continue
			}
			return false
		}
		notEmptyFound = true
	}
	return true
}
