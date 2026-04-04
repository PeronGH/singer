package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Group represents a collection of proxy node URIs.
// If URL is non-empty, the group is a subscription (refreshable, not editable).
// If URL is empty, it's a local group (editable, not refreshable).
type Group struct {
	Name  string
	Nodes []string
	URL   string
}

// IsSubscription reports whether this group has an upstream URL.
func (g *Group) IsSubscription() bool {
	return g.URL != ""
}

// Store manages proxy node group files on disk.
type Store struct {
	dir string
}

// New creates a Store rooted at $XDG_DATA_HOME/singer (default ~/.local/share/singer).
// It ensures the directory and manual.txt exist.
func New() (*Store, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	dir = filepath.Join(dir, "singer")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	// Ensure manual.txt exists.
	manualPath := filepath.Join(dir, "manual.txt")
	if _, err := os.Stat(manualPath); os.IsNotExist(err) {
		if err := os.WriteFile(manualPath, nil, 0o644); err != nil {
			return nil, err
		}
	}

	return &Store{dir: dir}, nil
}

// LoadGroups reads all group files. The "manual" group is always first.
func (s *Store) LoadGroups() ([]Group, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var manual *Group
	var groups []Group

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".txt") || strings.HasSuffix(name, ".url.txt") {
			continue
		}

		g, err := s.loadGroup(strings.TrimSuffix(name, ".txt"))
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", name, err)
		}

		if g.Name == "manual" {
			manual = &g
		} else {
			groups = append(groups, g)
		}
	}

	if manual == nil {
		manual = &Group{Name: "manual"}
	}

	return append([]Group{*manual}, groups...), nil
}

func (s *Store) loadGroup(name string) (Group, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, name+".txt"))
	if err != nil {
		return Group{}, err
	}

	var nodes []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			nodes = append(nodes, line)
		}
	}

	g := Group{Name: name, Nodes: nodes}

	urlData, err := os.ReadFile(filepath.Join(s.dir, name+".url.txt"))
	if err == nil {
		g.URL = strings.TrimSpace(string(urlData))
	}

	return g, nil
}

// SaveGroup writes a group's nodes to name.txt.
// If the group has a URL, it also writes name.url.txt.
func (s *Store) SaveGroup(g Group) error {
	content := strings.Join(g.Nodes, "\n")
	if len(g.Nodes) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(filepath.Join(s.dir, g.Name+".txt"), []byte(content), 0o644); err != nil {
		return err
	}

	if g.URL != "" {
		if err := os.WriteFile(filepath.Join(s.dir, g.Name+".url.txt"), []byte(g.URL+"\n"), 0o644); err != nil {
			return err
		}
	}

	return nil
}

// DeleteGroup removes a group's files (name.txt and name.url.txt if present).
func (s *Store) DeleteGroup(name string) error {
	if err := os.Remove(filepath.Join(s.dir, name+".txt")); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.dir, name+".url.txt")) // best-effort
	return nil
}
