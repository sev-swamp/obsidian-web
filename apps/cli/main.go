// Command cli is the Obsidian Web console utility: vault indexing
// statistics, broken-link checking and static HTML export.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/filesystem"
	"github.com/obsidianweb/obsidianweb/packages/links"
	"github.com/obsidianweb/obsidianweb/packages/markdown"
	"github.com/obsidianweb/obsidianweb/packages/search"
	"github.com/obsidianweb/obsidianweb/packages/templates"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Obsidian Web CLI

Usage:
  cli -vault <path> <command> [options]

Commands:
  index         index the vault and print statistics
  check-links   list broken wiki-links
  export -out <dir>   export all notes as HTML

Standalone (no -vault needed):
  hash-password   read a password from stdin, print its bcrypt hash
                  (for auth.users[].passwordHash in config.yaml)
  migrate-sso -users <path>   print the sso-oidc plugin env for the
                  built-in SSO config stored in users.yaml
`)
	os.Exit(2)
}

func main() {
	vaultPath := flag.String("vault", "", "path to the Obsidian vault")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() >= 1 && flag.Arg(0) == "hash-password" {
		runHashPassword()
		return
	}
	if flag.NArg() >= 1 && flag.Arg(0) == "migrate-sso" {
		runMigrateSSO(flag.Args()[1:])
		return
	}
	if *vaultPath == "" || flag.NArg() < 1 {
		usage()
	}
	command := flag.Arg(0)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(log)

	notes, linkIndex, err := buildCore(*vaultPath, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	switch command {
	case "index":
		runIndex(notes)
	case "check-links":
		runCheckLinks(linkIndex)
	case "export":
		exportFlags := flag.NewFlagSet("export", flag.ExitOnError)
		out := exportFlags.String("out", "export", "output directory")
		_ = exportFlags.Parse(flag.Args()[1:])
		runExport(notes, *out)
	default:
		usage()
	}
}

func buildCore(vaultPath string, log *slog.Logger) (*core.NoteService, *links.Index, error) {
	vault, err := filesystem.NewVault(vaultPath)
	if err != nil {
		return nil, nil, err
	}
	linkIndex := links.NewIndex()
	renderer := markdown.NewRenderer(linkIndex)
	notes := core.NewNoteService(
		vault, renderer, linkIndex, search.NewIndex(),
		templates.NewEngine(vault, "Templates"),
		core.NewEventBus(), core.NoteRules{}, log,
	)
	if err := notes.ReindexAll(); err != nil {
		return nil, nil, err
	}
	return notes, linkIndex, nil
}

func runIndex(notes *core.NoteService) {
	stats, err := notes.Stats()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("Notes:        %d\n", stats.Notes)
	fmt.Printf("Attachments:  %d\n", stats.Attachments)
	fmt.Printf("Folders:      %d\n", stats.Folders)
	fmt.Printf("Links:        %d\n", stats.Links)
	fmt.Printf("Broken links: %d\n", stats.BrokenLinks)
}

func runCheckLinks(linkIndex *links.Index) {
	broken := linkIndex.BrokenLinks()
	if len(broken) == 0 {
		fmt.Println("No broken links found.")
		return
	}
	sources := make([]string, 0, len(broken))
	for src := range broken {
		sources = append(sources, src)
	}
	sort.Strings(sources)
	total := 0
	for _, src := range sources {
		fmt.Println(src)
		for _, l := range broken[src] {
			fmt.Printf("  [[%s]]\n", l.Raw)
			total++
		}
	}
	fmt.Printf("\n%d broken link(s) in %d note(s)\n", total, len(sources))
	os.Exit(1)
}

// runHashPassword reads a password from stdin (piped or typed) and
// prints a bcrypt hash suitable for passwordHash fields in config.yaml.
func runHashPassword() {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Password: ")
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(os.Stderr, "error: could not read password from stdin")
		os.Exit(1)
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		fmt.Fprintln(os.Stderr, "error: empty password")
		os.Exit(1)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(string(hash))
}

// runMigrateSSO reads the legacy built-in SSO block from users.yaml and
// prints the equivalent environment for the sso-oidc plugin. It parses
// only the sso block, so it keeps working after acl.SSOConfig is gone,
// and it never modifies the file — the old block is inert and can be
// deleted by hand.
func runMigrateSSO(args []string) {
	fs := flag.NewFlagSet("migrate-sso", flag.ExitOnError)
	usersPath := fs.String("users", "", "path to users.yaml")
	_ = fs.Parse(args)
	if *usersPath == "" {
		fmt.Fprintln(os.Stderr, "error: -users <path> is required")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*usersPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	var data struct {
		SSO *struct {
			Enabled       bool   `yaml:"enabled"`
			Name          string `yaml:"name"`
			Issuer        string `yaml:"issuer"`
			ClientID      string `yaml:"clientId"`
			ClientSecret  string `yaml:"clientSecret"`
			DefaultRole   string `yaml:"defaultRole"`
			AutoProvision bool   `yaml:"autoProvision"`
		} `yaml:"sso"`
	}
	if err := yaml.Unmarshal(raw, &data); err != nil {
		fmt.Fprintln(os.Stderr, "error: parse", *usersPath+":", err)
		os.Exit(1)
	}
	if data.SSO == nil || data.SSO.Issuer == "" {
		fmt.Fprintln(os.Stderr, "no built-in SSO configuration found in", *usersPath)
		os.Exit(1)
	}
	sso := data.SSO
	role := sso.DefaultRole
	if role == "" {
		role = "viewer"
	}
	name := sso.Name
	if name == "" {
		name = "Sign in with SSO"
	}

	fmt.Printf(`# sso-oidc plugin environment migrated from %s
# 1. Create a service token in Settings -> SSO with permissions
#    users:provision and session:code (add login-providers:write to use
#    REGISTER_PROVIDER below), then set SERVICE_TOKEN to svc_<id>_<secret>.
# 2. Set PLATFORM_URL / PLATFORM_PUBLIC_URL / PLUGIN_PUBLIC_URL for your
#    deployment. 3. Delete the now-inert sso: block from users.yaml.
OIDC_ISSUER=%s
OIDC_CLIENT_ID=%s
OIDC_CLIENT_SECRET=%s
DEFAULT_ROLE=%s
REGISTER_PROVIDER=true
PROVIDER_ID=oidc
PROVIDER_NAME=%q
`, *usersPath, sso.Issuer, sso.ClientID, sso.ClientSecret, role, name)
	if !sso.AutoProvision {
		fmt.Fprintln(os.Stderr, "\nnote: the built-in SSO had autoProvision=false; the plugin always "+
			"provisions accounts. Give the service token a low roleCeiling to keep it constrained.")
	}
}

func runExport(notes *core.NoteService, outDir string) {
	metas, err := notes.ListNotes(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	for _, meta := range metas {
		note, err := notes.GetNote(meta.Path, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, "skip:", meta.Path, err)
			continue
		}
		rel := strings.TrimSuffix(meta.Path, ".md") + ".html"
		dest := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		page := "<!doctype html><meta charset=\"utf-8\"><title>" + note.Title + "</title>\n" + note.HTML
		if err := os.WriteFile(dest, []byte(page), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("Exported %d note(s) to %s\n", len(metas), outDir)
}
