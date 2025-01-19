package entity

import (
	"time"

	"github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

var _ mycontent.Data = &UserProfile{}

type UserProfile struct {
	Id             string        `json:"id"`
	OwnerId        string        `json:"owner_id"`
	Url            string        `json:"url"`
	Name           string        `json:"name"`
	CreatedAt      string        `json:"created_at"`
	OrganizationID string        `json:"organization_id"`
	Thumbnail_1x1  *entity.Image `json:"thumbnail_1x1"`
}

func (c *UserProfile) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *UserProfile) ID() string {
	return c.Id
}

func (c *UserProfile) WithNamespace(id string) mycontent.Data {
	c.OwnerId = id
	return c
}

func (c *UserProfile) Namespace() string {
	return c.OwnerId
}

func (c *UserProfile) URL() string {
	return c.Url
}

func (c *UserProfile) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func ValidateUserProfile(c *UserProfile) *types.CommonError {
	return nil
}

func (c *UserProfile) WithStartTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *UserProfile) StartTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *UserProfile) WithEndTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *UserProfile) EndTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *UserProfile) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *UserProfile) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *UserProfile) RefIDs() []string {
	return []string{c.OrganizationID}
}

func (c *UserProfile) Validate() *types.CommonError {
	return nil
}
