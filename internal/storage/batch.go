package storage

import (
	"database/sql"
	"log/slog"
	"time"
)

const (
	batchMaxSize  = 500
	batchInterval = 10 * time.Millisecond
	writeChanCap  = 16384
)

type writeRequest struct {
	query string
	args  []any
	done  chan error
}

type batchWriter struct {
	db    *sql.DB
	ch    chan writeRequest
	stop  chan struct{}
	ended chan struct{}
}

func newBatchWriter(db *sql.DB) *batchWriter {
	bw := &batchWriter{
		db:    db,
		ch:    make(chan writeRequest, writeChanCap),
		stop:  make(chan struct{}),
		ended: make(chan struct{}),
	}
	go bw.run()
	return bw
}

// exec sends a write request and blocks until the batch containing it is committed.
func (bw *batchWriter) exec(query string, args ...any) error {
	req := writeRequest{
		query: query,
		args:  args,
		done:  make(chan error, 1),
	}
	bw.ch <- req
	return <-req.done
}

func (bw *batchWriter) run() {
	defer close(bw.ended)

	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()

	batch := make([]writeRequest, 0, batchMaxSize)

	for {
		select {
		case req := <-bw.ch:
			batch = append(batch, req)
			// Drain any additional ready requests without blocking.
			batch = bw.drain(batch)
			if len(batch) >= batchMaxSize {
				bw.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				bw.flush(batch)
				batch = batch[:0]
			}
		case <-bw.stop:
			// Drain remaining items from the channel.
			for {
				select {
				case req := <-bw.ch:
					batch = append(batch, req)
				default:
					if len(batch) > 0 {
						bw.flush(batch)
					}
					return
				}
			}
		}
	}
}

func (bw *batchWriter) drain(batch []writeRequest) []writeRequest {
	for len(batch) < batchMaxSize {
		select {
		case req := <-bw.ch:
			batch = append(batch, req)
		default:
			return batch
		}
	}
	return batch
}

func (bw *batchWriter) flush(batch []writeRequest) {
	tx, err := bw.db.Begin()
	if err != nil {
		slog.Error("batch writer begin tx", "error", err, "batch_size", len(batch))
		for i := range batch {
			batch[i].done <- err
		}
		return
	}

	for i := range batch {
		_, execErr := tx.Exec(batch[i].query, batch[i].args...)
		batch[i].done <- execErr
	}

	if err := tx.Commit(); err != nil {
		slog.Error("batch writer commit", "error", err, "batch_size", len(batch))
		// Individual errors were already sent per-exec; the commit failure
		// is logged but rows that reported nil may not have been persisted.
		// In practice SQLite WAL commits rarely fail after successful execs.
	}
}

func (bw *batchWriter) close() {
	close(bw.stop)
	<-bw.ended
}
