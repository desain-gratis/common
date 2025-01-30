package entity

import (
	"time"

	"github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

type (

	// UserAuthorization
	// Defines which "namespace" (or "project", or "tenant", etc.) the application user can access
	UserAuthorization struct {
		Ns             string                   `json:"namespace"` // for admin, internal
		Url            string                   `json:"url"`
		Id             string                   `json:"id"` // email
		DefaultProfile UserProfile              `json:"profile"`
		DefaultProject string                   `json:"default_project"`
		Authorization  map[string]Authorization `json:"authorization"` // for application user "namespace" authorization
		CreatedAt      string                   `json:"created_at"`
	}

	// Authorization
	// Which & what operation is authorized for application user?
	Authorization struct {
		UserGroupID2       string              `json:"group_id_new",omitempty`
		UserGroupID        map[string]struct{} `json:"group_id",omitempty`
		UiAndApiPermission map[string]bool     `json:"api_and_ui_permission"`
	}

	// UserProfile
	// Default profile
	UserProfile struct {
		DisplayName   string        `json:"display_name"`
		Description   string        `json:"description"`
		Avatar1x1     *entity.Image `json:"avatar_1x1_url"`
		Background3x1 *entity.Image `json:"background_3x1_url"`
		CreatedAt     string        `json:"created_at"`
	}
)

func (c *UserAuthorization) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *UserAuthorization) ID() string {
	return c.Id
}

func (c *UserAuthorization) WithNamespace(id string) mycontent.Data {
	c.Ns = id
	return c
}

func (c *UserAuthorization) Namespace() string {
	return c.Ns
}

func (c *UserAuthorization) URL() string {
	return c.Url
}

func (c *UserAuthorization) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *UserAuthorization) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *UserAuthorization) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *UserAuthorization) RefIDs() []string {
	return nil
}

func (c *UserAuthorization) Validate() *types.CommonError {
	return nil
}
