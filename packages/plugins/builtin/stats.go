// Package builtin ships first-party plugins that double as reference
// implementations of the Plugin SDK.
package builtin

import (
	"encoding/json"
	"net/http"

	pluginsdk "github.com/obsidianweb/obsidianweb/sdk/plugin-sdk"
)

// StatsPlugin exposes vault statistics at /api/plugins/vault-stats/summary.
type StatsPlugin struct {
	host pluginsdk.Host
}

var _ pluginsdk.Plugin = (*StatsPlugin)(nil)

func (p *StatsPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:          "vault-stats",
		Name:        "Vault Statistics",
		Version:     "1.0.0",
		Description: "Aggregate counters for notes, attachments and links.",
		APIVersion:  pluginsdk.APIVersion,
	}
}

func (p *StatsPlugin) Init(host pluginsdk.Host) error {
	p.host = host
	host.Routes().Handle(http.MethodGet, "/summary", func(w http.ResponseWriter, r *http.Request) {
		stats, err := host.Notes().Stats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	})
	return nil
}

func (p *StatsPlugin) Close() error { return nil }
