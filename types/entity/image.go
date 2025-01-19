package entity

import (
	"time"

	types "github.com/desain-gratis/common/types/http"
	mycontent "github.com/desain-gratis/common/usecase/mycontent"
)

// Image, a special type of attachment (have thumbnails and image display configuration)
type Image struct {
	Id             string               `json:"id,omitempty"`
	RefIds         []string             `json:"ref_ids,omitempty"`
	ThumbnailUrl   string               `json:"thumbnail_url,omitempty"` // smaller version of the image
	OffsetX        int32                `json:"offset_x,omitempty"`
	OffsetY        int32                `json:"offset_y,omitempty"`
	RatioX         int32                `json:"ratio_x,omitempty"` // will only crop if image width is higher
	RatioY         int32                `json:"ratio_y,omitempty"` // will only crop if image height is higher
	DataUrl        string               `json:"data_url,omitempty"`
	Url            string               `json:"url,omitempty"`             // full version of the image (can be different ratio)
	ScalePx        int32                `json:"scale_px,omitempty"`        // scale in px
	ScaleDirection Image_ScaleDirection `json:"scale_direction,omitempty"` // either: "width" / "height"
	Description    string               `json:"description,omitempty"`
	Tags           []string             `json:"tags,omitempty"`
	Rotation       float64              `json:"rotation,omitempty"`
	CreatedAt      string               `json:"created_at,omitempty"`
	OwnerId        string               `json:"owner_id,omitempty"`
}

type Image_ScaleDirection int32

const (
	Image_WIDTH  Image_ScaleDirection = 0
	Image_HEIGHT Image_ScaleDirection = 1
)

func (c *Image) WithID(id string) mycontent.Data {
	c.Id = id
	return c
}

func (c *Image) ID() string {
	return c.Id
}

func (c *Image) WithNamespace(id string) mycontent.Data {
	c.OwnerId = id
	return c
}

func (c *Image) Namespace() string {
	return c.OwnerId
}

func (c *Image) URL() string {
	return c.Url
}

func (c *Image) WithURL(url string) mycontent.Data {
	c.Url = url
	return c
}

func (c *Image) WithCreatedTime(t time.Time) mycontent.Data {
	c.CreatedAt = t.Format(time.RFC3339)
	return c
}

func (c *Image) CreatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.CreatedAt)
	return t
}

func (c *Image) RefIDs() []string {
	return c.RefIds
}

func (c *Image) Validate() *types.CommonError {
	return nil
}

// TODO: compare difference / calculate hash
