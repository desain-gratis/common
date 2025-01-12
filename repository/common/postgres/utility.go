package postgres

import (
	"encoding/json"
)

func Parse[T any](in []byte) (T, error) {
	var t T
	err := json.Unmarshal(in, &t)
	if err != nil {
		return t, err
	}
	return t, nil
}

func unmarshalData[T any](in []byte) (out T, err error) {
	// var currentType T
	// switch any(currentType).(type) {
	// case m.AuthorizedUser:
	// 	row := m.AuthorizedUser{}
	// 	err = json.Unmarshal([]byte(in), &row)
	// 	if err != nil {
	// 		return
	// 	}
	// 	out = any(row).(T)
	// case m.UserGroup:
	// 	row := m.UserGroup{}
	// 	err = json.Unmarshal([]byte(in), &row)
	// 	if err != nil {
	// 		return
	// 	}
	// 	out = any(row).(T)

	// default:
	return Parse[T](in)
	// err = fmt.Errorf("unsupported type: %s", reflect.TypeOf(currentType))
	// }
	// return
}
