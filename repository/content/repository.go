package content

import (
	"context"
	"time"

	types "github.com/desain-gratis/common/types/http"
)

type Repository interface {
	Post(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError)

	// Get daya by owner ID
	Get(ctx context.Context, userID, ID string, refIDs []string) ([]Data, *types.CommonError)

	// Delete specific ID data. If no data, MUST return error
	Delete(ctx context.Context, userID, ID string, refIDs []string) (Data, *types.CommonError)
}

type Data struct {
	// The location of the data in the repository
	ID string

	// The actual data
	Data []byte

	LastUpdate time.Time

	// USED FOR SQL
	UserID string
	RefIDs []string
}
