package authapi

type IDTokenKey struct{}
type IDTokenNameKey struct{}

type SignInResponse struct {
	IDToken *string `json:"id_token,omitempty"`
	Expiry  string  `json:"expiry,omitempty"`
	Data    any     `json:"data,omitempty"`
}

type SignInResponseTyped[T any] struct {
	IDToken *string `json:"id_token,omitempty"`
	Expiry  string  `json:"expiry,omitempty"`
	Data    T       `json:"data,omitempty"`
}
