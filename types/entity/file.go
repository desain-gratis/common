package entity

type Attachment struct {
	Id           string   `json:"id,omitempty"`
	RefId        string   `json:"ref_id,omitempty"`
	OwnerId      string   `json:"owner_id,omitempty"`
	Path         string   `json:"path,omitempty"` // private path of the resource
	Name         string   `json:"name,omitempty"` // name of the resource
	Url          string   `json:"url,omitempty"`  // public URL of the resource
	ContentType  string   `json:"content_type,omitempty"`
	ContentSize  int64    `json:"content_size,omitempty"`
	Description  string   `json:"description,omitempty"`
	Tags         []string `json:"tags,omitempty"` // meta data
	Ordering     int32    `json:"ordering,omitempty"`
	ImageDataUrl string   `json:"image_data_url,omitempty"` // image (thumbnail) data URL if applicable
	CreatedAt    string   `json:"created_at,omitempty"`
	Hash         string   `json:"hash,omitempty"` // hash of the attachment
}

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

type File struct {
	Id          string   `json:"id,omitempty"`
	Url         string   `json:"url,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}
