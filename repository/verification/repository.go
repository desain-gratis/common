package verification

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
)

type Repository interface {
	Get(ctx context.Context, oidcIssuer, oidcSubject string) (payload []byte, errUC *types.CommonError)
	Set(ctx context.Context, oidcIssuer, oidcSubject string, payload []byte) (errUC *types.CommonError)
}
