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
	Enqueue(job TransferJob)
	Stop()
	QueueDepth() int
}

type manager struct {
	mu                sync.Mutex
	cond              *sync.Cond
	queue             []TransferJob
	closed            bool
	wg                sync.WaitGroup
	workers           int
	store             storage.Store
	minio             storage.ObjectStorage
	hub               *hub.Hub
	defaultMaxRetries int
	defaultBackoffSec int
}

// NewManager creates a Manager with the given worker count.
// Call Start before Enqueue.
func NewManager(workers int, store storage.Store, minio storage.ObjectStorage, h *hub.Hub, defaultMaxRetries, defaultBackoffSec int) Manager {
	m := &manager{
		workers:           workers,
		store:             store,
		minio:             minio,
		hub:               h,
		defaultMaxRetries: defaultMaxRetries,
		defaultBackoffSec: defaultBackoffSec,
	}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Start spawns N worker goroutines that drain the queue.
func (m *manager) Start(ctx context.Context) {
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for {
				m.mu.Lock()
				for len(m.queue) == 0 && !m.closed {
					m.cond.Wait()
				}
				if m.closed && len(m.queue) == 0 {
					m.mu.Unlock()
					return
				}
				job := m.queue[0]
				m.queue[0] = TransferJob{} // zero out for GC
				m.queue = m.queue[1:]
				m.mu.Unlock()

				m.process(ctx, job)
			}
		}()
	}
}

// Enqueue appends a job to the unbounded queue. Never fails.
func (m *manager) Enqueue(job TransferJob) {
	m.mu.Lock()
	m.queue = append(m.queue, job)
	m.mu.Unlock()
	m.cond.Signal()
}

// Stop signals all workers to drain remaining jobs and exit.
func (m *manager) Stop() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cond.Broadcast()
	m.wg.Wait()
}

// QueueDepth returns the number of jobs currently waiting.
func (m *manager) QueueDepth() int {
	m.mu.Lock()
	n := len(m.queue)
	m.mu.Unlock()
	return n
}
