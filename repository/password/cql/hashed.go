package cql

import (
	"context"
	"net/http"
	"strconv"

	"github.com/keenangebze/gockle"
	"golang.org/x/crypto/bcrypt"

	types "github.com/desain-gratis/common/types/http"
)

type hashedPasswordHandler struct {
	session   gockle.Session
	tableName string
}

func NewHashed(session gockle.Session, tableName string) *hashedPasswordHandler {
	return &hashedPasswordHandler{
		session:   session,
		tableName: tableName,
	}
}

func (h *hashedPasswordHandler) Validate(ctx context.Context, oidcIssuer, oidcSubject, password string) (ok bool, errUC *types.CommonError) {
	query := h.session.Query(
		`SELECT token(oidc_issuer, oidc_subject), password FROM `+h.tableName+` WHERE oidc_issuer = ? AND oidc_subject = ?`,
		oidcIssuer,
		oidcSubject,
	)
	iter := query.Iter()

	var token int64
	var passwordHash string
	_ = iter.Scan(&token, &passwordHash)

	err := iter.Close()
	if err != nil {
		return false, &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_GET_PASSWORD", HTTPCode: http.StatusFailedDependency, Message: "Failed to get password"},
			},
		}
	}

	if token == 0 || passwordHash == "" {
		return false, &types.CommonError{
			Errors: []types.Error{
				{Code: "PASSWORD_NOT_CONFIGURED", HTTPCode: http.StatusBadRequest, Message: "Password is not yet configured"},
			},
		}
	}

	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	if err != nil {
		return false, nil
	}

	return true, nil
}

func (h *hashedPasswordHandler) Set(ctx context.Context, oidcIssuer, oidcSubject, password string) (errUC *types.CommonError) {
	encrypted, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_HASH_PASSWORD", HTTPCode: http.StatusInternalServerError, Message: "Failed to set password"},
			},
		}
	}

	err = h.session.Exec(
		`UPDATE `+h.tableName+` SET password = ? WHERE oidc_issuer = ? AND oidc_subject = ?`,
		string(encrypted),
		oidcIssuer,
		oidcSubject,
	)
	if err != nil {
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_SET_PASSWORD", HTTPCode: http.StatusFailedDependency, Message: "Failed to set password"},
			},
		}
	}

	return nil
}

func (h *hashedPasswordHandler) GetID(ctx context.Context, oidcIssuer, oidcSubject string) (id string, errUC *types.CommonError) {
	query := h.session.Query(
		`SELECT token(oidc_issuer, oidc_subject) FROM `+h.tableName+` WHERE oidc_issuer = ? AND oidc_subject = ?`,
		oidcIssuer,
		oidcSubject,
	)
	iter := query.Iter()

	var token int64 // TODO: test
	_ = iter.Scan(&token)

	err := iter.Close()
	if err != nil {
		return "", &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_GET_PASSWORD", HTTPCode: http.StatusFailedDependency, Message: "Failed to get password"},
			},
		}
	}

	if token == 0 {
		return "", nil
	}

	// MIGHT BE WRONG -> Need to test and cast it to appropriate type (if greater than int64)
	return strconv.FormatInt(token, 10), nil
}
