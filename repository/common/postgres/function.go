package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

func (r repo[T]) Get(ctx context.Context, organizationID, id string, refID []string) (data []T, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.timeoutMs)*time.Millisecond)
	defer cancel()

	responses, err := r.client.Get(ctx, organizationID, refID, id)
	if err != nil {
		err = fmt.Errorf("failed to get: %s", err)
		return
	}

	for _, resp := range responses {
		var datum T
		datum, err = unmarshalData[T](resp.Data)
		if err != nil {
			return
		}
		data = append(data, datum)
	}

	return
}

func (r *repo[T]) Insert(ctx context.Context, organizationID, id string, refID []string, data T) (err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.timeoutMs)*time.Millisecond)
	defer cancel()

	payload, err := json.Marshal(data)
	if err != nil {
		return
	}

	postData := content.Data{
		Data: payload,
	}

	_, err = r.client.Post(ctx, organizationID, refID, id, postData)
	if err != nil {
		err = fmt.Errorf("failed to insert: %s", err)
		return
	}

	return
}

func (r *repo[T]) Update(ctx context.Context, organizationID, id string, refID []string, data T) (err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.timeoutMs)*time.Millisecond)
	defer cancel()

	payload, err := json.Marshal(data)
	if err != nil {
		return
	}

	updateData := content.Data{
		Data: payload,
	}

	_, err = r.client.Post(ctx, organizationID, refID, id, updateData)
	if err != nil {
		err = fmt.Errorf("failed to update: %s", err)
		return
	}

	return
}

func (r *repo[T]) Delete(ctx context.Context, organizationID, id string, refID []string) (err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.timeoutMs)*time.Millisecond)
	defer cancel()

	_, err = r.client.Delete(ctx, organizationID, refID, id)
	if err != nil {
		err = fmt.Errorf("failed to delete: %s", err)
		return
	}

	return
}
