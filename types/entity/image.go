package entity

// Image, a special type of attachment (have thumbnails and image display configuration)
// It's an upload config. It will be translated as attachment in the end.
type Image struct {
	Id             string         `json:"id,omitempty"`
	RefIds         []string       `json:"ref_ids,omitempty"`
	ThumbnailUrl   string         `json:"thumbnail_url,omitempty"` // smaller version of the image
	OffsetX        int32          `json:"offset_x,omitempty"`
	OffsetY        int32          `json:"offset_y,omitempty"`
	RatioX         int32          `json:"ratio_x,omitempty"` // will only crop if image width is higher
	RatioY         int32          `json:"ratio_y,omitempty"` // will only crop if image height is higher
	DataUrl        string         `json:"data_url,omitempty"`
	Url            string         `json:"url,omitempty"`             // full version of the image (can be different ratio)
	ScalePx        int32          `json:"scale_px,omitempty"`        // scale in px
	ScaleDirection ScaleDirection `json:"scale_direction,omitempty"` // either: "width" / "height"
	Description    string         `json:"description,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	Rotation       float64        `json:"rotation,omitempty"`
	CreatedAt      string         `json:"created_at,omitempty"`
	OwnerId        string         `json:"owner_id,omitempty"`
}

type ScaleDirection int32

var (
	SCALE_DIRECTION_HORIZONTAL ScaleDirection = 0
	SCALE_DIRECTION_VERTICAL   ScaleDirection = 1
)
