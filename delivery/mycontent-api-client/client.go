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
		return result, fmt.Errorf("delete: invalid url: %w", err)
	}

	v := wer.Query()
	v.Add("id", ID)

	for param, value := range refIDs {
		v.Add(param, value)
	}

	wer.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodDelete, wer.String(), nil)
	if err != nil {
		return result, fmt.Errorf("delete: new request: %w", err)
	}

	req = req.WithContext(ctx)
	if authToken != "" {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}
	req.Header.Add("X-Namespace", namespace)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return result, fmt.Errorf("delete: do: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("delete: read body: %w", err)
	}

	var cr types.CommonResponseTyped[T]
	err = json.Unmarshal(body, &cr)
	if err != nil {
		return result, fmt.Errorf("delete: parse data from server: %w | %v", err, string(body))
	}

	if cr.Error != nil {
		return result, fmt.Errorf("delete: from server: %w", parseError(cr.Error))
	}

	return cr.Success, nil
}

func (c *client[T]) get(ctx context.Context, authToken string, namespace string, refIDs map[string]string, ID string) (result []T, errUC error) {
	wer, err := url.Parse(c.endpoint)
	if err != nil {
		return result, fmt.Errorf("get: %w", err)
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
		return result, fmt.Errorf("get: new request error: %w", err)
	}

	req = req.WithContext(ctx)

	if authToken != "" {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}
	req.Header.Add("X-Namespace", namespace)

	// sff udrt sorg

	resp, err := c.httpc.Do(req)
	if err != nil {
		return result, fmt.Errorf("get: do error: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("get: read body: %w", err)
	}
	if resp.StatusCode != 200 {
		var commer types.CommonResponseTyped[error]
		err = json.Unmarshal(body, &commer)
		if err != nil {
			return result, fmt.Errorf("get: parse data from server: %w | %v", err, string(body))
		}

		return result, parseError(commer.Error)
	}

	var cr types.CommonResponseTyped[[]T]

	err = json.Unmarshal(body, &cr)
	if err != nil {
		return result, fmt.Errorf("get: parse server response: %w", err)
	}

	if cr.Error != nil {
		return result, fmt.Errorf("get: from server: %w", parseError(cr.Error))
	}

	return cr.Success, nil
}

func (c *client[T]) post(ctx context.Context, authToken string, data T) (result T, errUC error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return result, fmt.Errorf("post: marshal data: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, fmt.Errorf("post: new request: %w", err)
	}

	req = req.WithContext(ctx)

	if authToken != "" {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}

	req.Header.Add("X-Namespace", data.Namespace())

	resp, err := c.httpc.Do(req)
	if err != nil {
		return result, fmt.Errorf("post: do: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("post: read body: %w", err)
	}

	var cr types.CommonResponseTyped[T]
	err = json.Unmarshal(body, &cr)
	if err != nil {
		return result, fmt.Errorf("post: parse data from server: %w | %v", err, string(body))
	}

	if cr.Error != nil {
		return result, fmt.Errorf("post: from server: %w", parseError(cr.Error))
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

func parseError(err *types.CommonError) error {
	if err == nil || err.Errors == nil {
		return nil
	}
	var errs error
	for _, err := range err.Errors {
		errs = errors.Join(errs, fmt.Errorf("%v: %v", err.Code, err.Message))
	}

	return errs
}
