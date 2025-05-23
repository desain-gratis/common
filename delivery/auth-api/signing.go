package authapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/signing"
)

type signingService struct {
	verifier signing.Verifier
	signer   signing.Signer
}

// Base signing service
func New(
	signer signing.Signer,
	verifier signing.Verifier,
) *signingService {
	return &signingService{
		signer:   signer,
		verifier: verifier,
	}
}

// Keys allows other service to verify this published delivery Open ID credential
func (s *signingService) Keys(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	keys, errUC := s.signer.Keys(r.Context())
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
	data, errUC := verifyAuthorizationHeader(r.Context(), s.verifier, authHeader)
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	var session session.SessionData
	err := proto.Unmarshal(data, &session)
	if err != nil {
		log.Debug().Msg("Token schema changed. User need to update their token.")
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Code: "INVALID_TOKEN", HTTPCode: http.StatusBadRequest, Message: "Token schema changed. Please log in again."},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	payload, _err := json.Marshal(&types.CommonResponse{
		Success: &session,
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

func verifyAuthorizationHeader(ctx context.Context, verifier signing.Verifier, value string) (payload []byte, errUC *types.CommonError) {
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
