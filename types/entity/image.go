package entity

// Image, a special type of attachment (have thumbnails and image display configuration)
type Image struct {
	Id             string               `json:"id,omitempty"`
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
}

type Image_ScaleDirection int32

const (
	Image_WIDTH  Image_ScaleDirection = 0
	Image_HEIGHT Image_ScaleDirection = 1
)
