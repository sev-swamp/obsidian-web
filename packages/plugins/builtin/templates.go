package builtin

import (
	"encoding/json"
	"net/http"

	pluginsdk "github.com/obsidianweb/obsidianweb/sdk/plugin-sdk"
)

// TemplatesPlugin owns the note-template feature: it lists templates at
// /api/plugins/templates/list and declares the "folder" setting the
// template engine reads through the host wiring. Disabling the plugin
// hides templates from the UI and 404s the endpoint.
type TemplatesPlugin struct {
	host pluginsdk.Host
	// defaultDir seeds the setting default (vault.templatesDir from
	// config.yaml).
	defaultDir string
}

var _ pluginsdk.Plugin = (*TemplatesPlugin)(nil)

// NewTemplatesPlugin creates the plugin with the config-provided
// default templates folder.
func NewTemplatesPlugin(defaultDir string) *TemplatesPlugin {
	return &TemplatesPlugin{defaultDir: defaultDir}
}

func (p *TemplatesPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:          "templates",
		Name:        "Templates",
		Version:     "1.0.0",
		Description: "New-note templates from a vault folder ({{date}}, {{time}}, {{title}} variables).",
		APIVersion:  pluginsdk.APIVersion,
		Settings: []pluginsdk.SettingSpec{
			{Key: "folder", Label: "Templates folder", Default: p.defaultDir},
		},
	}
}

func (p *TemplatesPlugin) Init(host pluginsdk.Host) error {
	p.host = host
	host.Routes().Handle(http.MethodGet, "/list", func(w http.ResponseWriter, r *http.Request) {
		names, err := host.Notes().Templates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if names == nil {
			names = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(names)
	})
	return nil
}

func (p *TemplatesPlugin) Close() error { return nil }
