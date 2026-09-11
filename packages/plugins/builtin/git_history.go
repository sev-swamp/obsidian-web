package builtin

import (
	"fmt"
	"strconv"
	"time"

	"github.com/obsidianweb/obsidianweb/packages/core"
	gitHistory "github.com/obsidianweb/obsidianweb/packages/history"
	pluginsdk "github.com/obsidianweb/obsidianweb/sdk/plugin-sdk"
)

// GitHistoryPlugin owns the optional Git-backed history capability. The
// platform still owns note mutations; enabling this plugin simply attaches a
// History implementation to NoteService.
type GitHistoryPlugin struct {
	host            pluginsdk.Host
	defaultMode     string
	defaultDebounce int
	history         core.History
}

var _ pluginsdk.Plugin = (*GitHistoryPlugin)(nil)
var _ pluginsdk.Toggleable = (*GitHistoryPlugin)(nil)

func NewGitHistoryPlugin(mode string, debounceSec int) *GitHistoryPlugin {
	return &GitHistoryPlugin{defaultMode: mode, defaultDebounce: debounceSec}
}

func (p *GitHistoryPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{ID: "git-history", Name: "Git history", Version: "1.0.0", APIVersion: pluginsdk.APIVersion,
		Description: "Revisions, diffs, restore and Git-backed trash for the vault.",
		Settings: []pluginsdk.SettingSpec{
			{Key: "mode", Label: "Mode (managed or external)", Default: p.defaultMode},
			{Key: "externalDebounceSec", Label: "External edit debounce (seconds)", Default: strconv.Itoa(p.defaultDebounce)},
		},
	}
}

func (p *GitHistoryPlugin) Init(host pluginsdk.Host) error { p.host = host; return nil }

func (p *GitHistoryPlugin) Enable() error {
	if p.host == nil {
		return fmt.Errorf("git-history plugin is not initialized")
	}
	mode := p.host.Settings().Get("mode")
	if mode == "" || mode == "off" {
		return fmt.Errorf("history mode must be managed or external")
	}
	debounce, err := strconv.Atoi(p.host.Settings().Get("externalDebounceSec"))
	if err != nil || debounce < 0 {
		return fmt.Errorf("externalDebounceSec must be a non-negative integer")
	}
	root, err := p.host.Vault().AbsPath("")
	if err != nil {
		return fmt.Errorf("vault root: %w", err)
	}
	h, err := gitHistory.Open(root, mode, p.host.Logger())
	if err != nil {
		return err
	}
	p.history = h
	p.host.Notes().SetHistory(h, time.Duration(debounce)*time.Second)
	return nil
}

func (p *GitHistoryPlugin) Disable() error {
	if p.host != nil && p.history != nil {
		p.host.Notes().DetachHistory(p.history)
	}
	p.history = nil
	return nil
}

func (p *GitHistoryPlugin) Close() error { return p.Disable() }
