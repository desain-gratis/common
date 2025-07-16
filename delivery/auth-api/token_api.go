package authapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/signing"
)

const maxHeaderSize = 1 << 20

type TokenParser func(ctx context.Context, payload []byte) (any, *types.CommonError)

type signingService struct {
	signerVerifier SignerVerifier
	tokenParser    TokenParser
}

// NewTokenAPI
func NewTokenAPI(
	signerVerifier SignerVerifier,
	tokenParser TokenParser,
) *signingService {
	return &signingService{
		signerVerifier: signerVerifier,
		tokenParser:    tokenParser,
	}
}

// Keys allows other service to verify this published delivery Open ID credential
func (s *signingService) Keys(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	keys, errUC := s.signerVerifier.Keys(r.Context())
	if errUC != nil {
		if r.Context().Err() != nil {
			return
		}

		log.Err(errUC.Err()).Msgf("Failed to get keys")
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	payload, err := json.Marshal(&types.CommonResponse{
		Success: keys,
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}

		log.Err(err).Msgf("Failed to parse payload")
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to parse response", Code: "SERVER_ERROR"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

// Debug dumps valid token payload
func (s *signingService) Debug(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	authHeader := r.Header.Get("Authorization")
	data, errUC := verifyAuthorizationHeader(r.Context(), s.signerVerifier, authHeader)
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	result, errUC := s.tokenParser(r.Context(), data)
	if errUC != nil {
		log.Debug().Msg("Token schema changed. User need to update their token.")
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	payload, _err := json.Marshal(&types.CommonResponse{
		Success: result,
	})
	if _err != nil {
		if r.Context().Err() != nil {
			return
		}

		log.Err(_err).Msgf("Failed to parse payload")
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to parse response", Code: "SERVER_ERROR"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

func (s *signingService) MultiKeys(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	orgID := r.URL.Query().Get("org")
	if orgID == "" {
		cer := &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "EMPTY_ORGANIZATION",
					Message:  "Please spcecify `org` parameter",
				},
			},
		}
		errMessage := types.SerializeError(cer)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	s.Keys(w, r, p)
}

func verifyAuthorizationHeader(ctx context.Context, verifier signing.Verifier, value string) (payload []byte, errUC *types.CommonError) {
	if len(payload) > maxHeaderSize {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "BAD_REQUEST", HTTPCode: http.StatusBadRequest, Message: "Invalid authorization header. Too long."},
			},
		}
	}
	token := strings.Split(value, " ")
	if len(token) < 2 || token[1] == "" {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "UNAUTHORIZED", HTTPCode: http.StatusUnauthorized, Message: "Invalid authorization token format"},
			},
		}
	}

	data, errUC := verifier.Verify(ctx, token[1])
	if errUC != nil {
		log.Warn().Msgf("Failed to parse payload %v", errUC)
		return nil, errUC
	}

	return data, nil
}
