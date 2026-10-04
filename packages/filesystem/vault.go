// Package filesystem implements core.VaultFS over a local directory.
// The vault directory is the single source of truth; nothing is copied.
package filesystem

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// ErrOutsideVault is returned when a path escapes the vault root.
var ErrOutsideVault = errors.New("path escapes vault root")

// Vault provides sandboxed access to an Obsidian vault directory.
type Vault struct {
	root string
}

// NewVault validates the directory and returns a Vault.
func NewVault(root string) (*Vault, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("vault path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault path %q is not a directory", abs)
	}
	return &Vault{root: abs}, nil
}

// Root returns the absolute vault root.
func (v *Vault) Root() string { return v.root }

// CanonicalPath normalizes a vault-relative file path into its stable slash
// representation. It rejects the vault root and paths that escape it. Sync
// uses this value as its durable path identity; callers that need directories
// should continue to use AbsPath.
func (v *Vault) CanonicalPath(rel string) (string, error) {
	// API paths always use slash form. Treat a backslash as a separator even on
	// Unix, where filepath.ToSlash intentionally leaves it untouched.
	rel = strings.TrimSpace(strings.ReplaceAll(filepath.ToSlash(rel), `\`, "/"))
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", ErrOutsideVault
		}
	}
	p := path.Clean("/" + rel)
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." {
		return "", fmt.Errorf("vault-relative file path is required")
	}
	if _, err := v.AbsPath(p); err != nil {
		return "", err
	}
	return p, nil
}

// AbsPath resolves and validates a vault-relative path.
func (v *Vault) AbsPath(rel string) (string, error) {
	rel = filepath.FromSlash(strings.TrimPrefix(rel, "/"))
	abs := filepath.Clean(filepath.Join(v.root, rel))
	if abs != v.root && !strings.HasPrefix(abs, v.root+string(filepath.Separator)) {
		return "", ErrOutsideVault
	}
	return abs, nil
}

// Rel converts an absolute path back to a vault-relative slash path.
func (v *Vault) Rel(abs string) (string, bool) {
	rel, err := filepath.Rel(v.root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// notFound translates the adapter's missing-file errors into the
// implementation-agnostic core.ErrNotFound (keeping the original chained).
func notFound(path string, err error) error {
	if os.IsNotExist(err) {
		return fmt.Errorf("%s: %w (%w)", path, core.ErrNotFound, err)
	}
	return err
}

func (v *Vault) Read(path string) ([]byte, error) {
	abs, err := v.AbsPath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, notFound(path, err)
	}
	return data, nil
}

func (v *Vault) Write(path string, data []byte) error {
	abs, err := v.AbsPath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}

// WriteAtomic writes data durably through a same-directory temporary file.
// It is intended for operations that need crash recovery outside the file
// system (notably sync intents). Existing Vault.Write keeps its historical
// semantics for normal note saves.
func (v *Vault) WriteAtomic(p string, data []byte) error {
	p, err := v.CanonicalPath(p)
	if err != nil {
		return err
	}
	abs, err := v.AbsPath(p)
	if err != nil {
		return err
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".obsidianweb-sync-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return err
	}
	return syncDir(dir)
}

func (v *Vault) Mkdir(path string) error {
	abs, err := v.AbsPath(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

func (v *Vault) Delete(path string) error {
	abs, err := v.AbsPath(path)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		return notFound(path, err)
	}
	return nil
}

// DeleteAtomic removes a file and synchronizes the containing directory so a
// caller can safely finalise an external durable transaction afterwards.
func (v *Vault) DeleteAtomic(p string) error {
	p, err := v.CanonicalPath(p)
	if err != nil {
		return err
	}
	abs, err := v.AbsPath(p)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		return notFound(p, err)
	}
	return syncDir(filepath.Dir(abs))
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// FileState is the observed durable state used to match a controlled write
// against its watcher notifications.
type FileState struct {
	Exists bool   `json:"exists"`
	Hash   string `json:"hash,omitempty"`
}

func fileState(data []byte) FileState {
	sum := sha256.Sum256(data)
	return FileState{Exists: true, Hash: fmt.Sprintf("%x", sum)}
}

func (v *Vault) Exists(path string) bool {
	abs, err := v.AbsPath(path)
	if err != nil {
		return false
	}
	_, err = os.Stat(abs)
	return err == nil
}

func (v *Vault) Stat(path string) (core.FileInfo, error) {
	abs, err := v.AbsPath(path)
	if err != nil {
		return core.FileInfo{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return core.FileInfo{}, notFound(path, err)
	}
	rel, _ := v.Rel(abs)
	return fileInfo(rel, info), nil
}

func (v *Vault) List(dir string) ([]core.FileInfo, error) {
	abs, err := v.AbsPath(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	var out []core.FileInfo
	for _, e := range entries {
		if hidden(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		rel, _ := v.Rel(filepath.Join(abs, e.Name()))
		out = append(out, fileInfo(rel, info))
	}
	sortInfos(out)
	return out, nil
}

// Walk visits every visible file and directory in the vault.
func (v *Vault) Walk(fn func(info core.FileInfo) error) error {
	return filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if p == v.root {
			return nil
		}
		if hidden(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, ok := v.Rel(p)
		if !ok {
			return nil
		}
		return fn(fileInfo(rel, info))
	})
}

// Tree builds the visible directory tree of the vault.
func (v *Vault) Tree() (*core.TreeNode, error) {
	root := &core.TreeNode{Name: filepath.Base(v.root), Path: "", IsDir: true}
	nodes := map[string]*core.TreeNode{"": root}
	err := v.Walk(func(info core.FileInfo) error {
		parentPath := filepath.ToSlash(filepath.Dir(info.Path))
		if parentPath == "." {
			parentPath = ""
		}
		parent, ok := nodes[parentPath]
		if !ok {
			return nil
		}
		node := &core.TreeNode{Name: info.Name, Path: info.Path, IsDir: info.IsDir}
		parent.Children = append(parent.Children, node)
		if info.IsDir {
			nodes[info.Path] = node
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortTree(root)
	return root, nil
}

func sortTree(n *core.TreeNode) {
	sort.Slice(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir // directories first
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	for _, c := range n.Children {
		if c.IsDir {
			sortTree(c)
		}
	}
}

func sortInfos(infos []core.FileInfo) {
	sort.Slice(infos, func(i, j int) bool {
		a, b := infos[i], infos[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

func fileInfo(rel string, info fs.FileInfo) core.FileInfo {
	return core.FileInfo{
		Path:    rel,
		Name:    info.Name(),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
}

func hidden(name string) bool {
	return strings.HasPrefix(name, ".")
}
