package process

import (
	"context"
	"errors"
	"sync"
)

var ErrProcessNotFound = errors.New("process not running")

type Entry struct {
	PID  int
	PGID int
	Name string
	Wait func(context.Context) error
}

type Registry struct {
	mu      sync.Mutex
	entries map[int]Entry
}

func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[int]Entry),
	}
}

func (r *Registry) Register(pid, pgid int, name string) {
	r.RegisterWithWait(pid, pgid, name, nil)
}

func (r *Registry) RegisterWithWait(pid, pgid int, name string, wait func(context.Context) error) {
	if r == nil || pid <= 0 {
		return
	}
	r.mu.Lock()
	r.entries[pid] = Entry{
		PID:  pid,
		PGID: pgid,
		Name: name,
		Wait: wait,
	}
	r.mu.Unlock()
}

func (r *Registry) Unregister(pid int) {
	if r == nil || pid <= 0 {
		return
	}
	r.mu.Lock()
	delete(r.entries, pid)
	r.mu.Unlock()
}

func (r *Registry) StopAll(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	entries := make([]Entry, 0, len(r.entries))
	for _, entry := range r.entries {
		entries = append(entries, entry)
	}
	r.mu.Unlock()

	var stopErr error
	for _, entry := range entries {
		if err := stopProcess(ctx, entry.PID, entry.PGID, entry.Wait); err != nil && !errors.Is(err, ErrProcessNotFound) {
			stopErr = errors.Join(stopErr, err)
		}
	}
	if len(entries) > 0 {
		r.mu.Lock()
		for _, entry := range entries {
			delete(r.entries, entry.PID)
		}
		r.mu.Unlock()
	}
	return stopErr
}
