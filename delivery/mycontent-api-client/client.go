package mycontentapiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	types "github.com/desain-gratis/common/types/http"
)

// TODO: follow this interface
var _ mycontent.Usecase[mycontent.Data] = &client[mycontent.Data]{}

type client[T mycontent.Data] struct {
	endpoint  string
	refsParam []string
	authToken string

	httpc *http.Client
}

func New[T mycontent.Data](
	httpc *http.Client,
	endpoint string,
	refsParam []string,
	authToken string,
) *client[T] {
	return &client[T]{
		httpc:     httpc,
		endpoint:  endpoint,
		refsParam: refsParam, // TODO: ADD validation immediately to improve DevX
		authToken: authToken,
	}
}

// Post (create new or overwrite) resource here
func (c *client[T]) Post(ctx context.Context, data T, _ any) (T, error) {
	// for client, meta is ignored
	return c.post(ctx, c.authToken, data)
}

// Get all of your resource for your user ID here
func (c *client[T]) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]T, error) {
	params, err := toRefsParamGet(c.refsParam, refIDs)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}

	return c.get(ctx, c.authToken, namespace, params, ID)
}

// Stream response
func (c *client[T]) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan T, error) {
	// params := map[string]string{}
	// for idx := range refIDs {
	// 	key := c.refsParam[idx]
	// 	value := refIDs[idx]
	// 	params[key] = value
	// }
	// TODO
	return nil, errors.New("not implemented yet") // TODO
}

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (c *client[T]) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (T, error) {
	params, err := toRefsParam(c.refsParam, refIDs)
	if err != nil {
		var t T
		return t, fmt.Errorf("delete: %w", err)
	}

	return c.delete(ctx, c.authToken, namespace, params, ID)
}

func (c *client[T]) delete(ctx context.Context, authToken string, namespace string, refIDs map[string]string, ID string) (result T, errUC error) {
	wer, err := url.Parse(c.endpoint)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "invalid URL " + err.Error()},
			},
		}
	}

	v := wer.Query()
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
	if authToken != "" {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}
	req.Header.Add("X-Namespace", namespace)

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

func (c *client[T]) get(ctx context.Context, authToken string, namespace string, refIDs map[string]string, ID string) (result []T, errUC error) {
	wer, err := url.Parse(c.endpoint)
	if err != nil {
		return result, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "invalid URL " + err.Error()},
			},
		}
	}

	v := wer.Query()
	if ID != "" {
		v.Add("id", ID)
	}

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

	if authToken != "" {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}
	req.Header.Add("X-Namespace", namespace)

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
		var commer types.CommonResponseTyped[error]
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

func (c *client[T]) post(ctx context.Context, authToken string, data T) (result T, errUC error) {
	var t T
	payload, err := json.Marshal(data)
	if err != nil {
		return t, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "new request " + err.Error()},
			},
		}
	}

	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return t, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "new request " + err.Error()},
			},
		}
	}

	req = req.WithContext(ctx)

	if authToken != "" {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}

	req.Header.Add("X-Namespace", data.Namespace())

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

func toRefsParam(refsParam []string, refIDs []string) (map[string]string, error) {
	if len(refsParam) != len(refIDs) {
		return nil, fmt.Errorf("Parameter not matching! expected: %v got: %v", refsParam, refIDs)
	}
	result := make(map[string]string, len(refsParam))
	for i := range refIDs {
		result[refsParam[i]] = refIDs[i]
	}

	return result, nil
}

func toRefsParamGet(refsParam []string, refIDs []string) (map[string]string, error) {
	result := make(map[string]string, len(refsParam))
	for i := range refIDs {
		result[refsParam[i]] = refIDs[i]
	}

	return result, nil
}
