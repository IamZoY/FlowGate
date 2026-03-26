package webhook

import (
	"encoding/json"
	"fmt"
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

const appCacheTTL = 30 * time.Second

// Handler processes inbound MinIO webhook events and enqueues transfer jobs.
type Handler struct {
	store             storage.Store
	manager           transfer.Manager
	hub               *hub.Hub
	encKey            []byte // AES key for decrypting MinIO credentials
	defaultMaxRetries int
	defaultBackoffSec int
	dedup             *deduper
	apps              *appCache
}

// NewHandler creates a Handler wired to the given dependencies.
func NewHandler(store storage.Store, manager transfer.Manager, h *hub.Hub, encKey []byte, defaultMaxRetries, defaultBackoffSec int, dedupWindow time.Duration) *Handler {
	return &Handler{
		store:             store,
		manager:           manager,
		hub:               h,
		encKey:            encKey,
		defaultMaxRetries: defaultMaxRetries,
		defaultBackoffSec: defaultBackoffSec,
		dedup:             newDeduper(dedupWindow),
		apps:              newAppCache(appCacheTTL),
	}
}

// ServeHTTP implements http.Handler so Handler can be mounted on a chi router.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	groupSlug := chi.URLParam(r, "group")
	appSlug := chi.URLParam(r, "app")
	cacheKey := groupSlug + ":" + appSlug

	// Fast path: resolved + decrypted app from cache.
	decryptedApp := h.apps.Get(cacheKey)
	if decryptedApp == nil {
		// Cache miss: query DB, verify, decrypt, then cache.
		app, err := h.store.GetAppByRoute(r.Context(), groupSlug, appSlug)
		if err != nil || app == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if !app.Enabled {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		dec, err := h.decryptAppCreds(app)
		if err != nil {
			slog.Error("decrypt credentials", "error", err, "app_id", app.ID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		decryptedApp = dec
		h.apps.Set(cacheKey, decryptedApp)
	}

	// Validate HMAC token.
	if err := Verify(r, decryptedApp.WebhookSecret); err != nil {
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

	// Compute effective retry config: per-app overrides global default.
	maxRetries := h.defaultMaxRetries
	backoffSec := h.defaultBackoffSec
	if decryptedApp.RetryMaxAttempts > 0 {
		maxRetries = decryptedApp.RetryMaxAttempts
	}
	if decryptedApp.RetryBackoffSeconds > 0 {
		backoffSec = decryptedApp.RetryBackoffSeconds
	}

	// Process each record — MinIO may batch multiple events per request.
	for _, rec := range event.Records {
		rawKey := rec.S3.Object.Key
		objectKey, err := url.PathUnescape(rawKey)
		if err != nil {
			objectKey = rawKey // fall back to raw if unescape fails
		}

		dedupKey := fmt.Sprintf("%s:%s", decryptedApp.ID, objectKey)

		// Fast path: in-memory dedup catches rapid-fire duplicates.
		if h.dedup.IsDuplicate(dedupKey) {
			slog.Debug("duplicate webhook skipped (in-memory)",
				"app_id", decryptedApp.ID, "object_key", objectKey)
			continue
		}

		// Slow path: DB check covers restarts where in-memory map is empty.
		if active, _ := h.store.HasActiveTransfer(r.Context(), decryptedApp.ID, objectKey); active {
			slog.Debug("duplicate webhook skipped (active transfer exists)",
				"app_id", decryptedApp.ID, "object_key", objectKey)
			continue
		}

		// Persist a pending transfer record.
		t := &storage.Transfer{
			ID:         uuid.NewString(),
			AppID:      decryptedApp.ID,
			ObjectKey:  objectKey,
			SrcBucket:  decryptedApp.Src.Bucket,
			DstBucket:  decryptedApp.Dst.Bucket,
			ObjectSize: rec.S3.Object.Size,
			ETag:       rec.S3.Object.ETag,
			Status:     "pending",
			CreatedAt:  time.Now(),
			MaxRetries: maxRetries,
		}
		if err := h.store.CreateTransfer(r.Context(), t); err != nil {
			slog.Error("create transfer record", "error", err, "object_key", objectKey)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Build the job and enqueue it (with decrypted creds).
		job := transfer.TransferJob{
			Transfer:    t,
			App:         decryptedApp,
			ObjectKey:   objectKey,
			MaxRetries:  maxRetries,
			BackoffSecs: backoffSec,
		}
		h.manager.Enqueue(job)

		// Notify dashboard clients (throttled under high load).
		if h.manager.QueueDepth() <= 1000 {
			h.hub.Broadcast(hub.Message{
				Type:      hub.MsgTransferQueued,
				Timestamp: time.Now(),
				Payload: map[string]any{
					"transfer_id": t.ID,
					"app_id":      decryptedApp.ID,
					"object_key":  objectKey,
					"object_size": rec.S3.Object.Size,
					"src_bucket":  t.SrcBucket,
					"dst_bucket":  t.DstBucket,
				},
			})
		}

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
