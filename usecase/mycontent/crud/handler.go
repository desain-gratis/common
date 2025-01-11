package crud

import (
	"context"
	"net/http"
	"time"

	"github.com/desain-gratis/common/repository/content"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

var _ mycontent.Usecase[any] = &crud[any]{}

// URLFormat for custom URL (this should be the URL default)
type URLFormat func(dataPath string, userID string, refID []string, ID string) string

type crud[T any] struct {
	repo content.Repository[T]

	// Modifier to the underlying data
	wrap     func(T) mycontent.Data
	validate func(T) *types.CommonError

	urlFormat URLFormat
}

func New[T any](
	repo content.Repository[T],
	wrap func(T) mycontent.Data,
	validate func(T) *types.CommonError,
	urlFormat URLFormat,
) *crud[T] {
	return &crud[T]{
		repo:      repo,
		wrap:      wrap,
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

	wrap := c.wrap(data)
	if wrap.OwnerID() == "" {
		return t, &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "MISSING_OWNER_ID_IN_DATA", Message: "Please specify content owner ID"},
			},
		}
	}

	refIDs := wrap.RefIDs()
	if len(refIDs) > 0 {
		// make sure parent ID is indexed as last entry of the index
		if refIDs[len(refIDs)-1] != wrap.ParentID() {
			refIDs = append(refIDs, wrap.ParentID())
		}
	}

	result, err := c.repo.Put(ctx, wrap.OwnerID(), "", []string{}, content.Data[T]{
		ID:         wrap.ID(),
		Data:       data,
		LastUpdate: time.Now(),
		UserID:     wrap.OwnerID(),
		RefIDs:     refIDs, // allow to be queried by this indices
	})
	if err != nil {
		return result.Data, err
	}

	c.wrap(result.Data).
		WithID(result.ID)

	wrap = c.wrap(result.Data)

	if c.urlFormat != nil {
		c.wrap(result.Data).WithURL(c.urlFormat(wrap.URL(), wrap.OwnerID(), wrap.RefIDs(), wrap.ID()))
	}

	return result.Data, nil
}

// Get all of your resource for your user ID here
// Simple wrapper for repository
func (c *crud[T]) Get(ctx context.Context, userID string, refIDs []string, ID string) ([]T, *types.CommonError) {
	// 1. check if there is ID
	if ID != "" {
		result := make([]T, 0, 1)

		d, err := c.repo.GetByID(ctx, userID, ID)
		if err != nil {
			return nil, err
		}
		c.wrap(d.Data).
			WithID(d.ID)

		wrap := c.wrap(d.Data)
		if c.urlFormat != nil {
			c.wrap(d.Data).WithURL(c.urlFormat(wrap.URL(), wrap.OwnerID(), wrap.RefIDs(), wrap.ID()))
		}

		result = append(result, d.Data)
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
			c.wrap(d.Data).
				WithID(d.ID)

			wrap := c.wrap(d.Data)
			if c.urlFormat != nil {
				c.wrap(d.Data).WithURL(c.urlFormat(wrap.URL(), wrap.OwnerID(), wrap.RefIDs(), wrap.ID()))
			}

			result = append(result, d.Data)
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
		c.wrap(d.Data).
			WithID(d.ID)

		wrap := c.wrap(d.Data)
		if c.urlFormat != nil {
			c.wrap(d.Data).WithURL(c.urlFormat(wrap.URL(), wrap.OwnerID(), wrap.RefIDs(), wrap.ID()))
		}

		result = append(result, d.Data)
	}

	return result, nil
}

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (c *crud[T]) Delete(ctx context.Context, userID string, ID string) (T, *types.CommonError) {

	// TODO user ID validation

	d, err := c.repo.Delete(ctx, userID, ID, []string{})
	if err != nil {
		return d.Data, err
	}

	c.wrap(d.Data).
		WithID(d.ID)

	wrap := c.wrap(d.Data)
	if c.urlFormat != nil {
		c.wrap(d.Data).WithURL(c.urlFormat(wrap.URL(), wrap.OwnerID(), wrap.RefIDs(), wrap.ID()))
	}

	return d.Data, nil
}
