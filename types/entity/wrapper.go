package entity

import (
	"time"

	mycontent "github.com/desain-gratis/common/usecase/mycontent"
)

func (c *Attachment) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *Attachment) ID() string {
	return c.Id
}

func (c *Attachment) WithOwnerID(id string) mycontent.Data {
	c.OwnerId = id
	return c
}

func (c *Attachment) OwnerID() string {
	return c.OwnerId
}

func (c *Attachment) URL() string {
	return c.Url
}

func (c *Attachment) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *Attachment) ParentID() string {
	return ""
}

func (c *Attachment) WithStartTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Attachment) StartTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Attachment) WithEndTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Attachment) EndTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
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
	return []string{c.ParentID()}
}

// TODO: compare difference / calculate hash
