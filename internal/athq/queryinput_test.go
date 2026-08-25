package athq

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveQueryFromArgument(t *testing.T) {
	got, err := resolveQuery([]string{"SELECT", "1"}, "", false, nil)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != "SELECT 1" {
		t.Errorf("got = %q, want %q", got, "SELECT 1")
	}
}

func TestResolveQueryFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.sql")
	if err := os.WriteFile(path, []byte("  SELECT 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveQuery(nil, path, false, nil)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != "SELECT 1" {
		t.Errorf("got = %q, want %q", got, "SELECT 1")
	}
}

func TestResolveQueryRejectsTwoSources(t *testing.T) {
	if _, err := resolveQuery([]string{"SELECT 1"}, "q.sql", false, nil); err == nil {
		t.Error("got no error for an argument plus --file, want one")
	}
}

func TestResolveQueryWithoutAnySource(t *testing.T) {
	if _, err := resolveQuery(nil, "", false, nil); err == nil {
		t.Error("got no error without a query, want one")
	}
}

func TestResolveQueryRejectsCommentsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.sql")
	if err := os.WriteFile(path, []byte(editorTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveQuery(nil, path, false, nil); err == nil {
		t.Error("got no error for an untouched template, want one")
	}
}

func TestInitialTUIQueryWithNoSourceStartsEmpty(t *testing.T) {
	got, err := initialTUIQuery(nil, "", false, nil)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != "" {
		t.Errorf("got = %q, want an empty string", got)
	}
}

func TestInitialTUIQueryFromArgument(t *testing.T) {
	got, err := initialTUIQuery([]string{"SELECT", "1"}, "", false, nil)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != "SELECT 1" {
		t.Errorf("got = %q, want %q", got, "SELECT 1")
	}
}

func TestInitialTUIQueryFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.sql")
	if err := os.WriteFile(path, []byte("  SELECT 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := initialTUIQuery(nil, path, false, nil)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != "SELECT 1" {
		t.Errorf("got = %q, want %q", got, "SELECT 1")
	}
}

func TestInitialTUIQueryFromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("SELECT 1")
		_ = w.Close()
	}()

	got, err := initialTUIQuery(nil, "", false, r)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != "SELECT 1" {
		t.Errorf("got = %q, want %q", got, "SELECT 1")
	}
}

func TestInitialTUIQueryPropagatesAFileError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.sql")
	if _, err := initialTUIQuery(nil, missing, false, nil); err == nil {
		t.Error("got no error for a missing file, want one")
	}
}

func TestIsBlankQuery(t *testing.T) {
	if !isBlankQuery("\n-- just a comment\n\n") {
		t.Error("comments only: got = false, want true")
	}
	if isBlankQuery("-- comment\nSELECT 1") {
		t.Error("with SQL: got = true, want false")
	}
}

func TestEditorCommandPrefersATHQEditor(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	t.Setenv("ATHQ_EDITOR", "code --wait")
	got := editorCommand()
	if len(got) != 2 || got[0] != "code" || got[1] != "--wait" {
		t.Errorf("got = %v, want [code --wait]", got)
	}
}

func TestEditorCommandFallsBackToVi(t *testing.T) {
	t.Setenv("ATHQ_EDITOR", "")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	got := editorCommand()
	if len(got) != 1 || got[0] != "vi" {
		t.Errorf("got = %v, want [vi]", got)
	}
}
