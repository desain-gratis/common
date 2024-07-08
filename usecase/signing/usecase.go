package signing

// the package name should be signing; token related

import (
	"context"
	"time"

	types "github.com/desain-gratis/common/types/http"
)

type Usecase interface {
	Publisher
	Verifier
}

// Usecase converts OIDC credential to another identity token
// With Subject and Issuer field changed
// TOKEN Based authorization
type Publisher interface {
	// Convert any data proto to signed JWT token
	Sign(ctx context.Context, claim []byte) (token string, expiry time.Time, errUC *types.CommonError)

	// Get keys to verify token this usecase have signed
	Keys(ctx context.Context) (keys []Keys, errUC *types.CommonError)
}

type Verifier interface {
	// Verify token
	Verify(ctx context.Context, token string) (claim []byte, errUC *types.CommonError)
}

// Specific case of Verifier
type VerifierOf[T any] interface {
	// Verify token
	VerifyAs(ctx context.Context, token string) (claim *T, errUC *types.CommonError)
}

// TODO: Stateful Session authorization with credential / active user stored somewhere in DB
// TODO: Useful for elevated permission such as password / doing important things
type StatefulAuthorization interface {
	StoreSession(ctx context.Context, key []byte, payload []byte) (errUC *types.CommonError)
	GetSession(ctx context.Context, key []byte) (payload []byte, errUC *types.CommonError)
}

type Keys struct {
	CreatedAt string `json:"created_at"`
	KeyID     string `json:"key_id"`
	Key       string `json:"key"`
}

type Response struct {
	IDToken string  `json:"id_token"`
	Profile Profile `json:"profile"`
}

type Profile struct {
	DisplayName    string `json:"display_name"`
	ImageDataUrl   string `json:"image_data_url"`
	ImageURLSmall  string `json:"image_url_small"`
	ImageURLMedium string `json:"image_url_medium"`
	ImageURLLarge  string `json:"image_url_large"`
}
