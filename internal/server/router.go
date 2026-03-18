package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/ali/flowgate/internal/hub"
	"github.com/ali/flowgate/internal/transfer"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewRouter wires all routes and returns the root http.Handler.
func NewRouter(
	webhookHandler http.Handler,
	apiHandler http.Handler,
	dashHandler http.Handler,
	h *hub.Hub,
	manager transfer.Manager,
) http.Handler {
	r := chi.NewRouter()
	r.Use(RequestID)
	r.Use(Logger)
	r.Use(Recoverer)

	// Webhook ingest.
	r.Post("/webhook/{group}/{app}", webhookHandler.ServeHTTP)

	// REST API.
	r.Mount("/api", apiHandler)

	// WebSocket live feed.
	r.Get("/ws", func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		hub.NewClient(h, conn)
	})

	// Health endpoint.
	r.Get("/health", healthHandler(h, manager))

	// SPA catch-all — must come last.
	r.Mount("/", dashHandler)

	return r
}

// healthHandler returns a simple JSON liveness/readiness response.
func healthHandler(_ *hub.Hub, manager transfer.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		qd := manager.QueueDepth()
		w.Write([]byte(`{"status":"ok","queue_depth":` + itoa(qd) + `}`))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
