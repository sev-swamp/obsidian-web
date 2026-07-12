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
