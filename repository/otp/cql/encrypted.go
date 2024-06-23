package cql

import (
	"context"
	"net/http"
	"time"

	"github.com/keenangebze/gockle"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/otp"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/utility/encryption"
	"github.com/desain-gratis/common/utility/secretkv"
)

var _ otp.Repository = &otpStateHandler{}

type otpStateHandler struct {
	session        gockle.Session
	secretProvider secretkv.Provider
	keyID          string
	tableName      string
}

func NewEcrypted(session gockle.Session, secretProvider secretkv.Provider, keyID, tableName string) *otpStateHandler {
	return &otpStateHandler{
		session:        session,
		secretProvider: secretProvider,
		keyID:          keyID,
		tableName:      tableName,
	}
}

func (h *otpStateHandler) Get(ctx context.Context, oidcIssuer, oidcSubject string) (payload []byte, expiredAt time.Time, errUC *types.CommonError) {
	query := h.session.Query(
		`SELECT state, key_id, key_version, iv, expired_at FROM `+h.tableName+` WHERE oidc_issuer = ? AND oidc_subject = ?`,
		oidcIssuer,
		oidcSubject,
	)
	iter := query.Iter()

	var state []byte
	var keyID string
	var keyVersion int
	var iv []byte

	_ = iter.Scan(&state, &keyID, &keyVersion, &iv, &expiredAt)

	err := iter.Close()
	if err != nil {
		log.Err(err).Msgf("Failed to get otp state")
		return nil, time.Time{}, &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_GET_OTP", HTTPCode: http.StatusFailedDependency, Message: "Failed to get OTP Code"},
			},
		}
	}

	if len(state) == 0 {
		return nil, time.Time{}, nil
	}

	// purposely use h.keyID
	key, err := h.secretProvider.Get(ctx, h.keyID, keyVersion)
	if err != nil {
		log.Err(err).Msgf("Failed to get encryption key")
		return nil, time.Time{}, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusFailedDependency, Message: "Server error"},
			},
		}
	}

	result, errUC := encryption.Decrypt(key.Payload, iv, state)
	if errUC != nil {
		return nil, time.Time{}, errUC
	}

	return result, expiredAt, nil
}

func (h *otpStateHandler) Set(ctx context.Context, oidcIssuer, oidcSubject string, payload []byte, expiredAt time.Time) (errUC *types.CommonError) {
	key, err := h.secretProvider.Get(ctx, h.keyID, 0)
	if err != nil {
		log.Err(err).Msgf("Failed to get encryption key")
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Server error"},
			},
		}
	}

	encryptedPayload, iv, errUC := encryption.Encrypt(key.Payload, payload)
	if errUC != nil {
		return errUC
	}

	ttlSec := expiredAt.Sub(time.Now())
	if ttlSec <= 0 {
		log.Error().Msgf("Invalid TTL value")
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Server error"},
			},
		}
	}

	err = h.session.Exec(
		`UPDATE `+h.tableName+` USING TTL ? SET state = ?, key_id = ?, key_version = ?, iv = ?, expired_at = ? WHERE oidc_issuer = ? AND oidc_subject = ?`,
		int64(ttlSec.Seconds()),
		encryptedPayload,
		key.Key,
		key.Version,
		iv,
		expiredAt,
		oidcIssuer,
		oidcSubject,
	)
	if err != nil {
		log.Err(err).Msgf("Err update otp data")
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusFailedDependency, Message: "Server error"},
			},
		}
	}

	return nil
}

func (h *otpStateHandler) Expire(ctx context.Context, oidcIssuer, oidcSubject string) (errUC *types.CommonError) {
	err := h.session.Exec(
		`DELETE FROM `+h.tableName+` WHERE oidc_issuer = ? AND oidc_subject = ?`,
		oidcIssuer,
		oidcSubject,
	)
	if err != nil {
		log.Err(err).Msgf("Err delete otp data")
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusFailedDependency, Message: "Server error"},
			},
		}
	}
	return nil
}
