package helper

import (
	"fmt"
	"net/http"
	"net/url"

	types "github.com/desain-gratis/common/types/http"
)

func SetError(w http.ResponseWriter, body types.Error, code int) {
	errMessage := types.SerializeError(&types.CommonError{
		Errors: []types.Error{body},
	})
	w.WriteHeader(code)
	w.Write(errMessage)
}

func CheckRequiredFields(form url.Values, requiredFields []string) *types.Error {
	for _, key := range requiredFields {
		if len(form[key]) == 0 {
			err := types.Error{Message: fmt.Sprintf("Please fill `%s` field", key), Code: "EMPTY_REQUIRED_FIELD"}
			return &err
		}
	}

	return nil
}
