package transfer

import (
	"context"
	"sync"

	"github.com/ali/flowgate/internal/hub"
	"github.com/ali/flowgate/internal/storage"
)

// Manager is the public interface for the transfer worker pool.
type Manager interface {
	Start(ctx context.Context)
	Enqueue(job TransferJob) error
	Stop()
	QueueDepth() int
}

type manager struct {
	jobs    chan TransferJob
	wg      sync.WaitGroup
	workers int
	store   storage.Store
	minio   storage.ObjectStorage
	hub     *hub.Hub
}

// NewManager creates a Manager with the given worker count and channel capacity.
// Call Start before Enqueue.
func NewManager(workers, capacity int, store storage.Store, minio storage.ObjectStorage, h *hub.Hub) Manager {
	return &manager{
		jobs:    make(chan TransferJob, capacity),
		workers: workers,
		store:   store,
		minio:   minio,
		hub:     h,
	}
}

// Start spawns N worker goroutines that drain the jobs channel.
// Workers respect ctx for cooperative cancellation; when ctx is done they
// finish the current job and exit.
func (m *manager) Start(ctx context.Context) {
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for {
				select {
				case job, ok := <-m.jobs:
					if !ok {
						return
					}
					m.process(ctx, job)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}

// Enqueue places a job on the buffered channel without blocking.
// Returns ErrQueueFull immediately if the channel is at capacity.
func (m *manager) Enqueue(job TransferJob) error {
	select {
	case m.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// Stop closes the jobs channel and waits for all in-flight workers to finish.
// After Stop returns no further processing will occur.
func (m *manager) Stop() {
	close(m.jobs)
	m.wg.Wait()
}

// QueueDepth returns the number of jobs currently waiting in the channel.
func (m *manager) QueueDepth() int {
	return len(m.jobs)
}
