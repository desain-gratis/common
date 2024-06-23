package password

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
)

type Repository interface {
	Validate(ctx context.Context, oidcIssuer, oidcSubject, password string) (ok bool, errUC *types.CommonError)
	Set(ctx context.Context, oidcIssuer, oidcSubject, password string) (errUC *types.CommonError)
	GetID(ctx context.Context, oidcIssuer, oidcSubject string) (id string, errUC *types.CommonError)
}
