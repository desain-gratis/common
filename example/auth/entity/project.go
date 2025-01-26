package entity

import (
	"time"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

type Project struct {
	Ns          string `json:"namespace"`
	Url         string `json:"url"`
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

func (c *Project) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *Project) ID() string {
	return c.Id
}

func (c *Project) WithNamespace(id string) mycontent.Data {
	c.Ns = id
	return c
}

func (c *Project) Namespace() string {
	return c.Ns
}

func (c *Project) URL() string {
	return c.Url
}

func (c *Project) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *Project) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Project) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Project) RefIDs() []string {
	return nil
}

func (c *Project) Validate() *types.CommonError {
	return nil
}
