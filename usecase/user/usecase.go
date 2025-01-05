package user

import "context"

type UseCase interface {
	GetByOrgID(ctx context.Context, organizationID, nameContained string) ([]Details, error)
	GetDetail(ctx context.Context, email string) (Details, error)
	Insert(ctx context.Context, payload Payload) error
	Update(ctx context.Context, email string, payload Payload) error
	Delete(ctx context.Context, email string) error
}
