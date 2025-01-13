package mycontent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

type client[T mycontent.Data] struct {
	endpoint string
	token    string
	tenantID string
	userID   string

	httpc *http.Client
}

func New[T mycontent.Data](
	httpc *http.Client,
	endpoint string,
	tenantID string,
	userID string,
) *client[T] {
	return &client[T]{
		httpc:    httpc,
		endpoint: endpoint,
		tenantID: tenantID,
		userID:   userID,
	}
}

func (c *client[T]) WithAuthorization(token string) {
	c.token = token
}

func (c *client[T]) Delete(ctx context.Context, ownerID string, refIDs map[string]string, ID string) (result T, errUC *types.CommonError) {
	wer, err := url.Parse(c.endpoint)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "invalid URL " + err.Error()},
			},
		}
	}

	v := wer.Query()
	v.Add("owner_id", c.userID)
	v.Add("id", ID)

	for param, value := range refIDs {
		v.Add(param, value)
	}

	wer.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodDelete, wer.String(), nil)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "new request " + err.Error()},
			},
		}
	}

	req = req.WithContext(ctx)
	req.Header.Add("Authorization", "Bearer "+c.token)
	req.Header.Add("X-User-Id", c.userID)
	req.Header.Add("X-Tenant-Id", c.tenantID)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "do " + err.Error()},
			},
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "read body " + err.Error()},
			},
		}
	}

	var cr types.CommonResponseTyped[T]

	err = json.Unmarshal(body, &cr)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "unmarshal" + err.Error()},
			},
		}
	}

	if cr.Error != nil {
		return result, cr.Error
	}

	return cr.Success, nil
}

func (c *client[T]) Get(ctx context.Context, ownerID string, refIDs map[string]string, ID string) (result []T, errUC *types.CommonError) {
	wer, err := url.Parse(c.endpoint)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "invalid URL " + err.Error()},
			},
		}
	}

	v := wer.Query()
	v.Add("owner_id", ownerID)
	v.Add("id", ID)

	for param, value := range refIDs {
		v.Add(param, value)
	}

	wer.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, wer.String(), nil)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "new request " + err.Error()},
			},
		}
	}

	req = req.WithContext(ctx)

	req.Header.Add("Authorization", "Bearer "+c.token)
	req.Header.Add("X-User-Id", c.userID)
	req.Header.Add("X-Tenant-Id", c.tenantID)

	// sff udrt sorg

	resp, err := c.httpc.Do(req)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "do " + err.Error()},
			},
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "read body " + err.Error()},
			},
		}
	}
	if resp.StatusCode > 200 {
		var commer types.CommonResponseTyped[*types.CommonError]
		_ = json.Unmarshal(body, &commer)
		return result, commer.Error
	}

	var cr types.CommonResponseTyped[[]T]

	err = json.Unmarshal(body, &cr)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "unmarshal" + err.Error() + string(body)},
			},
		}
	}

	if cr.Error != nil {
		return result, cr.Error
	}

	return cr.Success, nil

}

func (c *client[T]) Put(ctx context.Context, data T) (result T, errUC *types.CommonError) {
	var t T
	payload, err := json.Marshal(data)
	if err != nil {
		return t, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "new request " + err.Error()},
			},
		}
	}

	req, err := http.NewRequest(http.MethodPut, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return t, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "new request " + err.Error()},
			},
		}
	}

	req = req.WithContext(ctx)

	req.Header.Add("Authorization", "Bearer "+c.token)
	req.Header.Add("X-User-Id", c.userID)
	req.Header.Add("X-Tenant-Id", c.tenantID)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return t, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "do " + err.Error()},
			},
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return t, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "read body " + err.Error()},
			},
		}
	}

	var cr types.CommonResponseTyped[T]
	err = json.Unmarshal(body, &cr)
	if err != nil {
		return t, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "unmarshal" + err.Error()},
			},
		}
	}

	if cr.Error != nil {
		return t, cr.Error
	}

	return cr.Success, nil
}
