package authapiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	types "github.com/desain-gratis/common/types/http"
)

type client struct {
	endpoint string
}

func New(endpoint string) *client {
	return &client{
		endpoint: endpoint,
	}
}

func (c *client) SignIn(ctx context.Context, idToken string) (authResp *authapi.SignInResponse, errUC *types.CommonError) {
	req, err := http.NewRequest(http.MethodGet, c.endpoint, nil)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "FAILED_CREATE_AUTH_REQUEST",
					Message:  "Failed to build HTTP request for auth. Make sure SignInEndpoint is correct." + err.Error(),
				},
			},
		}
	}

	req = req.WithContext(ctx)

	req.Header.Add("Authorization", "Bearer "+idToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "FAILED_GET_AUTH_REQUEST",
					Message:  "Failed to do request for auth." + err.Error(),
				},
			},
		}
	}

	// todo (low prio): add safeguard limit read (?)
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "FAILED_TO_READ_RESPONSE_PAYLOAD",
					Message:  "Failed to read auth response payload." + err.Error(),
				},
			},
		}
	}

	var loginResp types.CommonResponseTyped[authapi.SignInResponse]
	err = json.Unmarshal(payload, &loginResp)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "FAILED_TO_MARSHAL_SIGN_IN_RESPONSE_PAYLOAD",
					Message:  "Failed to read auth response payload. Something happening in server" + err.Error(),
				},
			},
		}
	}

	if loginResp.Error != nil {
		return nil, loginResp.Error
	}

	return &loginResp.Success, nil
}
