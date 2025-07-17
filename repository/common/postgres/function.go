package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	content "github.com/desain-gratis/common/delivery/mycontent-api"
)

func (r repo[T]) Get(ctx context.Context, organizationID, id string, refID []string) (data []T, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.timeoutMs)*time.Millisecond)
	defer cancel()

	responses, errGet := r.client.Get(ctx, organizationID, refID, id)
	if errGet != nil {
		if len(errGet.Errors) > 0 {
			err = fmt.Errorf("failed to get: %s", errGet.Errors[0].Message)
		}
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

	_, errInsert := r.client.Post(ctx, organizationID, refID, id, postData)
	if errInsert != nil {
		if len(errInsert.Errors) > 0 {
			err = fmt.Errorf("failed to insert: %s", errInsert.Errors[0].Message)
		}
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

	_, errUpdate := r.client.Post(ctx, organizationID, refID, id, updateData)
	if errUpdate != nil {
		if len(errUpdate.Errors) > 0 {
			err = fmt.Errorf("failed to update: %s", errUpdate.Errors[0].Message)
		}
	}

	return
}

func (r *repo[T]) Delete(ctx context.Context, organizationID, id string, refID []string) (err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.timeoutMs)*time.Millisecond)
	defer cancel()

	_, errDelete := r.client.Delete(ctx, organizationID, refID, id)
	if errDelete != nil {
		if len(errDelete.Errors) > 0 {
			err = fmt.Errorf("failed to delete: %s", errDelete.Errors[0].Message)
		}
	}

	return
}
