package crud

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/blob"
	"github.com/desain-gratis/common/repository/content"
	"github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

var _ mycontent.Usecase[*entity.Attachment] = &crudWithAttachment{}
var _ mycontent.Attachable[*entity.Attachment] = &crudWithAttachment{}

type crudWithAttachment struct {
	*crud[*entity.Attachment]
	blobRepo  blob.Repository
	hideUrl   bool // if data are quite sensitive
	namespace string
}

// NewWithAttachment creates the basic CRUD handle, but enables attachment
// Whether this is private or not will mostly be depended by the blob repository
func NewAttachment(
	repo content.Repository, // todo, change catalog.Attachment location to more common location (not uc specific)
	blobRepo blob.Repository,
	hideUrl bool,
	namespace string,
	expectedRefSize int,
) *crudWithAttachment {

	return &crudWithAttachment{
		crud: New[*entity.Attachment](
			repo,
			expectedRefSize,
		),
		blobRepo:  blobRepo,
		hideUrl:   hideUrl,
		namespace: namespace,
	}
}

// Overwrite for censoring
func (c *crudWithAttachment) Get(ctx context.Context, userID string, refIDs []string, ID string) ([]*entity.Attachment, *types.CommonError) {
	result, err := c.crud.Get(ctx, userID, refIDs, ID)
	if err != nil {
		return nil, err
	}
	if c.hideUrl {
		for _, d := range result {
			d.Url = ""
			d.Path = ""
		}
	}
	return result, nil
}

// BETA
func (c *crudWithAttachment) GetAttachment(ctx context.Context, userID string, refIDs []string, ID string) (payload io.ReadCloser, meta *entity.Attachment, err *types.CommonError) {
	result, err := c.crud.Get(ctx, userID, refIDs, ID)
	if err != nil {
		return nil, nil, err
	}
	if len(result) != 1 {
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusNotFound, Code: "NOT_FOUND", Message: "File not found"},
			},
		}
	}

	reader, _, err := c.blobRepo.Get(ctx, result[0].Path)
	if err != nil {
		return nil, nil, err
	}

	meta = result[0]

	return reader, meta, nil
}

// Put
// disables the default put behaviour
func (c *crudWithAttachment) Put(ctx context.Context, content *entity.Attachment) (*entity.Attachment, *types.CommonError) {
	return nil, &types.CommonError{
		Errors: []types.Error{
			{
				HTTPCode: http.StatusMethodNotAllowed,
				Code:     "NOT_SUPPORTED",
				Message:  "Put failed. Put are disabled for attachment API. Please use 'Upload' instead",
			},
		},
	}
}

func (c *crudWithAttachment) Attach(ctx context.Context, meta *entity.Attachment, payload io.Reader) (*entity.Attachment, *types.CommonError) {
	// TODO: Get all existing data based on user ID, calculate the total size to do validation

	// Check existing, if exist with the same ID, then use existing
	existing, errUC := c.crud.Get(ctx, meta.Namespace(), meta.RefIds, meta.Id)
	if errUC != nil {
		if errUC.Errors[0].HTTPCode != http.StatusNotFound {
			return nil, errUC
		}
	}

	var result *entity.Attachment
	for _, e := range existing {
		if e.Id == meta.Id {
			if e.CreatedAt == "" {
				log.Warn().Msgf("Existing DB have no created at information")
				continue
			}
			result = e
			break
		}
	}

	if meta.CreatedAt == "" {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "UPLOAD_FAILED",
					Message:  "Empty CreatedAt time",
				},
			},
		}
	}

	if result != nil {
		// Server overwritten properties (cannot be modified by user after creation)
		meta.Id = result.Id
		meta.RefIds = result.RefIds
		meta.Url = result.Url
		meta.OwnerId = result.OwnerId
		meta.CreatedAt = result.CreatedAt
		meta.Name = result.Name
		meta.Path = result.Path
	}

	// The rest can be modified
	result, errUC = c.crud.Post(ctx, meta, map[string]string{
		"created_at": time.Now().Format(time.RFC3339),
	})
	if errUC != nil {
		return nil, errUC
	}

	// If it's a new object, generate random ID
	if len(existing) != 1 { // can paranoid check id
		// make random name
		uid, err := uuid.NewRandom()
		if err != nil {
			log.Err(err).Msgf("Error generating UUID for photo uploaded")
			return nil, &types.CommonError{
				Errors: []types.Error{
					{
						HTTPCode: http.StatusInternalServerError,
						Code:     "UPLOAD_FAILED",
						Message:  "Server error when generating image id storage",
					},
				},
			}
		}

		// overwrite name with random (so cannot be guessed)
		result.Name = uid.String()
		t, err := time.Parse(time.RFC3339, result.CreatedAt)
		if err != nil {
			if err != nil {
				log.Err(err).Msgf("Error generating UUID for photo uploaded")
				return nil, &types.CommonError{
					Errors: []types.Error{
						{
							HTTPCode: http.StatusBadRequest,
							Code:     "UPLOAD_FAILED",
							Message:  "Invalid CreatedAt value. Must be RFC339 encoded time",
						},
					},
				}
			}
		}
		result.Path = strconv.Itoa(t.Year()) + "/" + strconv.Itoa(int(t.Month())) + "/" + uid.String()
		if c.namespace != "" {
			result.Path = c.namespace + "/" + result.Path
		}
	}

	// TODO: create proper path / brainstorm better approach (but this works also)
	repometa, err := c.blobRepo.Upload(ctx, result.Path, result.ContentType, payload)
	if err != nil {
		return nil, err
	}

	// Another server protected field
	result.ContentSize = repometa.ContentSize
	result.Url = repometa.PublicURL // the blob storage URL, not this metadata for this case

	// write back
	result, err = c.crud.Post(ctx, result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteAttachment generic binary at path
func (c *crudWithAttachment) Delete(ctx context.Context, userID string, refIDs []string, ID string) (*entity.Attachment, *types.CommonError) {
	result, err := c.crud.Get(ctx, userID, nil, ID)
	if err != nil {
		return nil, err
	}

	_, err = c.blobRepo.Delete(ctx, result[0].Path)
	if err != nil {
		return nil, err
	}

	at, err := c.crud.Delete(ctx, userID, refIDs, ID)
	if err != nil {
		return nil, err
	}

	if c.hideUrl {
		at.Url = ""
		at.Path = ""
	}
	return at, nil
}
