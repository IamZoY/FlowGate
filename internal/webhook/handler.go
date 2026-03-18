package webhook

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ali/flowgate/internal/group"
	"github.com/ali/flowgate/internal/hub"
	"github.com/ali/flowgate/internal/storage"
	"github.com/ali/flowgate/internal/transfer"
)

// Handler processes inbound MinIO webhook events and enqueues transfer jobs.
type Handler struct {
	store   storage.Store
	manager transfer.Manager
	hub     *hub.Hub
	encKey  []byte // AES key for decrypting MinIO credentials
}

// NewHandler creates a Handler wired to the given dependencies.
func NewHandler(store storage.Store, manager transfer.Manager, h *hub.Hub, encKey []byte) *Handler {
	return &Handler{store: store, manager: manager, hub: h, encKey: encKey}
}

// ServeHTTP implements http.Handler so Handler can be mounted on a chi router.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	groupSlug := chi.URLParam(r, "group")
	appSlug := chi.URLParam(r, "app")

	// Resolve (group_slug, app_slug) → App.
	app, err := h.store.GetAppByRoute(r.Context(), groupSlug, appSlug)
	if err != nil || app == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !app.Enabled {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Validate HMAC token.
	if err := Verify(r, app.WebhookSecret); err != nil {
		slog.Warn("webhook auth failed",
			"group", groupSlug,
			"app", appSlug,
			"error", err,
		)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse event body.
	var event Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "bad request: invalid JSON", http.StatusBadRequest)
		return
	}
	if len(event.Records) == 0 {
		http.Error(w, "bad request: no records", http.StatusBadRequest)
		return
	}

	// Decrypt MinIO credentials before passing to worker.
	decryptedApp, err := h.decryptAppCreds(app)
	if err != nil {
		slog.Error("decrypt credentials", "error", err, "app_id", app.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Process each record — MinIO may batch multiple events per request.
	for _, rec := range event.Records {
		rawKey := rec.S3.Object.Key
		objectKey, err := url.PathUnescape(rawKey)
		if err != nil {
			objectKey = rawKey // fall back to raw if unescape fails
		}

		// Persist a pending transfer record.
		t := &storage.Transfer{
			ID:         uuid.NewString(),
			AppID:      app.ID,
			ObjectKey:  objectKey,
			SrcBucket:  app.Src.Bucket,
			DstBucket:  app.Dst.Bucket,
			ObjectSize: rec.S3.Object.Size,
			ETag:       rec.S3.Object.ETag,
			Status:     "pending",
			CreatedAt:  time.Now(),
		}
		if err := h.store.CreateTransfer(r.Context(), t); err != nil {
			slog.Error("create transfer record", "error", err, "object_key", objectKey)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Build the job and enqueue it (with decrypted creds).
		job := transfer.TransferJob{
			Transfer:  t,
			App:       *decryptedApp,
			ObjectKey: objectKey,
		}
		if err := h.manager.Enqueue(job); err != nil {
			if errors.Is(err, transfer.ErrQueueFull) {
				http.Error(w, "service unavailable: queue full", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Notify dashboard clients.
		h.hub.Broadcast(hub.Message{
			Type:      hub.MsgTransferQueued,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"transfer_id": t.ID,
				"app_id":      app.ID,
				"object_key":  objectKey,
				"object_size": rec.S3.Object.Size,
				"src_bucket":  t.SrcBucket,
				"dst_bucket":  t.DstBucket,
			},
		})

		slog.Info("transfer queued",
			"transfer_id", t.ID,
			"group", groupSlug,
			"app", appSlug,
			"object_key", objectKey,
		)
	}

	w.WriteHeader(http.StatusAccepted)
}

// decryptAppCreds returns a copy of the App with src/dst SecretKey decrypted.
func (h *Handler) decryptAppCreds(app *group.App) (*group.App, error) {
	copy := *app
	srcSecret, err := group.DecryptSecret(app.Src.SecretKey, h.encKey)
	if err != nil {
		return nil, err
	}
	dstSecret, err := group.DecryptSecret(app.Dst.SecretKey, h.encKey)
	if err != nil {
		return nil, err
	}
	copy.Src.SecretKey = srcSecret
	copy.Dst.SecretKey = dstSecret
	return &copy, nil
}
