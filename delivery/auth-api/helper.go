package authapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/signing"
	"google.golang.org/protobuf/proto"

	types "github.com/desain-gratis/common/types/http"
)

func ParseAuthorizationToken(ctx context.Context, uc signing.Verifier, authorizationToken string) (*session.SessionData, *types.CommonError) {
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
	payload, errUC := uc.Verify(ctx, authToken)
	if errUC != nil {
		return &session.SessionData{}, errUC
	}

	// payload := []byte("")

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

func GetCredentials(header http.Header) (string, string, *types.Error) {
	organizationID := header.Get("X-Org")
	if organizationID == "" {
		err := types.Error{HTTPCode: http.StatusBadRequest, Message: "Please specify 'X-Org' in header", Code: "EMPTY_ORGANIZATION_ID"}
		return organizationID, "", &err
	}

	userID := header.Get("X-User-ID")
	if userID == "" {
		err := types.Error{HTTPCode: http.StatusBadRequest, Message: "Please specify 'X-User-ID' in header", Code: "EMPTY_USER_ID"}
		return organizationID, userID, &err
	}

	return organizationID, userID, nil
}
