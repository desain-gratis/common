package schedulerapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	"github.com/magicaltux/ringbuf"
	"gopkg.in/natefinch/lumberjack.v2"
)

type QueueLogger func(id string) (string, *slog.Logger)

type queueManager struct {
	raftClient StateMachineClient

	queueByID     map[string]chan *TaskExecutionRuntime
	queueByIDLock *sync.RWMutex

	queueData     map[string]*queueState
	queueDataLock *sync.RWMutex

	logDir string
}

type queueState struct {
	logBuf *ringbuf.Writer
}

// Add let's user specify how the worker will execute the task
func (q *queueManager) Add(ctx context.Context, id string, queue chan<- *taskExecutionRuntimeView) *queueManager {
	queue2 := make(chan *TaskExecutionRuntime)
	go func() {
		defer close(queue2)
		for task := range queue2 {
			t := &taskExecutionRuntimeView{base: task}
			queue <- t
		}
	}()

	func() {
		q.queueByIDLock.Lock()
		defer q.queueByIDLock.Unlock()
		q.queueByID[id] = queue2
	}()

	func() {
		q.queueDataLock.Lock()
		defer q.queueDataLock.Unlock()
		q.queueData[id] = &queueState{}
	}()

	return q
}

type Executor func(ctx context.Context, input any) (any, error)

// AddFunction is a convenience function
func (q *queueManager) AddFunction(ctx context.Context, id string, ngoroutine int, fn Executor) *queueManager {
	if ngoroutine <= 0 {
		ngoroutine = 1
	}

	queue := make(chan *TaskExecutionRuntime)

	logBuf, err := ringbuf.New(10 * 1024 * 1024) // 1MB ring buffer for logs
	if err != nil {
		// TODO
		return nil
	}

	reada := logBuf.BlockingReader()
	llogger := &lumberjack.Logger{
		Filename: q.logDir + "/queue/" + id + "/log.log", // todo: proper path handling
	}

	lok := slog.New(slog.NewJSONHandler(logBuf, nil))

	go func() {
		defer reada.Close()
		defer llogger.Close()

		for {
			_, err := io.Copy(llogger, reada)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return
			}
		}
	}()

	fnWithHarness := taskFunctionHarness(q.raftClient, fn, lok)

	for range ngoroutine {
		go func() {
			defer close(queue)

			for task := range queue {
				// todo: more proper context handlling & app shutdown handling
				if ctx.Err() != nil {
					break
				}

				fnWithHarness(task)
			}
		}()
	}

	func() {
		q.queueByIDLock.Lock()
		defer q.queueByIDLock.Unlock()
		q.queueByID[id] = queue
	}()

	func() {
		q.queueDataLock.Lock()
		defer q.queueDataLock.Unlock()
		q.queueData[id] = &queueState{
			logBuf: logBuf,
		}
	}()

	return q
}
