// Command server is the Obsidian Web backend: a single binary serving
// the REST API, WebSocket events and the embedded frontend.
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

	"github.com/obsidianweb/obsidianweb/apps/web"
	"github.com/obsidianweb/obsidianweb/packages/api"
	"github.com/obsidianweb/obsidianweb/packages/auth"
	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/filesystem"
	"github.com/obsidianweb/obsidianweb/packages/links"
	"github.com/obsidianweb/obsidianweb/packages/markdown"
	"github.com/obsidianweb/obsidianweb/packages/obsidian"
	"github.com/obsidianweb/obsidianweb/packages/plugins"
	"github.com/obsidianweb/obsidianweb/packages/plugins/builtin"
	"github.com/obsidianweb/obsidianweb/packages/search"
	"github.com/obsidianweb/obsidianweb/packages/settings"
	"github.com/obsidianweb/obsidianweb/packages/templates"
	"github.com/obsidianweb/obsidianweb/packages/websocket"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the YAML config file")
	vaultPath := flag.String("vault", "", "override vault path")
	flag.Parse()

	if err := run(*configPath, *vaultPath); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(configPath, vaultOverride string) error {
	cfg, err := settings.Load(configPath)
	if err != nil {
		return err
	}
	if vaultOverride != "" {
		cfg.Vault.Path = vaultOverride
	}

	log := newLogger(cfg.Log.Level)
	slog.SetDefault(log)

	// --- composition root: wire modules through interfaces -------------
	vault, err := filesystem.NewVault(cfg.Vault.Path)
	if err != nil {
		return err
	}
	bus := core.NewEventBus()
	linkIndex := links.NewIndex()
	searchIndex := search.NewIndex()
	renderer := markdown.NewRenderer(linkIndex)
	templateEngine := templates.NewEngine(vault, cfg.Vault.TemplatesDir)
	notes := core.NewNoteService(vault, renderer, linkIndex, searchIndex, templateEngine, bus, cfg.Notes, log)

	if cfg.Auth.Enabled && cfg.Auth.JWTSecret == "" {
		return errors.New("auth.enabled requires auth.jwtSecret (or OBSIDIANWEB_JWT_SECRET)")
	}
	authService := auth.NewService(cfg.Auth.Enabled, cfg.Auth.JWTSecret,
		time.Duration(cfg.Auth.TokenTTLHours)*time.Hour,
		[]auth.User{{
			Username:     cfg.Auth.Admin.Username,
			Password:     cfg.Auth.Admin.Password,
			PasswordHash: cfg.Auth.Admin.PasswordHash,
			Role:         auth.RoleAdmin,
		}})

	hub := websocket.NewHub(bus, log)

	pluginManager := plugins.NewManager(bus, notes, vault, log)
	pluginManager.Register(&builtin.StatsPlugin{})

	// Initial index, then keep it fresh through the file watcher.
	if err := notes.ReindexAll(); err != nil {
		log.Warn("initial indexing finished with errors", "error", err)
	}
	watcher, err := filesystem.NewWatcher(vault, log, notes.HandleFSEvent)
	if err != nil {
		return err
	}
	if err := watcher.Start(); err != nil {
		return err
	}
	defer watcher.Close()

	server := &api.Server{
		Notes:    notes,
		Vault:    vault,
		Config:   cfg,
		Auth:     authService,
		Hub:      hub,
		Plugins:  pluginManager,
		Obsidian: obsidian.New(vault.Root()),
		WebFS:    frontendFS(cfg, log),
		Log:      log,
	}

	httpServer := &http.Server{Addr: cfg.Server.Addr, Handler: server.Router()}
	go func() {
		log.Info("server listening", "addr", cfg.Server.Addr, "vault", vault.Root(), "auth", cfg.Auth.Enabled)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
	pluginManager.CloseAll()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

// frontendFS prefers an on-disk build (development) over the embedded one.
func frontendFS(cfg *settings.Config, log *slog.Logger) fs.FS {
	if cfg.Web.StaticDir != "" {
		if _, err := os.Stat(cfg.Web.StaticDir); err == nil {
			log.Info("serving frontend from directory", "dir", cfg.Web.StaticDir)
			return os.DirFS(cfg.Web.StaticDir)
		}
		log.Warn("web.staticDir not found, falling back to embedded frontend", "dir", cfg.Web.StaticDir)
	}
	return web.FS()
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
