package account

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
)

type Usecase interface {
	PasswordManager
}

type PasswordManager interface {
	UpdatePassword(ctx context.Context, oidcIssuer string, oidcSubject string, newPassword string) (userID string, errUC *types.CommonError)
	PasswordExist(ctx context.Context, oidcIssuer string, oidcSubject string) (exist bool, errUC *types.CommonError)
}
