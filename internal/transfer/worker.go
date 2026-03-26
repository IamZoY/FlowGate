package transfer

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/ali/flowgate/internal/hub"
	"github.com/ali/flowgate/internal/storage"
)

// process executes a single TransferJob: get → put → update DB → broadcast.
func (m *manager) process(ctx context.Context, job TransferJob) {
	t := job.Transfer
	app := job.App
	key := job.ObjectKey

	log := slog.With(
		"transfer_id", t.ID,
		"app_id", app.ID,
		"object_key", key,
	)

	// Mark in_progress only on the first attempt; retries are already
	// tracked in the DB with status "retrying".
	now := time.Now()
	if job.RetryCount == 0 {
		t.Status = StatusInProgress
		t.StartedAt = &now
		if err := m.store.UpdateTransfer(ctx, t); err != nil {
			log.Error("update in_progress", "error", err)
		}
		m.hub.Broadcast(hub.Message{
			Type:      hub.MsgTransferStarted,
			Timestamp: now,
			Payload: map[string]any{
				"transfer_id": t.ID,
				"app_id":      app.ID,
				"object_key":  key,
			},
		})
	}

	// Perform the actual object transfer.
	if err := m.doTransfer(ctx, job, log); err != nil {
		if job.RetryCount < job.MaxRetries {
			m.scheduleRetry(ctx, job, err, log)
		} else {
			m.fail(ctx, t, key, err)
		}
		return
	}

	// Mark success.
	completed := time.Now()
	durationMs := float64(completed.Sub(now).Milliseconds())
	t.Status = StatusSuccess
	t.CompletedAt = &completed
	t.DurationMs = durationMs
	t.BytesTransferred = t.ObjectSize
	if err := m.store.UpdateTransfer(ctx, t); err != nil {
		log.Error("update success", "error", err)
	}
	log.Info("transfer completed",
		"duration_ms", durationMs,
		"bytes", t.ObjectSize,
	)
	m.hub.Broadcast(hub.Message{
		Type:      hub.MsgTransferCompleted,
		Timestamp: completed,
		Payload: map[string]any{
			"transfer_id": t.ID,
			"app_id":      app.ID,
			"object_key":  key,
			"duration_ms": durationMs,
			"bytes":       t.ObjectSize,
		},
	})
}

// doTransfer streams the object from source MinIO to destination MinIO.
func (m *manager) doTransfer(ctx context.Context, job TransferJob, log *slog.Logger) error {
	app := job.App
	key := job.ObjectKey
	t := job.Transfer

	rc, size, err := m.minio.GetObject(ctx, app.Src, key)
	if err != nil {
		return fmt.Errorf("GetObject: %w", err)
	}
	defer rc.Close()

	// Use size from DB record if MinIO didn't return one.
	if size <= 0 {
		size = t.ObjectSize
	}

	log.Debug("streaming object", "size_bytes", size)

	if err := m.minio.PutObject(ctx, app.Dst, key, rc, size); err != nil {
		return fmt.Errorf("PutObject: %w", err)
	}
	return nil
}

// scheduleRetry sets the transfer to "retrying" and re-enqueues after a delay.
func (m *manager) scheduleRetry(ctx context.Context, job TransferJob, cause error, log *slog.Logger) {
	job.RetryCount++

	// Exponential backoff: base * 2^(attempt-1), with full jitter.
	base := time.Duration(job.BackoffSecs) * time.Second
	delay := base * (1 << (job.RetryCount - 1))

	const maxDelay = 1 * time.Hour
	if delay > maxDelay || delay <= 0 {
		delay = maxDelay
	}

	const minDelay = 5 * time.Second
	if delay < minDelay {
		delay = minDelay
	}
	if delay > minDelay {
		delay = minDelay + time.Duration(rand.Int63n(int64(delay-minDelay)))
	}

	nextRetry := time.Now().Add(delay)

	t := job.Transfer
	t.Status = StatusRetrying
	t.RetryCount = job.RetryCount
	t.ErrorMessage = cause.Error()
	t.NextRetryAt = &nextRetry

	if err := m.store.UpdateTransfer(ctx, t); err != nil {
		log.Error("update retrying status", "transfer_id", t.ID, "error", err)
	}

	log.Warn("scheduling retry",
		"retry", job.RetryCount,
		"max_retries", job.MaxRetries,
		"delay", delay.Round(time.Millisecond),
	)

	m.hub.Broadcast(hub.Message{
		Type:      hub.MsgTransferRetrying,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"transfer_id":   t.ID,
			"app_id":        job.App.ID,
			"object_key":    job.ObjectKey,
			"retry_count":   job.RetryCount,
			"max_retries":   job.MaxRetries,
			"next_retry_at": nextRetry,
			"error_message": cause.Error(),
		},
	})

	time.AfterFunc(delay, func() {
		// Reset transfer state for re-processing.
		t.Status = StatusPending
		t.ErrorMessage = ""
		t.StartedAt = nil
		t.CompletedAt = nil
		t.NextRetryAt = nil

		if err := m.Enqueue(job); err != nil {
			log.Error("re-enqueue retry failed", "transfer_id", t.ID, "error", err)
			m.fail(ctx, t, job.ObjectKey, fmt.Errorf("retry enqueue failed: %w (original: %s)", err, cause))
		}
	})
}

// fail marks a transfer as failed in the DB and broadcasts the event.
func (m *manager) fail(ctx context.Context, t *storage.Transfer, key string, cause error) {
	completed := time.Now()
	t.Status = StatusFailed
	t.ErrorMessage = cause.Error()
	t.CompletedAt = &completed
	slog.Error("transfer failed",
		"transfer_id", t.ID,
		"object_key", key,
		"error", cause,
	)
	if err := m.store.UpdateTransfer(ctx, t); err != nil {
		slog.Error("update failed status", "transfer_id", t.ID, "error", err)
	}
	m.hub.Broadcast(hub.Message{
		Type:      hub.MsgTransferFailed,
		Timestamp: completed,
		Payload: map[string]any{
			"transfer_id":   t.ID,
			"object_key":    key,
			"error_message": cause.Error(),
		},
	})
}
