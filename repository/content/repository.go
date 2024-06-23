package content

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	types "github.com/desain-gratis/common/types/http"
)

// IMPORTANT
//
// =================================================================
//
// ALL IMPLEMENTATION MUST ALWAYS PERFORM USER ID VERIFICATION
// FOR EACH OPERATION
//
// ==================================================================
//

// EACH IMPLEMENTATION CAN ACCOMODATE MULTIPLE 'TABLE' and form a group of table
// (not just each implementation have their own repository)
// for example, in the inmemory repository, we can create many repository implementation
// that represensts DB lock for multiple tables, etc...

// Basically, each repository is a complete DB modelling solution.
// Each implementation free to use Contraint to ensure data / reference integrity between each Data Type / Table.
// The repo here will not guarantee that. But only the deployment configuration in the db / in memory lock that do this.

// Stores your data in the internet and assign ID to resource so it can be located
// It stores the data using user ID
type Repository[T proto.Message] interface {
	// Store data with associated metadata
	// Metadata will be used for indexing & searching
	Put(ctx context.Context, userID string, data Data[T]) (Data[T], *types.CommonError)

	// Get daya by owner ID
	// If not exist, return empty result as success
	Get(ctx context.Context, userID string) ([]Data[T], *types.CommonError)

	// Delete specific ID data. If no data, MUST return error
	Delete(ctx context.Context, userID, ID string) (Data[T], *types.CommonError)

	// Get specific data by ID. If not exist, MUST return error
	GetByID(ctx context.Context, userID, ID string) (Data[T], *types.CommonError)

	// Main ref if the data is a dependent
	GetByMainRefID(ctx context.Context, userID, mainRefID string) ([]Data[T], *types.CommonError)
}

type Addressable interface {
	SetID(id string)
}

// Data is a wraper for proto  Message
// It is used to store information related to the datastore itself
// Such as indexing data like:
// ID (primary key), Main Ref ID (foreign key)
// An URL (if any)
// Update date
// Also some basic indexing such as last update date
type Data[T proto.Message] struct {
	// The location of the data in the repository
	ID string

	// If the data is dependent on / is part of another data,
	// We put the main reference ID here
	MainRefID string

	// The actual data
	Data T

	// The published URL
	URL string

	LastUpdate time.Time
}

// OptGetParam represent Optional Get Paramaater
// to be passed to repository when doing Get.
// Is it up to the implementation to support it or not.
// It is not required.
//
// I also just put this here just for future reminder
type OptGetParam struct {
	OrderByLastUpdate bool
}
