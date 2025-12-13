package idtokenverifier

import (
	"net/http"
	"strings"

	types "github.com/desain-gratis/common/types/http"
)

func getToken(authorizationToken string) (string, error) {
	token := strings.Split(authorizationToken, " ")
	if len(token) < 2 {
		return "", &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "INVALID_OR_EMPTY_AUTHORIZATION",
					Message:  "Authorization header is not valid",
				},
			},
		}
	}
	return token[1], nil
}
