package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ali/flowgate/internal/group"
	"github.com/ali/flowgate/internal/hub"
	"github.com/ali/flowgate/internal/storage"
)

// API provides all REST handlers for the dashboard.
type API struct {
	store storage.Store
	svc   *group.Service
	hub   *hub.Hub
}

// NewAPI creates an API handler set using the given store, AES key, and WebSocket hub.
func NewAPI(store storage.Store, encKey []byte, h *hub.Hub) *API {
	return &API{
		store: store,
		svc:   group.NewService(store, encKey),
		hub:   h,
	}
}

// Router returns a chi sub-router with all API routes mounted.
func (a *API) Router() http.Handler {
	r := chi.NewRouter()

	// Groups
	r.Get("/groups", a.ListGroups)
	r.Post("/groups", a.CreateGroup)
	r.Get("/groups/{id}", a.GetGroup)
	r.Put("/groups/{id}", a.UpdateGroup)
	r.Delete("/groups/{id}", a.DeleteGroup)

	// Apps (nested under group + flat)
	r.Get("/groups/{id}/apps", a.ListApps)
	r.Post("/groups/{id}/apps", a.CreateApp)
	r.Get("/apps/{id}", a.GetApp)
	r.Put("/apps/{id}", a.UpdateApp)
	r.Delete("/apps/{id}", a.DeleteApp)
	r.Get("/apps/{id}/webhook-url", a.GetWebhookURL)
	r.Get("/apps/{id}/transfers", a.AppTransfers)

	// Transfers
	r.Get("/transfers", a.ListTransfers)
	r.Get("/transfers/{id}", a.GetTransfer)

	// Stats
	r.Get("/stats", a.GetStats)

	return r
}

// ---------- helpers ----------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func queryStr(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}

func queryInt(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// ---------- Group handlers ---------------------------------------------------

func (a *API) ListGroups(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListGroups(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	g := &group.Group{
		ID:          uuid.NewString(),
		Name:        body.Name,
		Description: body.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := a.store.CreateGroup(r.Context(), g); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.hub.Broadcast(hub.Message{Type: hub.MsgGroupCreated, Timestamp: time.Now(), Payload: g})
	writeJSON(w, http.StatusCreated, g)
}

func (a *API) GetGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	g, err := a.store.GetGroup(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (a *API) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	g, err := a.store.GetGroup(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := readJSON(r, &body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name != nil {
		g.Name = *body.Name
	}
	if body.Description != nil {
		g.Description = *body.Description
	}
	g.UpdatedAt = time.Now()
	if err := a.store.UpdateGroup(r.Context(), g); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.hub.Broadcast(hub.Message{Type: hub.MsgGroupUpdated, Timestamp: time.Now(), Payload: g})
	writeJSON(w, http.StatusOK, g)
}

func (a *API) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.store.DeleteGroup(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrHasActiveTransfers) {
			http.Error(w, "conflict: has active transfers", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.hub.Broadcast(hub.Message{Type: hub.MsgGroupDeleted, Timestamp: time.Now(), Payload: map[string]string{"id": id}})
	w.WriteHeader(http.StatusNoContent)
}

// ---------- App handlers -----------------------------------------------------

func (a *API) ListApps(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	apps, err := a.store.ListAppsByGroup(r.Context(), groupID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (a *API) CreateApp(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Src         struct {
			Endpoint  string `json:"endpoint"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
			Bucket    string `json:"bucket"`
			Region    string `json:"region"`
			UseSSL    bool   `json:"use_ssl"`
		} `json:"src"`
		Dst struct {
			Endpoint  string `json:"endpoint"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
			Bucket    string `json:"bucket"`
			Region    string `json:"region"`
			UseSSL    bool   `json:"use_ssl"`
		} `json:"dst"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	app, err := a.svc.CreateApp(r.Context(), groupID, body.Name, body.Description,
		group.MinIOConfig{
			Endpoint:  body.Src.Endpoint,
			AccessKey: body.Src.AccessKey,
			SecretKey: body.Src.SecretKey,
			Bucket:    body.Src.Bucket,
			Region:    body.Src.Region,
			UseSSL:    body.Src.UseSSL,
		},
		group.MinIOConfig{
			Endpoint:  body.Dst.Endpoint,
			AccessKey: body.Dst.AccessKey,
			SecretKey: body.Dst.SecretKey,
			Bucket:    body.Dst.Bucket,
			Region:    body.Dst.Region,
			UseSSL:    body.Dst.UseSSL,
		},
	)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.hub.Broadcast(hub.Message{Type: hub.MsgAppCreated, Timestamp: time.Now(), Payload: map[string]string{"group_id": groupID, "app_id": app.ID}})
	writeJSON(w, http.StatusCreated, app)
}

func (a *API) GetApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := a.store.GetApp(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (a *API) UpdateApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := a.store.GetApp(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Enabled     *bool   `json:"enabled"`
		Src         *struct {
			Endpoint  string `json:"endpoint"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
			Bucket    string `json:"bucket"`
			Region    string `json:"region"`
			UseSSL    bool   `json:"use_ssl"`
		} `json:"src"`
		Dst *struct {
			Endpoint  string `json:"endpoint"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
			Bucket    string `json:"bucket"`
			Region    string `json:"region"`
			UseSSL    bool   `json:"use_ssl"`
		} `json:"dst"`
	}
	if err := readJSON(r, &body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if body.Name != nil {
		app.Name = *body.Name
	}
	if body.Description != nil {
		app.Description = *body.Description
	}
	if body.Enabled != nil {
		app.Enabled = *body.Enabled
	}
	if body.Src != nil {
		app.Src.Endpoint = body.Src.Endpoint
		app.Src.AccessKey = body.Src.AccessKey
		app.Src.Bucket = body.Src.Bucket
		app.Src.Region = body.Src.Region
		app.Src.UseSSL = body.Src.UseSSL
		if body.Src.SecretKey != "" {
			enc, err := a.svc.EncryptSecret(body.Src.SecretKey)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			app.Src.SecretKey = enc
		}
	}
	if body.Dst != nil {
		app.Dst.Endpoint = body.Dst.Endpoint
		app.Dst.AccessKey = body.Dst.AccessKey
		app.Dst.Bucket = body.Dst.Bucket
		app.Dst.Region = body.Dst.Region
		app.Dst.UseSSL = body.Dst.UseSSL
		if body.Dst.SecretKey != "" {
			enc, err := a.svc.EncryptSecret(body.Dst.SecretKey)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			app.Dst.SecretKey = enc
		}
	}
	app.UpdatedAt = time.Now()
	if err := a.store.UpdateApp(r.Context(), app); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.hub.Broadcast(hub.Message{Type: hub.MsgAppUpdated, Timestamp: time.Now(), Payload: map[string]string{"group_id": app.GroupID, "app_id": app.ID}})
	writeJSON(w, http.StatusOK, app)
}

func (a *API) DeleteApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Fetch app first to get group_id for the broadcast.
	app, _ := a.store.GetApp(r.Context(), id)
	if err := a.store.DeleteApp(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrHasActiveTransfers) {
			http.Error(w, "conflict: has active transfers", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	groupID := ""
	if app != nil {
		groupID = app.GroupID
	}
	a.hub.Broadcast(hub.Message{Type: hub.MsgAppDeleted, Timestamp: time.Now(), Payload: map[string]string{"id": id, "group_id": groupID}})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) GetWebhookURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := a.store.GetApp(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"webhook_secret": app.WebhookSecret,
		"app_id":         id,
	})
}

// ---------- Transfer handlers ------------------------------------------------

func (a *API) ListTransfers(w http.ResponseWriter, r *http.Request) {
	opts := storage.ListTransfersOpts{
		AppID:   queryStr(r, "app_id"),
		GroupID: queryStr(r, "group_id"),
		Status:  queryStr(r, "status"),
		Limit:   queryInt(r, "limit", 50),
		Offset:  queryInt(r, "offset", 0),
	}
	transfers, total, err := a.store.ListTransfers(r.Context(), opts)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transfers": transfers,
		"total":     total,
		"limit":     opts.Limit,
		"offset":    opts.Offset,
	})
}

func (a *API) GetTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := a.store.GetTransfer(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *API) AppTransfers(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	opts := storage.ListTransfersOpts{
		AppID:  appID,
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
		Status: queryStr(r, "status"),
	}
	transfers, total, err := a.store.ListTransfers(r.Context(), opts)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transfers": transfers,
		"total":     total,
	})
}

func (a *API) GetStats(w http.ResponseWriter, r *http.Request) {
	appID := queryStr(r, "app_id")
	groupID := queryStr(r, "group_id")
	stats, err := a.store.GetStats(r.Context(), appID, groupID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
