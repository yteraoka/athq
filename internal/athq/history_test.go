package athq

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndReadHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "history.jsonl")

	for i := 1; i <= 3; i++ {
		err := appendHistory(path, historyEntry{
			Time:             time.Now(),
			QueryExecutionID: fmt.Sprintf("id-%d", i),
			State:            "SUCCEEDED",
			Query:            "SELECT 1",
		})
		if err != nil {
			t.Fatalf("got error %v, want none", err)
		}
	}

	entries, err := readHistory(path, 2)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(entries) != 2 {
		t.Fatalf("count: got = %d, want 2", len(entries))
	}
	if entries[0].QueryExecutionID != "id-2" || entries[1].QueryExecutionID != "id-3" {
		t.Errorf("got = %q,%q, want the newest two in order", entries[0].QueryExecutionID, entries[1].QueryExecutionID)
	}
}

func TestHistoryIsWrittenUserOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := appendHistory(path, historyEntry{QueryExecutionID: "id"}); err != nil {
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

func TestHistoryDropsTheOldestEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")

	entries := make([]historyEntry, 0, maxHistoryEntries+5)
	for i := 0; i < maxHistoryEntries+5; i++ {
		entries = append(entries, historyEntry{QueryExecutionID: fmt.Sprintf("id-%d", i)})
	}
	// Seed everything but the last one directly, then append through the
	// normal path so the trimming runs.
	for _, e := range entries[:len(entries)-1] {
		if err := appendHistory(path, e); err != nil {
			t.Fatalf("got error %v, want none", err)
		}
	}
	if err := appendHistory(path, entries[len(entries)-1]); err != nil {
		t.Fatalf("got error %v, want none", err)
	}

	got, err := readHistory(path, 0)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(got) != maxHistoryEntries {
		t.Fatalf("count: got = %d, want %d", len(got), maxHistoryEntries)
	}
	if got[len(got)-1].QueryExecutionID != fmt.Sprintf("id-%d", maxHistoryEntries+4) {
		t.Errorf("newest: got = %q, want the last appended entry", got[len(got)-1].QueryExecutionID)
	}
}

func TestReadHistoryOfAMissingFile(t *testing.T) {
	entries, err := readHistory(filepath.Join(t.TempDir(), "none.jsonl"), 10)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(entries) != 0 {
		t.Errorf("count: got = %d, want 0", len(entries))
	}
}

func TestReadHistorySkipsDamagedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	content := "{\"query_execution_id\":\"good\"}\nnot json\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := readHistory(path, 0)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(entries) != 1 || entries[0].QueryExecutionID != "good" {
		t.Errorf("got = %v, want only the readable entry", entries)
	}
}

func TestLastSucceededSkipsFailures(t *testing.T) {
	entries := []historyEntry{
		{QueryExecutionID: "old", State: "SUCCEEDED"},
		{QueryExecutionID: "new", State: "SUCCEEDED"},
		{QueryExecutionID: "broken", State: "FAILED"},
	}
	got, ok := lastSucceeded(entries)
	if !ok {
		t.Fatal("got ok = false, want true")
	}
	if got.QueryExecutionID != "new" {
		t.Errorf("got = %q, want %q", got.QueryExecutionID, "new")
	}
}

func TestLastSucceededWithoutAnySuccess(t *testing.T) {
	if _, ok := lastSucceeded([]historyEntry{{State: "FAILED"}}); ok {
		t.Error("got ok = true, want false")
	}
}

func TestHistoryPathHonoursTheEnvironment(t *testing.T) {
	t.Setenv(envHistoryFile, "/tmp/athq-history.jsonl")
	got, err := historyPath()
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if got != "/tmp/athq-history.jsonl" {
		t.Errorf("got = %q, want the value from the environment", got)
	}
}

func TestSummarizeQueryFoldsWhitespace(t *testing.T) {
	if got := summarizeQuery("SELECT *\n  FROM t\nWHERE x = 1"); got != "SELECT * FROM t WHERE x = 1" {
		t.Errorf("got = %q, want it on one line", got)
	}
}
