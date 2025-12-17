package entity

import (
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
)

type Attachment struct {
	Id           string   `json:"id,omitempty"`
	RefIds       []string `json:"ref_ids,omitempty"`
	OwnerId      string   `json:"owner_id,omitempty"`
	Path         string   `json:"path,omitempty"` // private path of the resource
	Name         string   `json:"name,omitempty"` // name of the resource
	Url          string   `json:"url,omitempty"`  // public URL of the resource
	ContentType  string   `json:"content_type,omitempty"`
	ContentSize  uint64   `json:"content_size,omitempty"`
	Description  string   `json:"description,omitempty"`
	Tags         []string `json:"tags,omitempty"` // meta data
	Ordering     int32    `json:"ordering,omitempty"`
	ImageDataUrl string   `json:"image_data_url,omitempty"` // image (thumbnail) data URL if applicable
	CreatedAt    string   `json:"created_at,omitempty"`
	Hash         string   `json:"hash,omitempty"` // hash of the attachment
}

func (c *Attachment) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *Attachment) ID() string {
	return c.Id
}

func (c *Attachment) WithNamespace(id string) mycontent.Data {
	c.OwnerId = id
	return c
}

func (c *Attachment) Namespace() string {
	return c.OwnerId
}

func (c *Attachment) URL() string {
	return c.Url
}

func (c *Attachment) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *Attachment) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Attachment) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Attachment) RefIDs() []string {
	return c.RefIds
}

func (c *Attachment) Validate() error {
	return nil
}

// TODO: compare difference / calculate hash
