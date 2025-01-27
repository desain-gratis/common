package handler

import (
	"context"
	"net/http"

	"google.golang.org/api/idtoken"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/signing"
)

var _ signing.VerifierOf[*idtoken.Payload] = &googleVerifier{}

type googleVerifier struct {
	clientID string
}

func NewGoogleAuth(clientID string) *googleVerifier {
	return &googleVerifier{
		clientID: clientID,
	}
}

func (g *googleVerifier) VerifyAs(ctx context.Context, token string) (*idtoken.Payload, *types.CommonError) {
	result, err := idtoken.Validate(ctx, token, g.clientID)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "UNAUTHORIZED", HTTPCode: http.StatusBadRequest, Message: "Unauthorized. Please specify correct token."},
			},
		}
	}

	return result, nil
}
