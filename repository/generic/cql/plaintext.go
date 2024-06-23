package cql

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/keenangebze/gockle"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/generic"
	types "github.com/desain-gratis/common/types/http"
)

var _ generic.OIDCRepository = &plaintextHandler{}

type meta struct {
	ID         string
	WriteTime  time.Time
	KeyID      string
	KeyVersion int
	IV         string
}

type plaintextHandler struct {
	session   gockle.Session
	tableName string
}

func New(
	session gockle.Session,
	tableName string,
) *plaintextHandler {
	return &plaintextHandler{
		session:   session,
		tableName: tableName,
	}
}

// common get
func (h *plaintextHandler) get(ctx context.Context, oidcIssuer, oidcSubject string) (payload string, metadata *meta, errUC *types.CommonError) {
	query := h.session.Query(
		`SELECT token(oidc_issuer, oidc_subject), payload, key_id, key_version, iv, writetime(payload) FROM `+h.tableName+` 
			WHERE oidc_issuer = ? AND oidc_subject = ? LIMIT 1`,
		oidcIssuer,
		oidcSubject,
	)
	iter := query.Iter()

	var id int64
	var _payload, iv, keyID string
	var keyVersion int
	var _writeTime int64
	_ = iter.Scan(&id, &_payload, &keyID, &keyVersion, &iv, &_writeTime)
	err := iter.Close()
	if err != nil {
		log.Err(err).Msgf("Failed to get data")
		return "", nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_GET_DATA", HTTPCode: http.StatusFailedDependency, Message: "Failed to get data"},
			},
		}
	}

	writeTime := time.Unix(_writeTime/1000000, 0)
	return _payload, &meta{
		ID:         strconv.FormatInt(id, 10),
		WriteTime:  writeTime,
		KeyID:      keyID,
		KeyVersion: keyVersion,
		IV:         iv,
	}, nil
}

func (h *plaintextHandler) set(ctx context.Context, oidcIssuer, oidcSubject, payload string, optMeta *meta) (errUC *types.CommonError) {
	var err error
	if optMeta != nil {
		err = h.session.Exec(
			`UPDATE `+h.tableName+` SET payload = ?, key_id = ?, key_version = ?, iv = ? WHERE oidc_issuer = ? AND oidc_subject = ?`,
			payload,
			optMeta.KeyID,
			optMeta.KeyVersion,
			optMeta.IV,
			oidcIssuer,
			oidcSubject,
		)
	} else {
		err = h.session.Exec(
			`UPDATE `+h.tableName+` SET payload = ? WHERE oidc_issuer = ? AND oidc_subject = ?`,
			payload,
			oidcIssuer,
			oidcSubject,
		)
	}

	if err != nil {
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusFailedDependency, Message: "Failed set data"},
			},
		}
	}

	return nil
}

func (h *plaintextHandler) Get(ctx context.Context, oidcIssuer, oidcSubject string) (payload []byte, meta *generic.Meta, errUC *types.CommonError) {
	data, metadata, errUC := h.get(ctx, oidcIssuer, oidcSubject)
	if errUC != nil {
		return nil, nil, errUC
	}

	// If empty return success, but empty
	if data == "" {
		return nil, nil, nil
	}

	return []byte(data), &generic.Meta{
		ID:        metadata.ID,
		WriteTime: meta.WriteTime,
	}, nil
}

func (h *plaintextHandler) Set(ctx context.Context, oidcIssuer, oidcSubject string, payload []byte) (errUC *types.CommonError) {
	return h.set(ctx, oidcIssuer, oidcSubject, string(payload), nil)
}
