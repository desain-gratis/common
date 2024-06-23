package signin

import (
	"context"

	"github.com/julienschmidt/httprouter"

	"github.com/desain-gratis/common/types/protobuf/session"
)

// Data relates to sign in context of the user, that will be passed as JWT Token
// This data are currently public, so no secret2

type Context interface {
	Get(ctx context.Context, data session.SessionData) error
}

type signing struct{}

func New() *signing {
	return &signing{}
}

func (s *signing) Verifier(handler httprouter.Handle) httprouter.Handle {
	return handler
}
