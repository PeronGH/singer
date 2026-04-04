package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	localGroupName = "local"
	nodesFileName  = "nodes"
	urlFileName    = "url"
)

// GroupKind describes the source of a group.
type GroupKind int

const (
	GroupKindLocal GroupKind = iota
	GroupKindRemote
)

// Group represents one proxy group loaded from disk.
type Group struct {
	Name  string
	Kind  GroupKind
	Nodes []string
	URL   string
}

func (g Group) IsLocal() bool {
	return g.Kind == GroupKindLocal
}

func (g Group) IsRemote() bool {
	return g.Kind == GroupKindRemote
}

// Store manages proxy group directories on disk.
type Store struct {
	dir string
}

// New creates a Store rooted at $XDG_DATA_HOME/singer (default ~/.local/share/singer).
// It also normalizes the on-disk layout and ensures the built-in local group exists.
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

	s := &Store{dir: dir}
	if err := s.normalize(); err != nil {
		return nil, err
	}
	return s, nil
}

// ValidateRemoteName validates a remote group name.
func ValidateRemoteName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return errors.New("group name cannot be empty")
	case name == localGroupName:
		return fmt.Errorf("%q is reserved", localGroupName)
	case !validGroupName(name):
		return errors.New("group name may only contain letters, digits, dot, underscore, and hyphen")
	default:
		return nil
	}
}

// LoadGroups reads every valid group directory, returning local first and remotes sorted by name.
func (s *Store) LoadGroups() ([]Group, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	remotes := make([]Group, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		switch {
		case name == localGroupName:
			// Loaded separately to guarantee it stays first.
		case !validGroupName(name):
			continue
		default:
			g, err := s.loadRemote(name)
			if err != nil {
				return nil, fmt.Errorf("loading remote %q: %w", name, err)
			}
			remotes = append(remotes, g)
		}
	}

	slices.SortFunc(remotes, func(a, b Group) int {
		return strings.Compare(a.Name, b.Name)
	})

	local, err := s.loadLocal()
	if err != nil {
		return nil, fmt.Errorf("loading local group: %w", err)
	}

	return append([]Group{local}, remotes...), nil
}

// SaveLocal writes the editable local group.
func (s *Store) SaveLocal(nodes []string) error {
	if err := s.ensureLocal(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, localGroupName, nodesFileName), marshalNodes(nodes), 0o644)
}

// UpsertRemote creates or replaces one remote-backed group.
func (s *Store) UpsertRemote(name, url string, nodes []string) error {
	if err := ValidateRemoteName(name); err != nil {
		return err
	}

	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("remote URL cannot be empty")
	}

	groupDir := filepath.Join(s.dir, name)
	info, err := os.Stat(groupDir)
	switch {
	case err == nil && !info.IsDir():
		if err := os.RemoveAll(groupDir); err != nil {
			return err
		}
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return err
	}

	if err := os.MkdirAll(groupDir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(groupDir, nodesFileName), marshalNodes(nodes), 0o644); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(groupDir, urlFileName), []byte(url+"\n"), 0o644)
}

// DeleteRemote removes one remote-backed group directory.
func (s *Store) DeleteRemote(name string) error {
	if name == localGroupName {
		return fmt.Errorf("cannot delete %q", localGroupName)
	}
	if err := ValidateRemoteName(name); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.dir, name))
}

func (s *Store) normalize() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		switch {
		case name == localGroupName:
			// Verified separately to keep the rules simpler.
		case !validGroupName(name):
			if err := os.RemoveAll(filepath.Join(s.dir, name)); err != nil {
				return err
			}
		default:
			if err := s.normalizeRemote(name); err != nil {
				return err
			}
		}
	}

	return s.ensureLocal()
}

func (s *Store) ensureLocal() error {
	localDir := filepath.Join(s.dir, localGroupName)
	info, err := os.Stat(localDir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s.recreateLocal(localDir)
	case err != nil:
		return err
	case !info.IsDir():
		if err := os.RemoveAll(localDir); err != nil {
			return err
		}
		return s.recreateLocal(localDir)
	}

	if _, err := os.Stat(filepath.Join(localDir, urlFileName)); err == nil {
		if err := os.RemoveAll(localDir); err != nil {
			return err
		}
		return s.recreateLocal(localDir)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	nodesPath := filepath.Join(localDir, nodesFileName)
	nodesInfo, err := os.Stat(nodesPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return os.WriteFile(nodesPath, nil, 0o644)
	case err != nil:
		return err
	case !nodesInfo.Mode().IsRegular():
		if err := os.RemoveAll(nodesPath); err != nil {
			return err
		}
		return os.WriteFile(nodesPath, nil, 0o644)
	default:
		return nil
	}
}

func (s *Store) recreateLocal(localDir string) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(localDir, nodesFileName), nil, 0o644)
}

func (s *Store) normalizeRemote(name string) error {
	groupDir := filepath.Join(s.dir, name)
	nodesPath := filepath.Join(groupDir, nodesFileName)
	urlPath := filepath.Join(groupDir, urlFileName)

	nodesInfo, err := os.Stat(nodesPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return os.RemoveAll(groupDir)
	case err != nil:
		return err
	case !nodesInfo.Mode().IsRegular():
		return os.RemoveAll(groupDir)
	}

	urlInfo, err := os.Stat(urlPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return os.RemoveAll(groupDir)
	case err != nil:
		return err
	case !urlInfo.Mode().IsRegular():
		return os.RemoveAll(groupDir)
	}

	urlData, err := os.ReadFile(urlPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(urlData)) == "" {
		return os.RemoveAll(groupDir)
	}

	return nil
}

func (s *Store) loadLocal() (Group, error) {
	nodes, err := s.loadNodes(filepath.Join(s.dir, localGroupName, nodesFileName))
	if err != nil {
		return Group{}, err
	}

	return Group{
		Name:  localGroupName,
		Kind:  GroupKindLocal,
		Nodes: nodes,
	}, nil
}

func (s *Store) loadRemote(name string) (Group, error) {
	groupDir := filepath.Join(s.dir, name)

	nodes, err := s.loadNodes(filepath.Join(groupDir, nodesFileName))
	if err != nil {
		return Group{}, err
	}

	urlData, err := os.ReadFile(filepath.Join(groupDir, urlFileName))
	if err != nil {
		return Group{}, err
	}

	return Group{
		Name:  name,
		Kind:  GroupKindRemote,
		Nodes: nodes,
		URL:   strings.TrimSpace(string(urlData)),
	}, nil
}

func (s *Store) loadNodes(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	nodes := make([]string, 0, strings.Count(string(data), "\n")+1)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			nodes = append(nodes, line)
		}
	}

	return nodes, nil
}

func marshalNodes(nodes []string) []byte {
	content := strings.Join(nodes, "\n")
	if len(nodes) > 0 {
		content += "\n"
	}
	return []byte(content)
}

func validGroupName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return false
		}
	}

	return true
}
