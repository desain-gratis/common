package generic

import (
	"context"
	"time"

	types "github.com/desain-gratis/common/types/http"
)

type OIDCRepository interface {
	Get(ctx context.Context, oidcIssuer, oidcSubject string) (payload []byte, meta *Meta, errUC *types.CommonError)
	Set(ctx context.Context, oidcIssuer, oidcSubject string, payload []byte) (errUC *types.CommonError)
}

type Meta struct {
	ID        string
	WriteTime time.Time
}
