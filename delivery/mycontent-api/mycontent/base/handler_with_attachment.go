package base

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/blob"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	"github.com/desain-gratis/common/types/entity"
)

var _ mycontent.Usecase[*entity.Attachment] = &HandlerWithAttachment{}
var _ mycontent.Attachable[*entity.Attachment] = &HandlerWithAttachment{}

type HandlerWithAttachment struct {
	*Handler[*entity.Attachment]
	blobRepo  blob.Repository
	hideUrl   bool // if data are quite sensitive
	namespace string
}

// NewWithAttachment creates the basic CRUD handle, but enables attachment
// Whether this is private or not will mostly be depended by the blob repository
func NewAttachment(
	repo content.Repository,
	refSize int,
	blobRepo blob.Repository,
	hideUrl bool,
	blobNamespace string,
) *HandlerWithAttachment {

	return &HandlerWithAttachment{
		Handler:   New[*entity.Attachment](repo, refSize),
		blobRepo:  blobRepo,
		hideUrl:   hideUrl,
		namespace: blobNamespace,
	}
}

// Overwrite for censoring
func (c *HandlerWithAttachment) Get(ctx context.Context, userID string, refIDs []string, ID string) ([]*entity.Attachment, error) {
	result, err := c.Handler.Get(ctx, userID, refIDs, ID)
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
func (c *HandlerWithAttachment) GetAttachment(ctx context.Context, userID string, refIDs []string, ID string) (payload io.ReadCloser, meta *entity.Attachment, err error) {
	result, err := c.Handler.Get(ctx, userID, refIDs, ID)
	if err != nil {
		return nil, nil, err
	}
	if len(result) != 1 {
		return nil, nil, fmt.Errorf("%w: file not found", mycontent.ErrNotFound)
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
func (c *HandlerWithAttachment) Put(ctx context.Context, content *entity.Attachment) (*entity.Attachment, error) {
	return nil, errors.New("not supported")
}

func (c *HandlerWithAttachment) Attach(ctx context.Context, meta *entity.Attachment, payload io.Reader) (*entity.Attachment, error) {
	// TODO: Get all existing data based on user ID, calculate the total size to do validation

	// Check existing, if exist with the same ID, then use existing
	existing, err := c.Handler.Get(ctx, meta.Namespace(), meta.RefIds, meta.Id)
	if err != nil {
		if errors.Is(err, content.ErrNotFound) {
			return nil, err
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
		return nil, fmt.Errorf("%w: created at empty", mycontent.ErrValidation)
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
	result, err = c.Handler.Post(ctx, meta, map[string]string{
		"created_at": time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}

	// If it's a new object, generate random ID
	if len(existing) != 1 { // can paranoid check id
		// make random name
		uid, err := uuid.NewRandom()
		if err != nil {
			return nil, fmt.Errorf("%w: failed to generate uuid", err)
		}

		// overwrite name with random (so cannot be guessed)
		result.Name = uid.String()
		t, err := time.Parse(time.RFC3339, result.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid created at value, must be RFC3339", err)
		}

		result.Path = strconv.Itoa(t.Year()) + "/" + strconv.Itoa(int(t.Month())) + "/" + uid.String()
		if c.namespace != "" {
			result.Path = c.namespace + "/" + result.Path
		}
	}

	// TODO: create proper path / brainstorm better approach (but this works also)
	repometa, err := c.blobRepo.Upload(ctx, result.Path, result, payload)
	if err != nil {
		_, _ = c.Handler.Delete(ctx, meta.Namespace(), meta.RefIDs(), meta.ID())
		return nil, err
	}

	// Another server protected field
	result.ContentSize = uint64(repometa.ContentSize)
	result.Url = repometa.PublicURL // the blob storage URL, not this metadata for this case

	// write back
	result, err = c.Handler.Post(ctx, result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteAttachment generic binary at path
func (c *HandlerWithAttachment) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (*entity.Attachment, error) {
	result, err := c.Handler.Get(ctx, namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	_, err = c.blobRepo.Delete(ctx, result[0].Path)
	if err != nil {
		return nil, err
	}

	at, err := c.Handler.Delete(ctx, namespace, refIDs, ID)
	if err != nil {
		return nil, err
	}

	if c.hideUrl {
		at.Url = ""
		at.Path = ""
	}
	return at, nil
}
