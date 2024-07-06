package types

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
