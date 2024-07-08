package authapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/nbutton23/zxcvbn-go"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	accountUC "github.com/desain-gratis/common/usecase/account"
	"github.com/desain-gratis/common/usecase/signing"
)

type passwordService struct {
	*signingService
	account    accountUC.Usecase
	openIDAuth signing.Verifier
}

// ParseTokenAsGenericToken is a utility function to parse the token published by NewPassword
func ParseTokenAsGenericToken(payload []byte) (result *session.GenericToken, errUC *types.CommonError) {
	var data session.GenericToken
	err := proto.Unmarshal(payload, &data)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "INVALID_TOKEN", HTTPCode: http.StatusBadRequest, Message: "Token schema changed. Please log in again."},
			},
		}
	}

	return &data, nil
}

func NewPasswordService(
	openIDAuth signing.Verifier,
	signing signing.Usecase,
	account accountUC.Usecase,
) *passwordService {
	return &passwordService{
		signingService: &signingService{
			signing: signing,
		},
		openIDAuth: openIDAuth,
		account:    account,
	}
}

type CreatePasswordResponse struct {
	Token string `json:"token"`
}

func (s *passwordService) Create(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	payload, errUC := s.verifyAuthorizationHeader(r.Context(), s.openIDAuth, r.Header.Get("Authorization"))
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	// Expect open ID token
	openID, errUC := ParseTokenAsOpenID(payload)
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	oidcIssuer := openID.NonRegisteredId.Iss
	oidcSubject := openID.NonRegisteredId.Sub
	exist, errUC := s.account.PasswordExist(r.Context(), oidcIssuer, oidcSubject)
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}
	if exist {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "BAD_REQUEST", Message: "Password already created for your account"},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	// Read form data password + validate
	// TODO: limit size
	err := r.ParseForm()
	if err != nil {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "BAD_REQUEST", Message: "Invalid form data"},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	password := r.FormValue("password")
	errUC = s.validatePassword(password)
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	// Usecase get user ID based on open ID Iss and Sub
	userID, errUC := s.account.UpdatePassword(r.Context(), oidcIssuer, oidcSubject, password)
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	buf := make([]byte, 256)
	rand.Reader.Read(buf)

	tokenData := &session.SessionData{
		NonRegisteredId: openID.NonRegisteredId,
		UserId:          userID,
	}

	payload, err = proto.Marshal(tokenData)
	if err != nil {
		log.Err(err).Msg("Failed to marshal token data.")
		payload = make([]byte, 0)
	}

	// ignore error intentionally
	// create password at this time is already successful
	// will document on contract
	token, _, errUC := s.signing.Sign(r.Context(), payload)
	if errUC != nil {
		log.Err(errUC.Err()).Msg("Failed to sign token for password creation")
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}
	// log.Debug().Msgf("Dari create password: signed token `%v`", string(token))

	resp, err := json.Marshal(&types.CommonResponse{
		Success: CreatePasswordResponse{
			Token: token,
		},
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
	w.Write(resp)
}

func (s *passwordService) Update(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Need token from google ID
	// Need token from (previous) password
	// TODO: later
}

func (s *passwordService) Token(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Need token from google ID
	// Read form password
	// TODO: later
}

func (s *passwordService) Reset(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// TODO: later
}

func (s *passwordService) validatePassword(password string) (errUC *types.CommonError) {
	if len(password) < 6 {
		return &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "PASSWORD_TOO_SHORT",
					Message:  "Password must be at least six characters",
				},
			},
		}
	}
	if len(password) > 72 {
		return &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "PASSWORD_TOO_LONG",
					Message:  "Password must be under 72 bytes",
				},
			},
		}
	}

	entropy := zxcvbn.PasswordStrength(password, nil)
	if entropy.Score < 2 {
		return &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "WEAK_PASSWORD",
					Message:  "Your password is weak",
				},
			},
		}
	}

	return nil
}
