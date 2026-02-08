package clickhouseraft

import (
	"context"
	"encoding/json"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

var _ content.Repository = &repository{}

// TO BE USED INSIDE RAFT APPLICATION
type repository struct {
	base        *ContentApp
	tableConfig TableConfig
}

func (r *repository) Post(ctx context.Context, namespace string, refIDs []string, ID string, data content.Data) (content.Data, error) {
	result, err := r.base.post(ctx, DataWrapper{
		Table:     r.tableConfig.Name,
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
		Data:      data.Data, // raw json
		Meta:      data.Meta,
	})
	if err != nil {
		return content.Data{}, err
	}

	return content.Data{
		Namespace: result.Namespace,
		RefIDs:    result.RefIDs,
		ID:        result.ID,
		Data:      result.Data,
		Meta:      result.Meta,
		EventID:   result.EventID,
	}, nil
}

// Get daya by owner ID
func (r *repository) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]content.Data, error) {
	chanResp, err := r.base.queryMyContent(ctx, QueryMyContent{
		Table:     r.tableConfig.Name,
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
	})
	if err != nil {
		return nil, err
	}

	// for get, we store them all into memory
	result := make([]content.Data, 0)
	for data := range chanResp {
		result = append(result, *data)
	}

	return result, nil
}

// Delete specific ID data. If no data, MUST return error
func (r *repository) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (content.Data, error) {
	placeholder := json.RawMessage("{}")

	result, err := r.base.delete(ctx, DataWrapper{
		Table:     r.tableConfig.Name,
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
		Data:      placeholder,
		Meta:      placeholder,
	})
	if err != nil {
		return content.Data{}, err
	}

	return content.Data{
		EventID:   result.EventID,
		Namespace: result.Namespace,
		RefIDs:    result.RefIDs,
		ID:        result.ID,
		Data:      result.Data,
		Meta:      result.Meta,
	}, nil
}

// Stream Get data
func (r *repository) Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan content.Data, error) {
	chanResp, err := r.base.queryMyContent(ctx, QueryMyContent{
		Table:     r.tableConfig.Name,
		Namespace: namespace,
		RefIDs:    refIDs,
		ID:        ID,
	})
	if err != nil {
		return nil, err
	}

	// for get, we store them all into memory
	result := make(chan content.Data)
	go func() {
		defer close(result)

		for data := range chanResp {
			result <- *data
		}
	}()

	return result, nil
}
