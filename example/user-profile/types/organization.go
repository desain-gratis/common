package types

import (
	"time"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

var _ mycontent.Data = &Organization{}

type Organization struct {
	Id        string `json:"id"`
	OwnerId   string `json:"owner_id"`
	Url       string `json:"url"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func (c *Organization) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *Organization) ID() string {
	return c.Id
}

func (c *Organization) WithOwnerID(id string) mycontent.Data {
	c.OwnerId = id
	return c
}

func (c *Organization) OwnerID() string {
	return c.OwnerId
}

func (c *Organization) URL() string {
	return c.Url
}

func (c *Organization) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *Organization) ParentID() string {
	return ""
}

func ValidateOrganization(c *Organization) *types.CommonError {
	return nil
}

func (c *Organization) WithStartTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Organization) StartTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Organization) WithEndTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Organization) EndTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Organization) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Organization) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Organization) RefIDs() []string {
	// only accessible by user_id or id
	return []string{}
}
