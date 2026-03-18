package dashboard

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/ali/flowgate/internal/storage"
	"github.com/ali/flowgate/internal/transfer"
)

// Handler serves the embedded SPA assets.
// All paths that don't match a real file fall back to index.html.
type Handler struct {
	fsys   fs.FS
	server http.Handler
}

// NewHandler creates an SPA handler from the given filesystem (already rooted at web/).
func NewHandler(webFS fs.FS) *Handler {
	return &Handler{
		fsys:   webFS,
		server: http.FileServer(http.FS(webFS)),
	}
}

// ServeHTTP serves static assets or falls back to index.html for SPA routing.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try serving the exact file first.
	if r.URL.Path != "/" {
		f, err := h.fsys.Open(r.URL.Path[1:]) // strip leading /
		if err == nil {
			f.Close()
			h.server.ServeHTTP(w, r)
			return
		}
	}
	// Fall back to index.html for all other paths (SPA client-side routing).
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	h.server.ServeHTTP(w, r2)
}

// Health returns JSON with service status and queue depth.
func Health(store storage.Store, manager transfer.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbOK := "ok"
		if err := store.Ping(r.Context()); err != nil {
			dbOK = "error"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":      "ok",
			"db":          dbOK,
			"queue_depth": manager.QueueDepth(),
		})
	}
}
