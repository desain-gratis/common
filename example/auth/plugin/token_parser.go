package plugin

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"google.golang.org/protobuf/proto"
)

func ParseToken(ctx context.Context, payload []byte) (any, *types.CommonError) {
	var sess session.SessionData

	err := proto.Unmarshal(payload, &sess)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Message: "Error parsing token"},
			},
		}
	}

	return &sess, nil
}
