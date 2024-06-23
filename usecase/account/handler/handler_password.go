package handler

import (
	"context"

	"github.com/desain-gratis/common/repository/password"
	types "github.com/desain-gratis/common/types/http"
)

type passwordHandler struct {
	repo password.Repository
}

func NewPasswordHandler(repo password.Repository) *passwordHandler {
	return &passwordHandler{
		repo: repo,
	}
}

func (p *passwordHandler) UpdatePassword(ctx context.Context, oidcIssuer string, oidcSubject string, newPassword string) (userID string, errUC *types.CommonError) {
	errUC = p.repo.Set(ctx, oidcIssuer, oidcSubject, newPassword)
	if errUC != nil {
		// todo: communicate error
		return "", errUC
	}

	userID, errUC = p.repo.GetID(ctx, oidcIssuer, oidcSubject)
	if errUC != nil {
		// todo: communicate error
		return userID, errUC
	}

	return userID, nil
}

func (p *passwordHandler) PasswordExist(ctx context.Context, oidcIssuer string, oidcSubject string) (exist bool, errUC *types.CommonError) {
	userID, errUC := p.repo.GetID(ctx, oidcIssuer, oidcSubject)
	if errUC != nil {
		// todo: communicate error
		return false, errUC
	}

	return userID != "", nil
}
