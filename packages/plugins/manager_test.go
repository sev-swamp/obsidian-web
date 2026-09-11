package plugins

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/obsidianweb/obsidianweb/packages/core"
	pluginsdk "github.com/obsidianweb/obsidianweb/sdk/plugin-sdk"
)

type togglePlugin struct{ enabled, disabled int }

func (p *togglePlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{ID: "toggle", Name: "Toggle", Version: "1", APIVersion: pluginsdk.APIVersion}
}
func (p *togglePlugin) Init(pluginsdk.Host) error { return nil }
func (p *togglePlugin) Enable() error             { p.enabled++; return nil }
func (p *togglePlugin) Disable() error            { p.disabled++; return nil }
func (p *togglePlugin) Close() error              { return nil }

func TestManagerTogglesLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p := &togglePlugin{}
	m := NewManager(core.NewEventBus(), nil, nil, nil)
	m.Register(p)
	r := gin.New()
	if err := m.InitAll(r.Group("/plugins"), func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if p.enabled != 0 {
		t.Fatalf("disabled plugin enabled %d times", p.enabled)
	}
	if err := m.SetEnabled("toggle", true); err != nil {
		t.Fatal(err)
	}
	if p.enabled != 1 {
		t.Fatalf("enable count = %d, want 1", p.enabled)
	}
	if err := m.SetEnabled("toggle", false); err != nil {
		t.Fatal(err)
	}
	if p.disabled != 1 {
		t.Fatalf("disable count = %d, want 1", p.disabled)
	}
	if err := m.SetEnabled("toggle", false); err != nil {
		t.Fatal(err)
	}
	if p.disabled != 1 {
		t.Fatalf("idempotent disable count = %d, want 1", p.disabled)
	}

	// Ensure route setup remains usable after lifecycle handling.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/plugins/toggle/nope", nil))
	if w.Code != 404 {
		t.Fatalf("route status = %d, want 404", w.Code)
	}
}

func TestManagerRestartActiveOnly(t *testing.T) {
	p := &togglePlugin{}
	m := NewManager(core.NewEventBus(), nil, nil, nil)
	m.Register(p)
	r := gin.New()
	if err := m.InitAll(r.Group("/plugins"), nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart("toggle"); err != nil {
		t.Fatal(err)
	}
	if p.enabled != 2 || p.disabled != 1 {
		t.Fatalf("restart counts enabled=%d disabled=%d", p.enabled, p.disabled)
	}
	if err := m.SetEnabled("toggle", false); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart("toggle"); err != nil {
		t.Fatal(err)
	}
	if p.enabled != 2 {
		t.Fatalf("disabled plugin restarted: enabled=%d", p.enabled)
	}
}
