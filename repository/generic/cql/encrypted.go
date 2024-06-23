package cql

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/keenangebze/gockle"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/generic"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/utility/encryption"
	"github.com/desain-gratis/common/utility/secretkv"
)

var _ generic.OIDCRepository = &encryptedHandler{}

type encryptedHandler struct {
	*plaintextHandler
	ecryptionKeyID string
	secretProvider secretkv.Provider
}

func NewEcrypted(
	session gockle.Session,
	secretProvider secretkv.Provider,
	ecryptionKeyID string,
	tableName string,
) *encryptedHandler {
	return &encryptedHandler{
		plaintextHandler: &plaintextHandler{
			session:   session,
			tableName: tableName,
		},
		secretProvider: secretProvider,
		ecryptionKeyID: ecryptionKeyID,
	}
}

func (h *encryptedHandler) Get(ctx context.Context, oidcIssuer, oidcSubject string) (payload []byte, meta *generic.Meta, errUC *types.CommonError) {
	data, metadata, errUC := h.get(ctx, oidcIssuer, oidcSubject)
	if errUC != nil {
		return nil, nil, errUC
	}

	// If empty return success, but empty
	if data == "" {
		return nil, nil, nil
	}

	secret, err := h.secretProvider.Get(ctx, metadata.KeyID, metadata.KeyVersion)
	if err != nil {
		log.Err(err).Msgf("Failed to get encryption key")
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Server error"},
			},
		}
	}

	// base64 decode
	// so it can be export/import (eg. as CSV) easily if it is a printable string, rather than pure blob
	// since performance is not crucial
	payload, err = base64.RawStdEncoding.DecodeString(data)
	if err != nil {
		log.Err(err).Msgf("Failed to decode payload as bytes")
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Server error"},
			},
		}
	}

	iv, err := base64.RawStdEncoding.DecodeString(metadata.IV)
	if err != nil {
		log.Err(err).Msgf("Failed to decode iv as bytes")
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Server error"},
			},
		}
	}

	result, errUC := encryption.Decrypt(secret.Payload, iv, payload)
	if errUC != nil {
		return nil, nil, errUC
	}

	return result, &generic.Meta{
		ID:        metadata.ID,
		WriteTime: metadata.WriteTime,
	}, nil
}

func (h *encryptedHandler) Set(ctx context.Context, oidcIssuer, oidcSubject string, payload []byte) (errUC *types.CommonError) {
	key, err := h.secretProvider.Get(ctx, h.ecryptionKeyID, 0)
	if err != nil {
		log.Err(err).Msgf("Failed to get encryption key")
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Server error"},
			},
		}
	}

	payload, iv, errUC := encryption.Encrypt(key.Payload, payload)
	if errUC != nil {
		return errUC
	}

	// Encode to base64 string to make it printable
	// So it will be easy to do export to CSV in the database and other operations (eg. backup)
	encodedPayload := base64.RawStdEncoding.EncodeToString(payload)
	encodedIV := base64.RawStdEncoding.EncodeToString(iv)

	return h.set(ctx, oidcIssuer, oidcSubject, encodedPayload, &meta{KeyID: key.Key, KeyVersion: key.Version, IV: encodedIV})
}
