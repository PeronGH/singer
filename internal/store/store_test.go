package store

import (
	"os"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewCreatesManual(t *testing.T) {
	s := testStore(t)
	_, err := os.Stat(filepath.Join(s.dir, "manual.txt"))
	if err != nil {
		t.Fatal("manual.txt not created:", err)
	}
}

func TestLoadGroupsManualFirst(t *testing.T) {
	s := testStore(t)
	// Create another group.
	if err := os.WriteFile(filepath.Join(s.dir, "zz.txt"), []byte("socks://10.0.0.1:1080#zz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	groups, err := s.LoadGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) < 2 {
		t.Fatalf("got %d groups, want >= 2", len(groups))
	}
	if groups[0].Name != "manual" {
		t.Fatalf("first group = %q, want manual", groups[0].Name)
	}
	if groups[1].Name != "zz" {
		t.Fatalf("second group = %q, want zz", groups[1].Name)
	}
	if len(groups[1].Nodes) != 1 {
		t.Fatalf("zz nodes = %d, want 1", len(groups[1].Nodes))
	}
}

func TestSaveAndLoadLocalGroup(t *testing.T) {
	s := testStore(t)
	g := Group{
		Name:  "manual",
		Nodes: []string{"socks://10.0.0.1:1080#node1", "http://10.0.0.2:8080#node2"},
	}
	if err := s.SaveGroup(g); err != nil {
		t.Fatal(err)
	}

	groups, err := s.LoadGroups()
	if err != nil {
		t.Fatal(err)
	}
	manual := groups[0]
	if manual.Name != "manual" {
		t.Fatalf("name = %q", manual.Name)
	}
	if len(manual.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(manual.Nodes))
	}
	if manual.IsSubscription() {
		t.Fatal("manual should not be a subscription")
	}
}

func TestSaveAndLoadSubscriptionGroup(t *testing.T) {
	s := testStore(t)
	g := Group{
		Name:  "provider",
		Nodes: []string{"trojan://pass@example.com:443#node1"},
		URL:   "https://example.com/sub.txt",
	}
	if err := s.SaveGroup(g); err != nil {
		t.Fatal(err)
	}

	groups, err := s.LoadGroups()
	if err != nil {
		t.Fatal(err)
	}

	var found *Group
	for i := range groups {
		if groups[i].Name == "provider" {
			found = &groups[i]
			break
		}
	}
	if found == nil {
		t.Fatal("provider group not found")
	}
	if !found.IsSubscription() {
		t.Fatal("should be a subscription")
	}
	if found.URL != "https://example.com/sub.txt" {
		t.Fatalf("url = %q", found.URL)
	}
	if len(found.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(found.Nodes))
	}
}

func TestDeleteGroup(t *testing.T) {
	s := testStore(t)
	g := Group{
		Name:  "deleteme",
		Nodes: []string{"socks://10.0.0.1:1080#x"},
		URL:   "https://example.com/sub.txt",
	}
	if err := s.SaveGroup(g); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteGroup("deleteme"); err != nil {
		t.Fatal(err)
	}

	// Both files should be gone.
	if _, err := os.Stat(filepath.Join(s.dir, "deleteme.txt")); !os.IsNotExist(err) {
		t.Fatal("deleteme.txt still exists")
	}
	if _, err := os.Stat(filepath.Join(s.dir, "deleteme.url.txt")); !os.IsNotExist(err) {
		t.Fatal("deleteme.url.txt still exists")
	}
}

func TestDeleteNonexistentGroup(t *testing.T) {
	s := testStore(t)
	err := s.DeleteGroup("nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent group")
	}
}

func TestUrlTxtIgnoredAsGroup(t *testing.T) {
	s := testStore(t)
	// Write a .url.txt without a matching .txt — should not appear as a group.
	if err := os.WriteFile(filepath.Join(s.dir, "orphan.url.txt"), []byte("https://example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	groups, err := s.LoadGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if g.Name == "orphan.url" || g.Name == "orphan" {
			t.Fatalf("orphan .url.txt appeared as group %q", g.Name)
		}
	}
}

func TestSaveEmptyGroup(t *testing.T) {
	s := testStore(t)
	g := Group{Name: "empty", Nodes: nil}
	if err := s.SaveGroup(g); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(s.dir, "empty.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty file, got %q", data)
	}
}
