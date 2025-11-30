package entity

import (
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	types "github.com/desain-gratis/common/types/http"
)

var _ mycontent.Data = &UserPage{}

type UserPage struct {
	Ns             string `json:"namespace"`
	Id             string `json:"id"`
	Url            string `json:"url"`
	Name           string `json:"name"`
	CreatedAt      string `json:"created_at"`
	ProfileId      string `json:"profile_id"`
	OrganizationID string `json:"organization_id"`
}

func (c *UserPage) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *UserPage) ID() string {
	return c.Id
}

func (c *UserPage) WithNamespace(id string) mycontent.Data {
	c.Ns = id
	return c
}

func (c *UserPage) Namespace() string {
	return c.Ns
}

func (c *UserPage) URL() string {
	return c.Url
}

func (c *UserPage) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func ValidateUserPage(c *UserPage) *types.CommonError {
	return nil
}

func (c *UserPage) WithStartTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *UserPage) StartTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *UserPage) WithEndTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *UserPage) EndTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *UserPage) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *UserPage) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *UserPage) RefIDs() []string {
	// allows to be get by organization id and profile id
	return []string{c.OrganizationID, c.ProfileId}
}

func (c *UserPage) Validate() error {
	return nil
}
