package entity

import (
	"time"

	mycontent "github.com/desain-gratis/common/usecase/mycontent"
)

func New() *Attachment {
	return new(Attachment)
}

// wrapped provides functionalities that are required by the usecases
type Wrapper struct {
	*Attachment
}

func Wrap(c *Attachment) mycontent.Data {
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

func (c *Wrapper) WithOwnerID(id string) mycontent.Data {
	c.OwnerId = id
	return c
}

func (c *Wrapper) OwnerID() string {
	return c.OwnerId
}

func (c *Wrapper) URL() string {
	return c.Url
}

func (c *Wrapper) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *Wrapper) ParentID() string {
	return ""
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

func (c *Wrapper) RefIDs() []string {
	return []string{c.ParentID()}
}

// TODO: compare difference / calculate hash
