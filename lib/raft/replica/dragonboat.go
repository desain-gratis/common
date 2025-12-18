package replica

import (
	"strconv"
)

func convertID(x string) (uint64, error) {
	id, err := strconv.Atoi(x)
	if err != nil {
		return 0, err
	}

	return uint64(id), nil
}
