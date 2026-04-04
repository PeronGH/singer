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

func TestNewCreatesLocalNodesFile(t *testing.T) {
	s := testStore(t)

	info, err := os.Stat(filepath.Join(s.dir, localGroupName))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("local is not a directory")
	}

	if _, err := os.Stat(filepath.Join(s.dir, localGroupName, nodesFileName)); err != nil {
		t.Fatal("local/nodes not created:", err)
	}
}

func TestNewRecreatesBrokenLocalDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)

	appDir := filepath.Join(root, "singer")
	if err := os.MkdirAll(filepath.Join(appDir, localGroupName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, localGroupName, urlFileName), []byte("https://example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, localGroupName, "extra"), []byte("keep? no, local is recreated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(s.dir, localGroupName, urlFileName)); !os.IsNotExist(err) {
		t.Fatal("local/url should be removed")
	}

	data, err := os.ReadFile(filepath.Join(s.dir, localGroupName, nodesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("local/nodes should be empty, got %q", data)
	}
}

func TestLoadGroupsReturnsLocalFirstAndRemoteGroups(t *testing.T) {
	s := testStore(t)
	if err := s.UpsertRemote("b", "https://example.com/b", []string{"socks://10.0.0.2:1080#b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRemote("a", "https://example.com/a", []string{"socks://10.0.0.1:1080#a"}); err != nil {
		t.Fatal(err)
	}

	groups, err := s.LoadGroups()
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	if groups[0].Name != localGroupName || !groups[0].IsLocal() {
		t.Fatalf("first group = %+v, want local", groups[0])
	}
	if groups[1].Name != "a" || !groups[1].IsRemote() {
		t.Fatalf("second group = %+v, want remote a", groups[1])
	}
	if groups[2].Name != "b" || !groups[2].IsRemote() {
		t.Fatalf("third group = %+v, want remote b", groups[2])
	}
}

func TestSaveLocalWritesOnlyNodes(t *testing.T) {
	s := testStore(t)
	nodes := []string{"socks://10.0.0.1:1080#one", "http://10.0.0.2:8080#two"}

	if err := s.SaveLocal(nodes); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(s.dir, localGroupName, nodesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "socks://10.0.0.1:1080#one\nhttp://10.0.0.2:8080#two\n" {
		t.Fatalf("unexpected local/nodes content %q", data)
	}

	if _, err := os.Stat(filepath.Join(s.dir, localGroupName, urlFileName)); !os.IsNotExist(err) {
		t.Fatal("local/url should not exist")
	}
}

func TestUpsertRemoteWritesNodesAndURL(t *testing.T) {
	s := testStore(t)
	nodes := []string{"trojan://pass@example.com:443#node1"}

	if err := s.UpsertRemote("provider", "https://example.com/sub.txt", nodes); err != nil {
		t.Fatal(err)
	}

	nodeData, err := os.ReadFile(filepath.Join(s.dir, "provider", nodesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(nodeData) != "trojan://pass@example.com:443#node1\n" {
		t.Fatalf("unexpected provider/nodes content %q", nodeData)
	}

	urlData, err := os.ReadFile(filepath.Join(s.dir, "provider", urlFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(urlData) != "https://example.com/sub.txt\n" {
		t.Fatalf("unexpected provider/url content %q", urlData)
	}
}

func TestDeleteRemoteRemovesDirectory(t *testing.T) {
	s := testStore(t)
	if err := s.UpsertRemote("deleteme", "https://example.com/sub.txt", []string{"socks://10.0.0.1:1080#x"}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteRemote("deleteme"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(s.dir, "deleteme")); !os.IsNotExist(err) {
		t.Fatal("deleteme directory still exists")
	}
}

func TestDeleteRemoteRejectsLocal(t *testing.T) {
	s := testStore(t)
	if err := s.DeleteRemote(localGroupName); err == nil {
		t.Fatal("expected deleting local to fail")
	}
}

func TestValidateRemoteNameRejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"", "local", "with space", "slash/name", ".."} {
		if err := ValidateRemoteName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

func TestNewDeletesInvalidRemoteDirectories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)

	appDir := filepath.Join(root, "singer")
	badNameDir := filepath.Join(appDir, "bad name")
	missingURLDir := filepath.Join(appDir, "missing-url")
	emptyURLDir := filepath.Join(appDir, "empty-url")
	validDir := filepath.Join(appDir, "keep-me")

	for _, dir := range []string{badNameDir, missingURLDir, emptyURLDir, validDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, nodesFileName), []byte("socks://10.0.0.1:1080#x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(emptyURLDir, urlFileName), []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validDir, urlFileName), []byte("https://example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New()
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"bad name", "missing-url", "empty-url"} {
		if _, err := os.Stat(filepath.Join(s.dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should have been deleted", name)
		}
	}
	if _, err := os.Stat(validDir); err != nil {
		t.Fatal("valid remote directory should remain:", err)
	}
}

func TestNewKeepsValidRemoteDirectoriesWithExtraFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)

	appDir := filepath.Join(root, "singer")
	remoteDir := filepath.Join(appDir, "keep-me")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, nodesFileName), []byte("socks://10.0.0.1:1080#x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, urlFileName), []byte("https://example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, "notes"), []byte("leave me alone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(s.dir, "keep-me", "notes")); err != nil {
		t.Fatal("extra file should be preserved:", err)
	}
}
