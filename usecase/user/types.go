package user

import (
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
)

type (
	Details struct {
		ID              string                   `json:"id"` // email
		Profile         UserProfile              `json:"profile"`
		GSI             GSIConfig                `json:"gsi"`
		MIP             MIPConfig                `json:"mip"`
		DefaultHomepage string                   `json:"default_homepage"`
		Authorization   map[string]Authorization `json:"authorization"` // organization/tenant id as key
	}

	Authorization struct {
		UserID             string              `json:"user_id"` // organization/tenant id
		Name               string              `json:"name"`    // organization/tenant name
		UserGroupID        map[string]struct{} `json:"user_group_id"`
		UiAndApiPermission map[string]bool     `json:"api_and_ui_permission"`
	}

	Payload struct {
		Ns              string                   `json:"namespace"`
		Url             string                   `json:"url"`
		Id              string                   `json:"id"` // email
		Profile         UserProfile              `json:"profile"`
		GSI             GSIConfig                `json:"gsi"`
		MIP             MIPConfig                `json:"mip"`
		DefaultHomepage string                   `json:"default_homepage"`
		Authorization   map[string]Authorization `json:"authorization"` // organization/tenant id as key
		CreatedAt       string                   `json:"created_at"`
	}

	UserProfile struct {
		ID               string `json:"id"`
		ImageURL         string `json:"image_url"`
		Name             string `json:"name"`
		DisplayName      string `json:"display_name"`
		Role             string `json:"role"`
		Description      string `json:"description"`
		Avatar1x1URL     string `json:"avatar_1x1_url"`
		Background3x1URL string `json:"background_3x1_url"`
		CreatedAt        string `json:"created_at"`
	}

	GSIConfig struct {
		Email string `json:"email"`
	}

	MIPConfig struct {
		Email string `json:"email"`
	}
)

func (c *Payload) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *Payload) ID() string {
	return c.Id
}

func (c *Payload) WithNamespace(id string) mycontent.Data {
	c.Ns = id
	return c
}

func (c *Payload) Namespace() string {
	return c.Ns
}

func (c *Payload) URL() string {
	return c.Url
}

func (c *Payload) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *Payload) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Payload) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Payload) RefIDs() []string {
	return nil
}

func (c *Payload) Validate() error {
	return nil
}
