package main

import (
	"context"
	"embed"
	"html/template"
	"io/fs"
	"path/filepath"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"os/exec"
	"strings"

	"tideflow/internal/config"
	"tideflow/internal/enforcer"
	"tideflow/internal/handlers"
	"tideflow/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed app/static
var staticFS embed.FS

//go:embed app/templates/index.html
var templateFS embed.FS

// Build-time variables, injected via -ldflags.
var (
	version   = "dev"
	commitSHA = "unknown"
	buildTime = "unknown"
)

func init() {
	// Fallback: read VERSION file when not built with ldflags.
	if version == "dev" {
		if data, err := os.ReadFile(filepath.Join(".", "VERSION")); err == nil {
			version = strings.TrimSpace(string(data))
		}
	}
	// Fallback: try to get git commit when not built with ldflags.
	if commitSHA == "unknown" {
		if out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
			commitSHA = strings.TrimSpace(string(out))
		}
	}
}

func main() {
	config.Init()

	// File-based store
	store, err := storage.New(config.DataDir)
	if err != nil {
		log.Fatalf("Storage init failed: %v", err)
	}

	// Enforcer engine
	eng := enforcer.NewEngine(store)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go eng.Run(ctx)

	// Handlers
	downloadsH := &handlers.DownloadsHandler{Engine: eng}
	sourcesH := &handlers.SourcesHandler{Store: store, Engine: eng}
	statsH := &handlers.StatsHandler{Store: store, Engine: eng}
	settingsH := &handlers.SettingsHandler{Store: store}

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Static files (embedded)
	staticSub, _ := fs.Sub(staticFS, "app/static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Template with custom delimiters for Vue.js compatibility (embedded)
	tmpl := template.Must(template.New("index.html").Delims("{[{", "}]}").ParseFS(templateFS, "app/templates/index.html"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.ExecuteTemplate(w, "index.html", map[string]interface{}{"request": r}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Health
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "name": "TideFlow"})
	})

	// Version
	r.Get("/api/version", func(w http.ResponseWriter, r *http.Request) {
		handlers.WriteJSON(w, http.StatusOK, map[string]string{
			"version": version,
			"commit":  commitSHA,
			"built":   buildTime,
		})
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Auth
		r.Post("/auth", handlers.HandleAuth)

		// Downloads
		r.Post("/downloads/pause", downloadsH.HandlePause)
		r.Post("/downloads/resume", downloadsH.HandleResume)

		// Sources
		r.Get("/sources", sourcesH.ListSources)
		r.Post("/sources/test", sourcesH.TestURL)
		r.Post("/sources", sourcesH.CreateSource)
		r.Put("/sources/{id}", sourcesH.UpdateSource)
		r.Delete("/sources/{id}", sourcesH.DeleteSource)
		r.Post("/sources/clear-cooldowns", sourcesH.ClearCooldowns)

		// Stats
		r.Get("/stats", statsH.HandleStats)
		r.Get("/stats/traffic", statsH.HandleTrafficHistory)

		// Settings
		r.Get("/settings", settingsH.HandleGetSettings)
		r.Put("/settings", settingsH.HandleUpdateSettings)
		r.Get("/settings/defaults", settingsH.HandleGetDefaults)
	})

	// Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		log.Println("Shutting down server...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("TideFlow starting on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("TideFlow shut down")
}
