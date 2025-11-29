package utility

import "encoding/json"

func ParseJsonAs[T any](payload json.RawMessage) (T, error) {
	var t T
	err := json.Unmarshal(payload, &t)
	return t, err
}

// RowOf represents table row
func RowOf[T any](tableName string, key string, value T) Row[T] {
	return Row[T]{
		TableName: tableName,
		Key:       key,
		Value:     value,
	}
}

type Row[T any] struct {
	TableName string `json:"table_name"`
	Key       string `json:"key"`   // grouping key of the row
	Value     T      `json:"value"` // row data
}
