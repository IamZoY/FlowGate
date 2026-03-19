package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ali/flowgate/internal/config"
	"github.com/ali/flowgate/internal/dashboard"
	"github.com/ali/flowgate/internal/hub"
	"github.com/ali/flowgate/internal/group"
	"github.com/ali/flowgate/internal/server"
	"github.com/ali/flowgate/internal/storage"
	"github.com/ali/flowgate/internal/transfer"
	"github.com/ali/flowgate/internal/webhook"
	"github.com/ali/flowgate/web"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	if *showVersion {
		fmt.Printf("flowgate %s (commit: %s, built: %s)\n", version, commit, buildTime)
		os.Exit(0)
	}

	// 1. Load configuration.
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	// 2. Configure structured logging.
	var logLevel slog.Level
	_ = logLevel.UnmarshalText([]byte(cfg.Logging.Level))
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: logLevel}
	if cfg.Logging.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))

	// 3. Open SQLite store (runs migrations automatically).
	store, err := storage.NewSQLiteStore(
		cfg.Database.Path,
		cfg.Database.MaxOpenConnections,
		cfg.Database.MaxIdleConnections,
	)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// 4. Derive AES encryption key from config.
	encKey, err := group.DeriveKey(cfg.Security.SecretKey)
	if err != nil {
		slog.Error("derive key", "error", err)
		os.Exit(1)
	}

	// 5. MinIO object storage client.
	minioClient := storage.NewMinIOClient()

	// 6. WebSocket hub.
	h := hub.NewHub()
	go h.Run()

	// 7. Transfer worker pool.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	mgr := transfer.NewManager(
		cfg.Transfer.WorkerPoolSize,
		cfg.Transfer.QueueCapacity,
		store,
		minioClient,
		h,
	)
	mgr.Start(ctx)

	// 8. Periodic stats broadcast.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				stats, err := store.GetStats(ctx, "", "")
				if err == nil {
					h.Broadcast(hub.Message{Type: hub.MsgStatsUpdate, Timestamp: time.Now(), Payload: stats})
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 9. HTTP handlers.
	webhookHandler := webhook.NewHandler(store, mgr, h, encKey)
	api := dashboard.NewAPI(store, encKey, h)
	webAssets, err := fs.Sub(web.Assets, "assets")
	if err != nil {
		slog.Error("embed web assets", "error", err)
		os.Exit(1)
	}
	dashHandler := dashboard.NewHandler(webAssets)

	// 9. Router.
	router := server.NewRouter(webhookHandler, api.Router(), dashHandler, h, mgr)

	// 10. HTTP server.
	srv := server.New(cfg.Server, router)

	go func() {
		slog.Info("flowgate starting",
			"addr", cfg.Server.Host+":"+itoa(cfg.Server.Port),
		)
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			cancel()
		}
	}()

	// 11. Wait for shutdown signal.
	<-ctx.Done()
	slog.Info("shutting down")

	// 12. Graceful shutdown: drain HTTP first, then stop workers.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown", "error", err)
	}
	mgr.Stop()
	slog.Info("shutdown complete")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
