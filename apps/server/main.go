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
	"github.com/obsidianweb/obsidianweb/packages/acl"
	"github.com/obsidianweb/obsidianweb/packages/api"
	"github.com/obsidianweb/obsidianweb/packages/auth"
	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/filesystem"
	"github.com/obsidianweb/obsidianweb/packages/history"
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

	// Team accounts, groups, folder ACL and plugin state (hot-reloadable
	// users.yaml). Loaded before the note service because the template
	// engine resolves its folder from plugin settings.
	aclStore, err := acl.Load(cfg.Auth.UsersFile)
	if err != nil {
		return fmt.Errorf("users file: %w", err)
	}

	// The templates folder is a plugin setting; the config value is the
	// default. Resolved per call so admin edits apply without restart.
	templatesDir := func() string {
		if v := aclStore.PluginSettings("templates")["folder"]; v != "" {
			return v
		}
		return cfg.Vault.TemplatesDir
	}
	templateEngine := templates.NewEngineFunc(vault, templatesDir)
	notes := core.NewNoteService(vault, renderer, linkIndex, searchIndex, templateEngine, bus, cfg.Notes, log)

	if cfg.History.Enabled && cfg.History.Mode != "off" {
		hist, err := history.Open(vault.Root(), cfg.History.Mode, log)
		if err != nil {
			log.Warn("history disabled", "error", err)
		} else {
			notes.AttachHistory(hist, time.Duration(cfg.History.ExternalDebounceSec)*time.Second)
		}
	}

	if cfg.Auth.Enabled && cfg.Auth.JWTSecret == "" {
		return errors.New("auth.enabled requires auth.jwtSecret (or OBSIDIANWEB_JWT_SECRET)")
	}
	users, err := buildUsers(cfg)
	if err != nil {
		return err
	}
	authService := auth.NewService(cfg.Auth.Enabled, cfg.Auth.JWTSecret,
		time.Duration(cfg.Auth.TokenTTLHours)*time.Hour, users)

	// Seed the three built-in roles and let sessions resolve permissions
	// from the (customizable) role definitions.
	if err := aclStore.SeedRoles(defaultRoleRecords()); err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}
	authService.SetRoleResolver(aclStore.PermissionsForRole)

	var wsAccess websocket.AccessFunc
	if cfg.Auth.Enabled {
		wsAccess = func(username, path string) bool {
			return aclStore.Access(username, path) >= acl.AccessRead
		}
	}
	hub := websocket.NewHub(bus, wsAccess, log)

	pluginManager := plugins.NewManager(bus, notes, vault, log)
	pluginManager.SetSettingsSource(aclStore.PluginSettings)
	pluginManager.Register(&builtin.StatsPlugin{})
	pluginManager.Register(builtin.NewTemplatesPlugin(cfg.Vault.TemplatesDir))
	pluginManager.RegisterUI(plugins.UIPlugin{
		ID:          "recent-changes",
		Name:        "Recent changes",
		Version:     "1.0.0",
		Description: "Sidebar section with the most recently modified notes.",
	})

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
		ACL:      aclStore,
		Hub:      hub,
		Plugins:  pluginManager,
		Obsidian: obsidian.New(vault.Root()),
		Bus:      bus,
		WebFS:    frontendFS(cfg, log),
		Log:      log,
	}

	httpServer := &http.Server{Addr: cfg.Server.Addr, Handler: server.Router()}
	serverErr := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.Server.Addr, "vault", vault.Root(), "auth", cfg.Auth.Enabled)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM; a failed listener (port in
	// use…) unwinds through the same path so deferred cleanup runs.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		pluginManager.CloseAll()
		return fmt.Errorf("http server: %w", err)
	case <-stop:
	}
	log.Info("shutting down")
	pluginManager.CloseAll()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

// buildUsers assembles all accounts from the config (admin + auth.users)
// and fails fast on duplicates, unknown roles or missing credentials.
func buildUsers(cfg *settings.Config) ([]auth.User, error) {
	users := []auth.User{{
		Username:     cfg.Auth.Admin.Username,
		Password:     cfg.Auth.Admin.Password,
		PasswordHash: cfg.Auth.Admin.PasswordHash,
		Role:         auth.RoleAdmin,
	}}
	seen := map[string]bool{cfg.Auth.Admin.Username: true}
	for i, u := range cfg.Auth.Users {
		if u.Username == "" {
			return nil, fmt.Errorf("auth.users[%d]: username is required", i)
		}
		if seen[u.Username] {
			return nil, fmt.Errorf("auth.users[%d]: duplicate username %q", i, u.Username)
		}
		seen[u.Username] = true
		if cfg.Auth.Enabled && u.Password == "" && u.PasswordHash == "" {
			return nil, fmt.Errorf("auth.users[%d] (%s): password or passwordHash is required", i, u.Username)
		}
		if u.Role != "" && !auth.ValidRole(u.Role) {
			return nil, fmt.Errorf("auth.users[%d] (%s): unknown role %q (viewer|editor|admin)", i, u.Username, u.Role)
		}
		users = append(users, auth.User{
			Username:     u.Username,
			Password:     u.Password,
			PasswordHash: u.PasswordHash,
			Role:         u.Role,
		})
	}
	return users, nil
}

// defaultRoleRecords converts the built-in role definitions into store
// records for seeding users.yaml on first run.
func defaultRoleRecords() []acl.RoleRecord {
	defs := auth.DefaultRoles()
	out := make([]acl.RoleRecord, len(defs))
	for i, d := range defs {
		out[i] = acl.RoleRecord{Name: d.Name, Description: d.Description, Permissions: d.Permissions}
	}
	return out
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
