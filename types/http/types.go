package types

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"
)

type CommonResponse struct {
	Success any          `json:"success,omitempty"`
	Error   *CommonError `json:"error,omitempty"`
}

type CommonResponseTyped[T any] struct {
	Success T            `json:"success,omitempty"`
	Error   *CommonError `json:"error,omitempty"`
}

type CommonError struct {
	Errors []Error `json:"errors,omitempty"`
}

func (c *CommonError) Err() error {
	var result []string
	for _, err := range c.Errors {
		result = append(result, "("+err.Code+") "+err.Message)
	}
	return errors.New(strings.Join(result, ","))
}

type Error struct {
	HTTPCode int    `json:"http_code,omitempty"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	URL      string `json:"url,omitempty"`
	IconURL  string `json:"icon_url,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// User Identity
type ProfileAvatar struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	DisplayName string `json:"display_name"`
	ImageURL    string `json:"image_url_100"`
	ClickURL    string `json:"click_url"`
	IsVerified  bool   `json:"is_verified"`
	LastOnline  string `json:"last_online"`
	Location    string `json:"location"`
}

func (i *ProfileAvatar) GetID() int64 {
	return i.ID
}

func (i *ProfileAvatar) GetOwnerID() int64 {
	return i.ID
}

func (i *ProfileAvatar) SetID(id int64) {
	i.ID = id
}

func (i *ProfileAvatar) SetOwnerID(id int64) {
	i.ID = id
}

func (i *ProfileAvatar) MarshalTo() ([]byte, error) {
	return json.Marshal(i)
}

func (i *ProfileAvatar) UnmarshalFrom(payload []byte) error {
	return json.Unmarshal(payload, i)
}

func (i *ProfileAvatar) Validate() *CommonError {
	return nil
}

func SerializeError(err *CommonError) []byte {
	d, errMarshal := json.Marshal(&CommonResponse{
		Error: err,
	})
	if errMarshal != nil {
		log.Err(errMarshal).Msgf("Failed to parse err")
	}
	return d
}
