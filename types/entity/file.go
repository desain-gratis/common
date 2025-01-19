package entity

import (
	"time"

	types "github.com/desain-gratis/common/types/http"
	mycontent "github.com/desain-gratis/common/usecase/mycontent"
)

type File struct {
	Id          string   `json:"id,omitempty"`
	RefIds      []string `json:"ref_ids,omitempty"`
	Url         string   `json:"url,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	OwnerId     string   `json:"owner_id,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
}

func (c *File) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *File) ID() string {
	return c.Id
}

func (c *File) WithNamespace(id string) mycontent.Data {
	c.OwnerId = id
	return c
}

func (c *File) Namespace() string {
	return c.OwnerId
}

func (c *File) URL() string {
	return c.Url
}

func (c *File) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *File) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *File) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *File) RefIDs() []string {
	return c.RefIds
}

func (c *File) Validate() *types.CommonError {
	return nil
}

// TODO: compare difference / calculate hash
