package entity

// It's an upload config. It will be translated as attachment in the end.
type File struct {
	Id          string   `json:"id,omitempty"`
	RefIds      []string `json:"ref_ids,omitempty"`
	Url         string   `json:"url,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	OwnerId     string   `json:"owner_id,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
}
