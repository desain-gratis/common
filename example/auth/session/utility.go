package session

import (
	"context"
	"net/http"
	"strings"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/signing"
	"google.golang.org/protobuf/proto"
)

// parseAuthorizationToken is a http Handler that provides authorization data to each request
// You can use your own type instead of *session.SessionData
func parseAuthorizationToken(ctx context.Context, verifier signing.Verifier, authorizationToken string) (*session.SessionData, *types.CommonError) {
	token := strings.Split(authorizationToken, " ")
	if len(token) < 2 {
		return &session.SessionData{}, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "INVALID_OR_EMPTY_AUTHORIZATION",
					Message:  "Authorization header is no valid",
				},
			},
		}
	}

	authToken := token[1]
	payload, errUC := verifier.Verify(ctx, authToken)
	if errUC != nil {
		return &session.SessionData{}, errUC
	}

	var sessionData session.SessionData
	err := proto.UnmarshalOptions{AllowPartial: true}.Unmarshal(payload, &sessionData)
	if err != nil {
		return &sessionData, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusUnauthorized,
					Code:     "UNAUTHORIZED",
					Message:  "Your role is unauthorized for this API.",
				},
			},
		}
	}

	return &sessionData, nil
}
