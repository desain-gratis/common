package authapiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (c *client) SignIn(ctx context.Context, idToken string) (authResp *authapi.SignInResponse, err error) {
	req, err := http.NewRequest(http.MethodGet, c.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to %v err: %w", c.endpoint, err)
	}

	req = req.WithContext(ctx)

	req.Header.Add("Authorization", "Bearer "+idToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %w", err)
	}

	// todo (low prio): add safeguard limit read (?)
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var loginResp types.CommonResponseTyped[authapi.SignInResponse]
	err = json.Unmarshal(payload, &loginResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sign-in response: %w", err)
	}

	if loginResp.Error != nil {
		return nil, fmt.Errorf("server response with error: %w", parseError(loginResp.Error))
	}

	return &loginResp.Success, nil
}

func parseError(resp *types.CommonError) error {
	errs := make([]error, 0, len(resp.Errors))
	for _, err := range resp.Errors {
		errs = append(errs, fmt.Errorf("code: %v message: %v", err.Code, err.Message))
	}
	return errors.Join(errs...)
}
