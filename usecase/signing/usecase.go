package signing

// the package name should be signing; token related

import (
	"context"
	"time"

	types "github.com/desain-gratis/common/types/http"
)

type Usecase interface {
	Signer
	Verifier
}

// Usecase converts OIDC credential to another identity token
// With Subject and Issuer field changed
// TOKEN Based authorization
type Signer interface {
	// Convert any data proto to signed JWT token
	Sign(ctx context.Context, claim []byte, expire time.Time) (token string, errUC *types.CommonError)

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
	VerifyAs(ctx context.Context, token string) (claim T, errUC *types.CommonError)
}

type Keys struct {
	CreatedAt string `json:"created_at"`
	KeyID     string `json:"key_id"`
	Key       string `json:"key"`
}
