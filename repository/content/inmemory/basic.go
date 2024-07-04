package inmemory

import (
	"bytes"
	"context"
	"encoding/gob"
	"net/http"
	"sort"
	"strconv"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/content"
	types "github.com/desain-gratis/common/types/http"
)

var _ content.Repository[any] = &handler[any]{}

// limitation can only handle int
type handler[T any] struct {
	mtx     *sync.Mutex
	counter int

	indexByUserID     map[string]map[string]struct{}
	data              map[string]content.Data[T]
	enableOverwriteID bool
}

// To emulate DB also, can make this global
//
// New spawns new "generic" DB to store things online
// But this one does not provide any mechanism to do referential integritiy with other table
//
// enableOverwriteID is for entity that depend on another entity ID to live
// it allows to PUT using ID even the current user don't have it.
// as long is the same as the another entity ID
func New[T any](enableOverwriteID bool) *handler[T] {
	return &handler[T]{
		mtx:               &sync.Mutex{},
		indexByUserID:     make(map[string]map[string]struct{}),
		data:              make(map[string]content.Data[T], 0),
		enableOverwriteID: enableOverwriteID,
	}
}

func (h *handler[T]) Put(ctx context.Context, userID string, data content.Data[T]) (content.Data[T], *types.CommonError) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	var result content.Data[T]

	// Check whether this was a overwrite operation
	if data.ID != "" {
		byuserID, ok := h.indexByUserID[userID]
		if !ok && !h.enableOverwriteID {
			// short circuit not found
			return content.Data[T]{}, &types.CommonError{
				Errors: []types.Error{
					{
						Code:     "NOT_FOUND",
						HTTPCode: http.StatusNotFound,
						Message:  "You specify item ID, but the specified ID is not found.",
					},
				},
			}
		}
		_, ok = byuserID[data.ID]
		if ok {
			// user ID validation is on the usecase, not here

			// let's see whether this will work or not

			// PREVIOUSLY USE PROTO
			// copied, ok := proto.Clone(data.Data).(T)
			copied, ok := copyData(data.Data)
			if !ok {
				log.Fatal().Msgf("HEHE cannot copy %+v", copied)
			}
			data.Data = copied

			h.data[data.ID] = data

			return h.data[data.ID], nil
		}

		if h.enableOverwriteID {
			// Create new

			copied, ok := copyData(data.Data)
			if !ok {
				// TODO not fatal
				log.Fatal().Msgf("%+v", data)
			}

			data.Data = copied
			h.data[data.ID] = data

			// 3. reindex by user ID
			if _, ok := h.indexByUserID[userID]; !ok {
				h.indexByUserID[userID] = make(map[string]struct{})
			}
			h.indexByUserID[userID][data.ID] = struct{}{}

			return h.data[data.ID], nil
		}

		return result, &types.CommonError{
			Errors: []types.Error{
				{
					Code:     "NOT_FOUND",
					HTTPCode: http.StatusNotFound,
					Message:  "You specify item ID, but the specified ID is not found.",
				},
			},
		}
	}

	// Create new

	copied, ok := copyData(data.Data)
	if !ok {
		// TODO not fatal
		log.Fatal().Msgf("%+v", data)
	}

	data.Data = copied

	availableID := ""
	for {
		h.counter++
		availableID = strconv.Itoa(h.counter)
		_, ok := h.data[availableID]
		if !ok {
			break
		}
	}

	data.ID = availableID
	h.data[availableID] = data

	// 3. reindex by user ID
	if _, ok := h.indexByUserID[userID]; !ok {
		h.indexByUserID[userID] = make(map[string]struct{})
	}
	h.indexByUserID[userID][data.ID] = struct{}{}

	return h.data[data.ID], nil
}

func (h *handler[T]) Get(ctx context.Context, userID string) ([]content.Data[T], *types.CommonError) {
	ids := h.indexByUserID[userID]

	idsarr := make([]string, 0, len(ids))
	for k := range ids {
		idsarr = append(idsarr, k)
	}

	result := make([]content.Data[T], 0, len(ids))
	for _, id := range idsarr {
		item := h.data[id]
		copied, _ := copyData(item.Data)
		item.Data = copied
		result = append(result, item)
	}

	sort.Slice(result, func(a int, b int) bool {
		return result[a].LastUpdate.After(result[b].LastUpdate)
	})

	return result, nil
}

func (h *handler[T]) Delete(ctx context.Context, userID, ID string) (content.Data[T], *types.CommonError) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	ids, ok := h.indexByUserID[userID]
	if !ok {
		// not found
		return content.Data[T]{}, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusNotFound,
					Code:     "NOT_FOUND",
					Message:  "Delete failed. Data does not exist",
				},
			},
		}
	}

	if _, ok := ids[ID]; !ok {
		return content.Data[T]{}, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusNotFound,
					Code:     "NOT_FOUND",
					Message:  "Delete failed. Data does not exist",
				},
			},
		}
	}

	data := h.data[ID]

	delete(h.indexByUserID[userID], ID)
	delete(h.data, ID)

	return data, nil
}

func (h *handler[T]) GetByID(ctx context.Context, userID, ID string) (content.Data[T], *types.CommonError) {
	ids, ok := h.indexByUserID[userID]
	if !ok {
		// not found
		return content.Data[T]{}, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusNotFound,
					Code:     "NOT_FOUND",
					Message:  "ID Not found",
				},
			},
		}
	}

	if ID == "" {
		return content.Data[T]{}, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "BAD_REQUEST",
					Message:  "Not a valid ID. ID is empty!",
				},
			},
		}
	}

	if _, ok := ids[ID]; !ok {
		return content.Data[T]{}, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusNotFound,
					Code:     "NOT_FOUND",
					Message:  "You don't have any data that have the ID '" + ID + "'",
				},
			},
		}
	}

	result := h.data[ID]

	copied, _ := copyData(result.Data)
	result.Data = copied

	return result, nil
}

func (w *handler[T]) GetByMainRefID(ctx context.Context, userID, mainRefID string) ([]content.Data[T], *types.CommonError) {
	all, err := w.Get(ctx, userID)
	if err != nil || len(all) == 0 {
		return all, err
	}

	filtered := make([]content.Data[T], 0, len(all))
	for _, v := range all {
		if v.MainRefID == mainRefID {
			filtered = append(filtered, v)
		}
	}

	return filtered, nil
}

func copyData[T any](a T) (T, bool) {
	_buf := make([]byte, 0, 1000)
	buf := bytes.NewBuffer(_buf)
	encoder := gob.NewEncoder(buf)
	decoder := gob.NewDecoder(buf)
	encoder.Encode(a)
	var t T
	log.Error().Msgf("%+v", buf)
	decoder.Decode(&t)
	return t, true
}
