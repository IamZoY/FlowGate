package transfer

import (
	"context"
	"fmt"
	"log/slog"
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

	// Mark in_progress.
	now := time.Now()
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

	// Perform the actual object transfer.
	if err := m.doTransfer(ctx, job, log); err != nil {
		m.fail(ctx, t, key, err)
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

	log.Info("streaming object", "size_bytes", size)

	if err := m.minio.PutObject(ctx, app.Dst, key, rc, size); err != nil {
		return fmt.Errorf("PutObject: %w", err)
	}
	return nil
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
