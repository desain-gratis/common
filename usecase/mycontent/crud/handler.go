package crud

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/desain-gratis/common/repository/content"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

var _ mycontent.Usecase[mycontent.Data] = &crud[mycontent.Data]{}

type crud[T mycontent.Data] struct {
	repo            content.Repository
	expectedRefSize int // TODO: change to just size (eg. expected refIDs size)
}

func New[T mycontent.Data](
	repo content.Repository,
	expectedRefSize int,
) *crud[T] {
	return &crud[T]{
		repo:            repo,
		expectedRefSize: expectedRefSize,
	}
}

// Post (create new or overwrite) resource here
func (c *crud[T]) Post(ctx context.Context, data T, meta any) (T, *types.CommonError) {
	var t T
	err := data.Validate()
	if err != nil {
		return t, err
	}

	// TODO: !!!! ALL VALIDATION TO MOVE TO DELIVERY..
	if data.Namespace() == "" {
		return t, &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "MISSING_NAMESPACE_IN_DATA", Message: "Please specify content owner ID"},
			},
		}
	}

	if !isValid(data.RefIDs()) && len(filterEmpty(data.RefIDs())) != c.expectedRefSize {
		return t, &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "INVALID_REF", Message: "Make sure all params are specified"},
			},
		}
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
	if date == (time.Time{}) {
		data.WithCreatedTime(time.Now())
	}

	payload, errMarshal := json.Marshal(data)
	if errMarshal != nil {
		return t, &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "JSON_ENCODE_FAILED", Message: "Failed marshal"},
			},
		}
	}

	var metaPayload []byte
	if meta != nil {
		metaPayload, errMarshal = json.Marshal(meta)
		if errMarshal != nil {
			return t, &types.CommonError{
				Errors: []types.Error{
					{HTTPCode: http.StatusBadRequest, Code: "JSON_ENCODE_FAILED", Message: "Failed marshal meta"},
				},
			}
		}
	}

	result, err := c.repo.Post(ctx, data.Namespace(), data.RefIDs(), data.ID(), content.Data{
		Data: payload,
		Meta: metaPayload,
	})
	if err != nil {
		return t, err
	}

	parsedResult, err := Parse[T](result.Data)
	if err != nil {
		return t, err
	}

	return parsedResult, nil
}

// Get all of your resource for your user ID here
// Simple wrapper for repository
func (c *crud[T]) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]T, *types.CommonError) {
	// 1. check if there is ID
	if ID != "" {
		if !isValid(refIDs) || len(filterEmpty(refIDs)) != c.expectedRefSize {
			result := make([]T, 0, 1)
			return result, &types.CommonError{
				Errors: []types.Error{
					{
						Code:     "NOT_FOUND",
						HTTPCode: http.StatusNotFound,
						Message:  "You specify item ID, but some refs are missing",
					},
				},
			}
		}

		result := make([]T, 0, 1)

		d, err := c.repo.Get(ctx, namespace, refIDs, ID)
		if err != nil {
			return nil, err
		}

		if len(d) == 0 {
			return result, &types.CommonError{
				Errors: []types.Error{
					{
						Code:     "NOT_FOUND",
						HTTPCode: http.StatusNotFound,
						Message:  "You specify item ID, but the specified ID is not found.",
					},
				},
			}
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

	// 3. get by user ID | TODO DELETE | redundant with above, because empty refIDs is valid as well..
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

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (c *crud[T]) Delete(ctx context.Context, userID string, refIDs []string, ID string) (t T, err *types.CommonError) {
	if !isValid(refIDs) && len(filterEmpty(refIDs)) != c.expectedRefSize {
		return t, &types.CommonError{
			Errors: []types.Error{
				{
					Code:     "NOT_FOUND",
					HTTPCode: http.StatusNotFound,
					Message:  "You specify item ID, but some refs are missing",
				},
			},
		}
	}

	// TODO user ID validation
	d, err := c.repo.Delete(ctx, userID, refIDs, ID)
	if err != nil {
		var t T
		return t, err
	}

	parsedResult, err := Parse[T](d.Data)
	if err != nil {
		return t, err
	}

	return parsedResult, nil
}

func Parse[T any](in []byte) (T, *types.CommonError) {
	var t T
	err := json.Unmarshal(in, &t)
	if err != nil {
		log.Err(err).Msgf("err Parse: '%v'", string(in))
		return t, &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "JSON_DECODE_FAILED", Message: "Failed unmarshal" + err.Error()},
			},
		}
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
