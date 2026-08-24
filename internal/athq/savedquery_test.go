package athq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPutAndReadSavedQueries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "queries.jsonl")

	for _, name := range []string{"users", "events"} {
		replaced, err := putSavedQuery(path, savedQuery{Name: name, Description: "about " + name, Query: "SELECT 1"})
		if err != nil {
			t.Fatalf("got error %v, want none", err)
		}
		if replaced {
			t.Errorf("%s: got replaced = true, want a new entry", name)
		}
	}

	queries, err := readSavedQueries(path)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(queries) != 2 {
		t.Fatalf("count: got = %d, want 2", len(queries))
	}
	if queries[0].Name != "events" || queries[1].Name != "users" {
		t.Errorf("got = %q,%q, want them by name", queries[0].Name, queries[1].Name)
	}
	if queries[0].Description != "about events" || queries[0].Query != "SELECT 1" {
		t.Errorf("got = %+v, want the description and the query kept", queries[0])
	}
	if queries[0].SavedAt.IsZero() {
		t.Error("got a zero saved_at, want the time of the save")
	}
}

func TestPutSavedQueryReplacesTheSameName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.jsonl")

	if _, err := putSavedQuery(path, savedQuery{Name: "users", Query: "SELECT 1"}); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	replaced, err := putSavedQuery(path, savedQuery{Name: " users ", Description: "now with a description", Query: "SELECT 2"})
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if !replaced {
		t.Error("got replaced = false, want the earlier entry replaced")
	}

	queries, err := readSavedQueries(path)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(queries) != 1 {
		t.Fatalf("count: got = %d, want 1", len(queries))
	}
	if queries[0].Query != "SELECT 2" || queries[0].Description != "now with a description" {
		t.Errorf("got = %+v, want the new query and description", queries[0])
	}
}

func TestPutSavedQueryRefusesEmptyInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.jsonl")

	if _, err := putSavedQuery(path, savedQuery{Name: "  ", Query: "SELECT 1"}); err == nil {
		t.Error("an empty name: got no error, want one")
	}
	if _, err := putSavedQuery(path, savedQuery{Name: "users", Query: " \n"}); err == nil {
		t.Error("an empty query: got no error, want one")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the file was written, want nothing stored")
	}
}

func TestSavedQueriesAreWrittenUserOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.jsonl")
	if _, err := putSavedQuery(path, savedQuery{Name: "users", Query: "SELECT 1"}); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("got = %v, want 0600", got)
	}
}

func TestReadSavedQueriesSkipsDamagedAndNamelessLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.jsonl")
	content := "{\"name\":\"good\",\"query\":\"SELECT 1\"}\nnot json\n{\"query\":\"SELECT 2\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	queries, err := readSavedQueries(path)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(queries) != 1 || queries[0].Name != "good" {
		t.Errorf("got = %v, want only the usable entry", queries)
	}
}

func TestReadSavedQueriesOfAMissingFile(t *testing.T) {
	queries, err := readSavedQueries(filepath.Join(t.TempDir(), "none.jsonl"))
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(queries) != 0 {
		t.Errorf("count: got = %d, want 0", len(queries))
	}
}

func TestSavedQueriesPathHonoursTheEnvironment(t *testing.T) {
	t.Setenv(envSavedQueriesFile, "/tmp/athq-queries.jsonl")
	got, err := savedQueriesPath()
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != "/tmp/athq-queries.jsonl" {
		t.Errorf("got = %q, want the value from the environment", got)
	}
}

func TestSavedQueriesLiveUnderTheConfigDirectory(t *testing.T) {
	t.Setenv(envSavedQueriesFile, "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/somebody")

	got, err := savedQueriesPath()
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if want := filepath.Join("/home/somebody", ".config", "athq", "queries.jsonl"); got != want {
		t.Errorf("got = %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, err = savedQueriesPath()
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if want := filepath.Join("/xdg", "athq", "queries.jsonl"); got != want {
		t.Errorf("XDG_CONFIG_HOME: got = %q, want %q", got, want)
	}
}

func TestWriteJSONLAtomicLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queries.jsonl")
	if _, err := putSavedQuery(path, savedQuery{Name: "users", Query: "SELECT 1", SavedAt: time.Now()}); err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range names {
		if strings.HasPrefix(e.Name(), ".athq-") {
			t.Errorf("got %q left behind, want only the listing", e.Name())
		}
	}
}
