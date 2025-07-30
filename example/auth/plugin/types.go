package plugin

import "github.com/desain-gratis/common/types/protobuf/session"

type AuthData struct {
	// Profile from OIDC provider
	LoginProfile *Profile `json:"login_profile,omitempty"`

	Locale  []string `json:"locale,omitempty"`
	IDToken *string  `json:"id_token,omitempty"`
	// Collection of grants NOT signed, for debugging.
	// DO NOT USE THIS FOR BACK END VALIDATION!!!
	Grants map[string]*session.Grant `json:"grants"`
	Expiry string                    `json:"expiry,omitempty"`
	Data   any                       `json:"data,omitempty"`
}

type Profile struct {
	URL              string `json:"url"`
	DisplayName      string `json:"display_name"`
	ImageDataURL     string `json:"image_data_url"`
	ImageURL         string `json:"image_url"`
	Avatar1x1URL     string `json:"avatar_1x1_url"`
	Background3x1URL string `json:"background_3x1_url"`
	Email            string `json:"email"`
}
