package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/signing"
	jwtrsa "github.com/desain-gratis/common/utility/secret/rsa"
)

var _ signing.Verifier = &remoteVerifier{}

type remoteVerifier struct {
	keysURL  string
	client   *http.Client
	keys     []keys
	rsaStore jwtrsa.Provider
}

func NewRemoteLoginVerifier(rsaStore jwtrsa.Provider, keysURL string) *remoteVerifier {
	s := &remoteVerifier{
		keysURL: keysURL,
		client: &http.Client{
			Timeout: 1000 * time.Millisecond,
		},
		rsaStore: rsaStore,
	}
	// TODO graceful error and retry
	keys, err := s.updateKeys(context.Background())
	if err != nil {
		log.Err(err.Err()).Str("url", s.keysURL).Msgf("Failed to parse public signing key %+v", err)
	}
	s.keys = keys

	return s
}

func (s *remoteVerifier) Verify(ctx context.Context, token string) (claim []byte, errUC *types.CommonError) {
	if len(s.keys) == 0 {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "EMPTY_KEYS", HTTPCode: http.StatusInternalServerError, Message: "No keys in this server :("},
			},
		}
	}

	payload, err := s.rsaStore.ParseRSAJWTToken(token, s.keys[0].KeyID)
	if err != nil {
		return claim, &types.CommonError{
			Errors: []types.Error{
				{Code: "INVALID_TOKEN", HTTPCode: http.StatusBadRequest, Message: "Invalid token"},
			},
		}
	}

	return payload, nil
}

func (s *remoteVerifier) updateKeys(ctx context.Context) (result []keys, errUC *types.CommonError) {
	req, err := http.NewRequest(http.MethodGet, s.keysURL, nil)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_CREATE_REQUEST", Message: "Failed to create request", HTTPCode: http.StatusInternalServerError},
			},
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_SEND_REQUEST", Message: "Failed to send request", HTTPCode: http.StatusInternalServerError},
			},
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_READ_REQUEST_BODY", Message: "Failed to read response body", HTTPCode: http.StatusInternalServerError},
			},
		}
	}

	var commonResponse types.CommonResponseTyped[[]signing.Keys]
	err = json.Unmarshal(body, &commonResponse)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_UNMARSHAL", Message: "Failed to unmarshal response body", HTTPCode: http.StatusInternalServerError},
			},
		}
	}
	if commonResponse.Error != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "DEPENDENCY_ERROR", Message: "Dependency error", HTTPCode: http.StatusInternalServerError},
			},
		}
	}

	t := time.Now()
	var k []keys
	for _, v := range commonResponse.Success {
		tc, _ := time.Parse(time.RFC3339, v.CreatedAt)
		k = append(k, keys{
			Keys: signing.Keys{
				CreatedAt: v.CreatedAt,
				KeyID:     v.KeyID,
				Key:       v.Key,
			},
			cacheUpdateTime: t,
			createdAt:       tc,
		})
		s.rsaStore.StorePublicKey(v.KeyID, v.Key)
	}

	return k, nil
}
