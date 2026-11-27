package schedulerapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/scheduler-api/statemachine"
	entity "github.com/desain-gratis/common/types/entity/scheduler"
	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
	"github.com/minio/simdjson-go"
)

type httpAPI struct {
	*worker
	raftClient StateMachineClient
	tes        statemachine.TaskExecutionUsecase
}

func (h *httpAPI) Hello(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	taskCh, errCh := h.tes.GetAll(r.Context(), "deployd")
	for taskCh != nil || errCh != nil {
		select {
		case task, ok := <-taskCh:
			if !ok {
				taskCh = nil
				continue
			}

			// Process task.
			result, err := json.Marshal(task)
			if err != nil {
				continue
			}
			fmt.Fprintln(w, string(result))

		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}

			if err != nil {
				log.Println("GetAll error:", err)
			}
		}
	}
}

func (h *httpAPI) GetDailyCount(
	w http.ResponseWriter,
	r *http.Request,
	p httprouter.Params,
) {
	filter, timezone, indexes, err := parseAggregationInput(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	results, errs := h.tes.GetDailyCount(
		r.Context(),
		filter,
		timezone,
		indexes...,
	)

	data := make([]*statemachine.TaskExecutionDailyCount, 0)

	for results != nil || errs != nil {
		select {
		case result, ok := <-results:
			if !ok {
				results = nil
				continue
			}
			data = append(data, result)

		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": err.Error(),
				})
				return
			}

		case <-r.Context().Done():
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": data})
}

func (h *httpAPI) GetHourlyCount(
	w http.ResponseWriter,
	r *http.Request,
	p httprouter.Params,
) {
	filter, timezone, indexes, err := parseAggregationInput(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	results, errs := h.tes.GetHourlyCount(
		r.Context(),
		filter,
		timezone,
		indexes...,
	)

	data := make([]*statemachine.TaskExecutionHourlyCount, 0)

	for results != nil || errs != nil {
		select {
		case result, ok := <-results:
			if !ok {
				results = nil
				continue
			}
			data = append(data, result)

		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": err.Error(),
				})
				return
			}

		case <-r.Context().Done():
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": data})
}

func (h *httpAPI) ExecuteTask(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "WADUUH: %v", err)
		return
	}

	var input entity.TaskExecution
	err = json.Unmarshal(payload, &input)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "WADUUH: %v", err)
		return
	}

	t0 := time.Now()

	taskExec := &entity.TaskExecution{
		Ns:             input.Ns,
		Id:             uuid.NewString(),
		ParentId:       input.ParentId,
		QueueID:        input.QueueID,
		ScheduledRunAt: t0,
		Status:         entity.ExecutionStatusPending,
		Input:          input.Input,
		UserTime:       input.UserTime,
		ExecutionMode:  entity.TaskExecutionModeImmediate,
		PublishedAt:    t0,

		// overwrriten
		AssignmentState: nil,
		ExecutionState:  nil,
		RecurringTaskID: nil,
		URLx:            "",
	}

	// server do something
	data := statemachine.CreateTaskExecution(taskExec)

	result, err := h.raftClient.Propose(r.Context(), data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "XXX UWADUUH: %v %v", err, data)
		return
	}

	resp, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "XXX UWADUUH: %v %v", err, data)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, string(resp))
}

func (h *httpAPI) CancelRecurringTask(w http.ResponseWriter, r *http.Request, p httprouter.Params) {

}

func (h *httpAPI) CancelTaskExecution(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "WADUUH: %v", err)
		return
	}

	var input entity.TaskExecution
	err = json.Unmarshal(payload, &input)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "WADUUH: %v", err)
		return
	}

	taskExec, err := h.tes.GetByID(r.Context(), input.Ns, input.QueueID, input.Id)
	if err != nil {
		if errors.Is(err, mycontent.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "WADUUH: %v", err)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "WADUUH: %v", err)
		return
	}

	// client validation (raft will validate as well, this just to make it quicker)

	err = statemachine.ValidateUpdate(taskExec, &input)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "WADUUH: %v", err)
		return
	}

	taskExec.Status = entity.ExecutionStatusCancelled

	// TODO: take some field from input (eg. cancellation reason, etc..)

	// Update to raft
	result, err := h.raftClient.Propose(r.Context(), statemachine.UpdateTaskExecution(taskExec))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "XXX UWADUUH: %v %v", err, taskExec)
		return
	}

	resp, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "XXX UWADUUH: %v %v", err, result)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(resp))
}

func (h *httpAPI) CreateRecurringTask(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "WADUUH: %v", err)
		return
	}

	var taskDefinition entity.RecurringTask
	err = json.Unmarshal(payload, &taskDefinition)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "WADUUH: %v", err)
		return
	}

	taskDefinition.Id = uuid.NewString()
	taskDefinition.PublishedAt = time.Now().In(taskDefinition.UserTime.Location())

	// server do something
	data := statemachine.CreateRecurringTask{
		Task:       taskDefinition,
		ServerTime: time.Now(),
	}

	result, err := h.raftClient.Propose(r.Context(), data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "XXX UWADUUH: %v %v", err, data)
		return
	}

	resp, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "XXX UWADUUH: %v %v", err, data)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, string(resp))
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func parseAggregationInput(
	r *http.Request,
) (statemachine.TaskExecutionFilter, string, []string, error) {
	query := r.URL.Query()

	namespace := strings.TrimSpace(r.Header.Get("X-Namespace"))
	if namespace == "" {
		return statemachine.TaskExecutionFilter{}, "", nil,
			fmt.Errorf("X-Namespace header is required")
	}

	timezone := query.Get("timezone")
	if timezone == "" {
		timezone = "UTC"
	}

	if _, err := time.LoadLocation(timezone); err != nil {
		return statemachine.TaskExecutionFilter{}, "", nil,
			fmt.Errorf("invalid timezone: %s", timezone)
	}

	filter := statemachine.TaskExecutionFilter{}

	if value := query.Get("before"); value != "" {
		before, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return filter, "", nil,
				fmt.Errorf("invalid before: expected RFC3339")
		}
		filter.Before = &before
	}

	parseDate := func(key string) (*time.Time, error) {
		value := query.Get(key)
		if value == "" {
			return nil, nil
		}

		date, err := time.Parse("2006-01-02", value)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: expected YYYY-MM-DD", key)
		}

		return &date, nil
	}

	var err error

	filter.StartDate, err = parseDate("start_date")
	if err != nil {
		return filter, "", nil, err
	}

	filter.EndDate, err = parseDate("end_date")
	if err != nil {
		return filter, "", nil, err
	}

	if filter.StartDate != nil &&
		filter.EndDate != nil &&
		filter.StartDate.After(*filter.EndDate) {
		return filter, "", nil,
			fmt.Errorf("start_date must not be after end_date")
	}

	if value := query["execution_status"]; len(value) > 0 {
		for _, v := range value {
			if strings.TrimSpace(v) == "" {
				return filter, "", nil,
					fmt.Errorf("execution_status cannot be empty")
			}
			filter.ExecutionStatus = append(filter.ExecutionStatus, entity.ExecutionStatus(v))
		}
	}

	if value := query.Get("assignment_status"); value != "" {
		v := entity.AssignmentStatus(value)
		filter.AssignmentStatus = &v
	}

	if values := query["assigned_node"]; len(values) > 0 {
		for _, value := range values {
			nodeID, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return filter, "", nil,
					fmt.Errorf("invalid assigned_node: %s", value)
			}
			filter.AssignedNode = append(filter.AssignedNode, nodeID)
		}
	}

	indexes := []string{namespace}

	if value := query.Get("queue_id"); value != "" {
		indexes = append(indexes, value)
	} else if query.Has("id") {
		return filter, "", nil,
			fmt.Errorf("queue_id is required when id is specified")
	}

	if value := query.Get("id"); value != "" {
		indexes = append(indexes, value)
	}

	return filter, timezone, indexes, nil
}

func (q *httpAPI) TailLog(appCtx context.Context) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if r.URL.Query().Get("task_id") == "" {
			q.tailLogAll(appCtx, w, r, p)
			return
		}

		q.tailLogFiltered(appCtx, w, r, p)
	}
}

func (q *httpAPI) tailLogFiltered(
	appCtx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	_ httprouter.Params,
) {
	queueID := r.URL.Query().Get("queue_id")
	taskID := r.URL.Query().Get("task_id")
	// task, err := q.worker.teStorageV2.GetByID(r.Context(), q.namespace, queueID, taskID)
	// if err != nil {
	// 	writeJSON(w, http.StatusBadRequest, map[string]string{
	// 		"error": err.Error(),
	// 	})
	// 	return
	// }

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var queueState *queueState
	func() {
		q.worker.queue.queueDataLock.RLock()
		defer q.worker.queue.queueDataLock.RUnlock()

		queueState = q.worker.queue.queueData[queueID]
	}()

	// might miss subtle case / duplcate

	if queueState == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "queue not found",
		})
		return
	}

	reader := queueState.logBuf.BlockingReader()

	// just tail the latest log
	reader.Reset()

	var closeOnce sync.Once
	closeReader := func() {
		closeOnce.Do(func() {
			reader.Close()
		})
	}
	defer closeReader()

	// Close the reader when the HTTP client disconnects.
	stop := make(chan struct{})
	defer close(stop)

	go func() {
		select {
		case <-appCtx.Done():
			closeReader()
		case <-r.Context().Done():
			closeReader()
		case <-stop:
		}
	}()

	// simdjson NDJSON parser.
	reuse := make(chan *simdjson.ParsedJson, 10)
	stream := make(chan simdjson.Stream, 1)

	go simdjson.ParseNDStream(reader, stream, reuse)

	connected := true

	for got := range stream {
		if got.Error != nil {
			if errors.Is(got.Error, io.EOF) {
				break
			}

			// Expected when the HTTP client disconnects.
			if r.Context().Err() != nil {
				break
			}

			if appCtx.Err() != nil {
				break
			}

			if errors.Is(got.Error, io.ErrClosedPipe) {
				break
			}

			log.Printf("log parser error: %v", got.Error)
			continue
		}

		if got.Value == nil {
			continue
		}

		matched := taskID == ""

		// Filter by task_id if requested.
		if taskID != "" {
			var elem *simdjson.Element

			err := got.Value.ForEach(func(i simdjson.Iter) error {
				var err error

				elem, err = i.FindElement(elem, "task_id")
				if err != nil {
					return nil
				}

				bts, err := elem.Iter.StringBytes()
				if err != nil {
					return nil
				}

				matched = string(bts) == taskID
				return nil
			})

			if err != nil {
				log.Printf("log filter error: %v", err)
			}
		}

		// Write original JSON bytes without re-serialization.
		if connected && matched {
			if _, err := w.Write(got.Value.Message); err != nil {
				connected = false
				closeReader()
			} else {
				// ParseNDStream trims trailing whitespace from Message.
				// Restore the NDJSON delimiter.
				if _, err := w.Write([]byte{'\n'}); err != nil {
					connected = false
					closeReader()
				} else {
					flusher.Flush()
				}
			}
		}

		// Return ParsedJson to the parser for reuse.
		reuse <- got.Value
	}
}

// Raw streaming without JSON parsing or filtering.
func (q *httpAPI) tailLogAll(
	appCtx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	_ httprouter.Params,
) {
	// Move the TailLogRaw implementation here.
	queueID := r.URL.Query().Get("queue_id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var queueState *queueState
	func() {
		q.worker.queue.queueDataLock.RLock()
		defer q.worker.queue.queueDataLock.RUnlock()

		queueState = q.worker.queue.queueData[queueID]
	}()

	if queueState == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "queue not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	reader := queueState.logBuf.BlockingReader()
	reader.Reset()
	defer reader.Close()

	var closeOnce sync.Once
	closeReader := func() {
		closeOnce.Do(func() {
			reader.Close()
		})
	}

	stop := make(chan struct{})
	defer close(stop)

	go func() {
		select {
		case <-appCtx.Done():
			closeReader()
		case <-r.Context().Done():
			closeReader()
		case <-stop:
		}
	}()

	buf := make([]byte, 32*1024)

	for {
		n, err := reader.Read(buf)

		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				closeReader()
				return
			}

			flusher.Flush()
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}

			if r.Context().Err() != nil || appCtx.Err() != nil {
				return
			}

			if errors.Is(err, io.ErrClosedPipe) {
				return
			}

			log.Printf("log reader error: %v", err)
			return
		}
	}
}
