package entity

import (
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
)

var _ mycontent.Data = &Worker{}

// RecurringTask represents a recurring task
// It's the main entry point for executing a task
type Worker struct {
	Ns string `json:"namespace"`

	// Raft ID
	Id string `json:"id"`

	IdInt uint64 `json:"id_int"`

	// Supported queue in this worker
	Queue map[string]*Queue `json:"queue"`

	// Host name
	Host string `json:"host"`

	LogUrl string `json:"log_url"`

	Url string `json:"url"`

	PublishedAt time.Time `json:"published_at"`
}

func (a *Worker) CreatedTime() time.Time {
	return a.PublishedAt
}

func (a *Worker) ID() string {
	return a.Id
}

func (a *Worker) Namespace() string {
	return a.Ns
}

func (a *Worker) RefIDs() []string {
	return nil
}

func (a *Worker) URL() string {
	return a.Url
}

func (a *Worker) Validate() error {
	return nil
}

func (a *Worker) WithCreatedTime(t time.Time) mycontent.Data {
	a.PublishedAt = t
	return a
}

func (a *Worker) WithID(id string) mycontent.Data {
	a.Id = id
	return a
}

func (a *Worker) WithNamespace(id string) mycontent.Data {
	a.Ns = id
	return a
}

func (a *Worker) WithURL(url string) mycontent.Data {
	a.Url = url
	return a
}

func (a *Worker) WithVersion(ver uint64) mycontent.Data {
	return a
}

func (a *Worker) DGVersion() *uint64 {
	return nil
}
