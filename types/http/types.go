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

func (c *CommonError) Error() string {
	var result []string
	if c == nil {
		return ""
	}
	for _, err := range c.Errors {
		result = append(result, "("+err.Code+") "+err.Message)
	}
	return strings.Join(result, ",")
}

type Error struct {
	HTTPCode int    `json:"http_code,omitempty"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	URL      string `json:"url,omitempty"`
	IconURL  string `json:"icon_url,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

func SerializeError(err error) []byte {
	d, errMarshal := json.Marshal(&CommonResponse{
		Error: &CommonError{
			Errors: []Error{
				{Message: err.Error()},
			},
		},
	})
	if errMarshal != nil {
		log.Err(errMarshal).Msgf("Failed to parse err")
	}
	return d
}
