package attachment

import (
	"time"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

func New() *types.Attachment {
	return new(types.Attachment)
}

// wrapped provides functionalities that are required by the usecases
type Wrapper struct {
	*types.Attachment
}

func Wrap(c *types.Attachment) mycontent.Data {
	return &Wrapper{
		Attachment: c,
	}
}

func (c *Wrapper) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *Wrapper) ID() string {
	return c.Id
}

func (c *Wrapper) URL() string {
	return c.Url
}

func (c *Wrapper) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *Wrapper) MainRefID() string {
	return c.RefId
}

func (c *Wrapper) InternalPath() string {
	return c.Path
}

func (c *Wrapper) Size() int64 {
	return c.ContentSize
}

func (c *Wrapper) Type() string {
	return c.ContentType
}

func (c *Wrapper) SetInternalPath(path string) {
	c.Path = path
}

func (c *Wrapper) SetURL(path string) {
	c.Url = path
}

func (c *Wrapper) SetSize(size int64) {
	c.ContentSize = size
}

func (c *Wrapper) SetType(contentType string) {
	c.ContentType = contentType
}

func (c *Wrapper) WithOwnerID(id string) mycontent.Data {
	c.OwnerId = id
	return c
}
func (c *Wrapper) OwnerID() string {
	return c.OwnerId
}

func (c *Wrapper) WithStartTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}
func (c *Wrapper) StartTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Wrapper) WithEndTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}
func (c *Wrapper) EndTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Wrapper) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Wrapper) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func Validate(c *types.Attachment) *types.CommonError {
	return nil
}
