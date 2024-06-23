package cql

import (
	"context"
	"net/http"
	"time"

	"github.com/keenangebze/gockle"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/limiter"
	types "github.com/desain-gratis/common/types/http"
)

const DEFAULT_TTL_SECONDS = 300

var _ limiter.Repository = &cqlHandler{}

type cqlHandler struct {
	session   gockle.Session
	tableName string
}

func New(session gockle.Session, tableName string) *cqlHandler {
	//  create table account.limiter (oidc_issuer text, oidc_subject text, id text, expired_at timestamp, count counter, primary key ((oidc_issuer, oidc_subject), id, expired_at)) with clustering order by (id ASC, expired_at desc);
	return &cqlHandler{
		session:   session,
		tableName: tableName,
	}
}

func (h *cqlHandler) Get(ctx context.Context, oidcIssuer, oidcSubject, id string) (counter int, expiredAt time.Time, errUC *types.CommonError) {
	query := h.session.Query(
		`SELECT expired_at, count FROM `+h.tableName+` 
			WHERE oidc_issuer= ? AND oidc_subject= ? AND id = ? LIMIT 1`,
		oidcIssuer,
		oidcSubject,
		id,
	)
	iter := query.Iter()

	_ = iter.Scan(&expiredAt, &counter)

	err := iter.Close()
	if err != nil {
		log.Err(err).Msgf("Failed to get limiter")
		return 0, time.Time{}, &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_GET_LIMITER", HTTPCode: http.StatusFailedDependency, Message: "Failed to get request limiter data"},
			},
		}
	}

	return counter, expiredAt, nil
}

func (h *cqlHandler) Increment(ctx context.Context, oidcIssuer, oidcSubject, id string, expiryAt time.Time) (errUC *types.CommonError) {
	ttl := int(expiryAt.Sub(time.Now()).Seconds())
	if ttl < 1 {
		ttl = DEFAULT_TTL_SECONDS
	}

	err := h.session.Exec(
		`UPDATE `+h.tableName+` USING TTL ? SET count = count + 1 WHERE oidc_issuer = ? AND oidc_subject = ? AND id = ? AND expired_at = ?`,
		ttl,
		oidcIssuer,
		oidcSubject,
		id,
		expiryAt,
	)
	if err != nil {
		log.Err(err).Msgf("Failed to increment limiter")
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_SET_LIMITER", HTTPCode: http.StatusFailedDependency, Message: "Failed to set limiter data"},
			},
		}
	}

	return nil
}

func (h *cqlHandler) Expire(ctx context.Context, oidcIssuer, oidcSubject, id string) (errUC *types.CommonError) {
	// 2 so that can expire slowly (make sure not get rounded to 0)
	err := h.session.Exec(
		`DELETE FROM `+h.tableName+` WHERE oidc_issuer = ? AND oidc_subject = ? AND id = ?`,
		oidcIssuer,
		oidcSubject,
		id,
	)
	if err != nil {
		log.Err(err).Msgf("Failed to delete limiter")
		return &types.CommonError{
			Errors: []types.Error{
				{Code: "FAILED_TO_SET_LIMITER", HTTPCode: http.StatusFailedDependency, Message: "Failed to delete limiter data"},
			},
		}
	}

	return nil
}
