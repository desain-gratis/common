package plugin

import (
	"context"
	"io"
	"net/http"

	common_entity "github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
	mycontent_base "github.com/desain-gratis/common/usecase/mycontent/base"
)

var _ mycontent.Usecase[*common_entity.Attachment] = &mcAttachAuth{}

type mcAttachAuth struct {
	*mycontent_base.HandlerWithAttachment
}

func MyContentAttachmentWithAuth(base *mycontent_base.HandlerWithAttachment) *mcAttachAuth {
	return &mcAttachAuth{
		HandlerWithAttachment: base,
	}
}

func (a *mcAttachAuth) Post(ctx context.Context, data *common_entity.Attachment, meta any) (*common_entity.Attachment, *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil {
		return data, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "EMPTY_AUTHORIZATION",
					Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
				},
			},
		}
	}

	// verify namespace
	err := verifyNamespace(auth, data.Namespace())
	if err != nil {
		return data, err
	}

	// you can get existing first to check the "permission"

	return a.Handler.Post(ctx, data, meta)
}

// Get all of your resource for your user ID here
func (a *mcAttachAuth) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]*common_entity.Attachment, *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "EMPTY_AUTHORIZATION",
					Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
				},
			},
		}
	}

	// verify namespace
	err := verifyNamespace(auth, namespace)
	if err != nil {
		return nil, err
	}

	return a.Handler.Get(ctx, namespace, refIDs, ID)

	// you can also filter result based on each get result afterward based on "permission"
}

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (a *mcAttachAuth) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (*common_entity.Attachment, *types.CommonError) {
	var data *common_entity.Attachment

	auth := getAuth(ctx)
	if auth == nil {
		return data, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "EMPTY_AUTHORIZATION",
					Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
				},
			},
		}
	}

	// verify namespace
	err := verifyNamespace(auth, namespace)
	if err != nil {
		return data, err
	}

	// you can get existing first to check the "permission"

	return a.Handler.Delete(ctx, namespace, refIDs, ID)
}

func (a *mcAttachAuth) Attach(ctx context.Context, meta *common_entity.Attachment, payload io.Reader) (*common_entity.Attachment, *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil {
		return meta, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "EMPTY_AUTHORIZATION",
					Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
				},
			},
		}
	}

	// verify namespace
	err := verifyNamespace(auth, meta.Namespace())
	if err != nil {
		return meta, err
	}

	return a.HandlerWithAttachment.Attach(ctx, meta, payload)
}

func (a *mcAttachAuth) GetAttachment(ctx context.Context, userID string, refIDs []string, ID string) (payload io.ReadCloser, meta *common_entity.Attachment, err *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil {
		return nil, meta, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "EMPTY_AUTHORIZATION",
					Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
				},
			},
		}
	}

	// verify namespace
	err = verifyNamespace(auth, meta.Namespace())
	if err != nil {
		return nil, meta, err
	}

	return a.HandlerWithAttachment.GetAttachment(ctx, userID, refIDs, ID)
}
