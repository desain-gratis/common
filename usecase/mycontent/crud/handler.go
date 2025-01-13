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

// URLFormat for custom URL (this should be the URL default)
type URLFormat func(dataPath string, userID string, refID []string, ID string) string

type crud[T mycontent.Data] struct {
	repo      content.Repository
	validate  func(T) *types.CommonError
	urlFormat URLFormat
}

func New[T mycontent.Data](
	repo content.Repository,
	validate func(T) *types.CommonError,
	urlFormat URLFormat,
) *crud[T] {
	return &crud[T]{
		repo:      repo,
		validate:  validate,
		urlFormat: urlFormat,
	}
}

// Put (create new or overwrite) resource here
func (c *crud[T]) Put(ctx context.Context, data T) (T, *types.CommonError) {
	var t T
	err := c.validate(data)
	if err != nil {
		return t, err
	}

	if data.OwnerID() == "" {
		return t, &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "MISSING_OWNER_ID_IN_DATA", Message: "Please specify content owner ID"},
			},
		}
	}

	refIDs := data.RefIDs()
	if len(refIDs) > 0 {
		// make sure parent ID is indexed as last entry of the index
		if refIDs[len(refIDs)-1] != data.ParentID() {
			refIDs = append(refIDs, data.ParentID())
		}
	}

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

	result, err := c.repo.Post(ctx, data.OwnerID(), data.ID(), refIDs, content.Data{
		ID:         data.ID(), // might be redundant
		Data:       payload,
		LastUpdate: time.Now(),
		RefIDs:     refIDs, // allow to be queried by this indices
		UserID:     data.OwnerID(),
	})
	if err != nil {
		return t, err
	}

	parsedResult, err := Parse[T](result.Data)
	if err != nil {
		return t, err
	}

	parsedResult.WithID(result.ID)

	if c.urlFormat != nil {
		parsedResult.WithURL(
			c.urlFormat(
				parsedResult.URL(),
				parsedResult.OwnerID(),
				parsedResult.RefIDs(),
				parsedResult.ID(),
			),
		)
	}

	return parsedResult, nil
}

// Get all of your resource for your user ID here
// Simple wrapper for repository
func (c *crud[T]) Get(ctx context.Context, userID string, refIDs []string, ID string) ([]T, *types.CommonError) {
	// 1. check if there is ID
	if ID != "" {
		result := make([]T, 0, 1)

		d, err := c.repo.Get(ctx, userID, ID, refIDs)
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

		parsedResult.
			WithID(d[0].ID) // should be already fine without this..

		if c.urlFormat != nil {
			parsedResult.WithURL(c.urlFormat(parsedResult.URL(), parsedResult.OwnerID(), parsedResult.RefIDs(), parsedResult.ID()))
		}

		result = append(result, parsedResult)
		return result, nil
	}

	// 2. check if there is main ref ID
	if len(refIDs) > 0 {
		ds, err := c.repo.Get(ctx, userID, "", refIDs)
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

			parsedResult.
				WithID(d.ID)

			if c.urlFormat != nil {
				parsedResult.WithURL(c.urlFormat(parsedResult.URL(), parsedResult.OwnerID(), parsedResult.RefIDs(), parsedResult.ID()))
			}

			result = append(result, parsedResult)
		}
		return result, nil
	}

	// 3. get by user ID
	ds, err := c.repo.Get(ctx, userID, "", []string{})
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

		parsedResult.
			WithID(d.ID)

		if c.urlFormat != nil {
			parsedResult.WithURL(c.urlFormat(parsedResult.URL(), parsedResult.OwnerID(), parsedResult.RefIDs(), parsedResult.ID()))
		}

		result = append(result, parsedResult)
	}

	return result, nil
}

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (c *crud[T]) Delete(ctx context.Context, userID string, ID string) (t T, err *types.CommonError) {

	// TODO user ID validation
	d, err := c.repo.Delete(ctx, userID, ID, []string{})
	if err != nil {
		var t T
		return t, err
	}

	parsedResult, err := Parse[T](d.Data)
	if err != nil {
		return t, err
	}

	parsedResult.
		WithID(d.ID)

	if c.urlFormat != nil {
		parsedResult.WithURL(c.urlFormat(parsedResult.URL(), parsedResult.OwnerID(), parsedResult.RefIDs(), parsedResult.ID()))
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
