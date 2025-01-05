package helper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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

func SetError(w http.ResponseWriter, body types.Error, code int) {
	errMessage := types.SerializeError(&types.CommonError{
		Errors: []types.Error{body},
	})
	w.WriteHeader(code)
	w.Write(errMessage)
}

func GetCredentials(header http.Header) (string, string, *types.Error) {
	organizationID := header.Get("X-Org")
	if organizationID == "" {
		err := types.Error{HTTPCode: http.StatusBadRequest, Message: "Please specify 'X-Org' in header", Code: "EMPTY_ORGANIZATION_ID"}
		return organizationID, "", &err
	}

	userID := header.Get("User-ID")
	if userID == "" {
		err := types.Error{HTTPCode: http.StatusBadRequest, Message: "Please specify 'User-ID' in header", Code: "EMPTY_USER_ID"}
		return organizationID, userID, &err
	}

	return organizationID, userID, nil
}

func CheckRequiredFields(form url.Values, requiredFields []string) *types.Error {
	for _, key := range requiredFields {
		if len(form[key]) == 0 {
			err := types.Error{Message: fmt.Sprintf("Please fill `%s` field", key), Code: "EMPTY_REQUIRED_FIELD"}
			return &err
		}
	}

	return nil
}
