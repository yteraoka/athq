package athq

import (
	"strings"
	"testing"
)

// editorTUI is loadedTUI with the focus in the editor and sql already typed,
// which is where tab completes.
func editorTUI(t *testing.T, sql string) tuiModel {
	t.Helper()
	m := insertingEditor(loadedTUI(t))
	m.editor.SetValue(sql)
	m.editor.MoveToEnd()
	return m
}

func TestCompletionStemSplitsOnTheLastDot(t *testing.T) {
	tests := []struct {
		line      string
		qualifier string
		stem      string
	}{
		{"SELECT * FROM ev", "", "ev"},
		{"SELECT * FROM analytics.", "analytics", ""},
		{"SELECT * FROM analytics.ev", "analytics", "ev"},
		{"SELECT analytics.events.ev", "analytics.events", "ev"},
		{"SELECT * FROM ", "", ""},
		{"SELECT a, b_2", "", "b_2"},
	}
	for _, tt := range tests {
		line := []rune(tt.line)
		qualifier, stem := completionStem(line, len(line))
		if qualifier != tt.qualifier || stem != tt.stem {
			t.Errorf("%q: got = %q, %q, want %q, %q", tt.line, qualifier, stem, tt.qualifier, tt.stem)
		}
	}
}

func TestCompletionStemStopsAtTheCursor(t *testing.T) {
	line := []rune("SELECT events FROM x")
	qualifier, stem := completionStem(line, len("SELECT ev"))
	if qualifier != "" || stem != "ev" {
		t.Errorf("got = %q, %q, want \"\", %q", qualifier, stem, "ev")
	}
}

func TestCompletionCandidatesComeFromTheCatalog(t *testing.T) {
	m := loadedTUI(t)

	tests := []struct {
		name      string
		qualifier string
		stem      string
		want      []string
	}{
		// A bare word can be a database or a table of an expanded one.
		{"databases and tables", "", "", []string{"analytics", "events", "logs", "sessions"}},
		{"a prefix of a database", "", "an", []string{"analytics"}},
		{"a prefix shared by tables", "", "s", []string{"sessions"}},
		{"the tables of a database", "analytics", "", []string{"events", "sessions"}},
		{"a prefix of a table", "analytics", "ev", []string{"events"}},
		{"the columns of a table", "analytics.events", "", []string{"dt", "event_id"}},
		{"a database whose tables are unknown", "logs", "", nil},
		{"a database that does not exist", "nope", "", nil},
		{"too many dots", "a.b.c", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.completionCandidates(tt.qualifier, tt.stem)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("got = %v, want %v", got, tt.want)
			}
		})
	}
}

// The columns of the table under the catalog cursor complete without their
// table name, which is what a query that names the table once needs.
func TestCompletionOffersTheColumnsOfTheSelectedTable(t *testing.T) {
	m := loadedTUI(t)
	m.catCursor = 1 // analytics.events
	got := m.completionCandidates("", "event")
	if strings.Join(got, ",") != "event_id,events" {
		t.Errorf("got = %v, want the column and the table", got)
	}
}

func TestTabCompletesAUniqueName(t *testing.T) {
	m := editorTUI(t, "SELECT * FROM anal")
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics" {
		t.Errorf("got = %q, want the database completed", got)
	}
}

func TestTabCompletesAQualifiedName(t *testing.T) {
	m := editorTUI(t, "SELECT * FROM analytics.se")
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics.sessions" {
		t.Errorf("got = %q, want the table completed", got)
	}
}

func TestTabIgnoresTheCaseOfWhatWasTyped(t *testing.T) {
	m := editorTUI(t, "SELECT * FROM ANAL")
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics" {
		t.Errorf("got = %q, want the catalog spelling", got)
	}
}

func TestTabDoesNothingWithoutACandidate(t *testing.T) {
	m := editorTUI(t, "SELECT * FROM zzz")
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM zzz" {
		t.Errorf("got = %q, want the query untouched (no tab character)", got)
	}
	if m.focus != paneEditor {
		t.Errorf("focus: got = %v, want the editor", m.focus)
	}
}

func TestTabCompletesTheCommonPrefixAndListsTheRest(t *testing.T) {
	m := editorTUI(t, "SELECT * FROM analytics.e")
	m.databases[0].tables = append(m.databases[0].tables, tuiTable{name: "events_raw"})

	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics.events" {
		t.Errorf("got = %q, want the common prefix", got)
	}
	if !strings.Contains(m.status, "events") || !strings.Contains(m.status, "events_raw") {
		t.Errorf("status: got = %q, want both candidates listed", m.status)
	}
	if m.statusErr {
		t.Error("the candidate list is not an error")
	}
}

func TestFurtherTabsCycleThroughTheCandidates(t *testing.T) {
	m := editorTUI(t, "SELECT * FROM analytics.e")
	m.databases[0].tables = append(m.databases[0].tables, tuiTable{name: "events_raw"})

	m = pressKey(t, m, "tab") // the common prefix, "events"
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics.events" {
		t.Errorf("first candidate: got = %q", got)
	}
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics.events_raw" {
		t.Errorf("second candidate: got = %q", got)
	}
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics.events" {
		t.Errorf("back to the first: got = %q", got)
	}
}

func TestAnotherKeyEndsTheCompletion(t *testing.T) {
	m := editorTUI(t, "SELECT * FROM analytics.e")
	m.databases[0].tables = append(m.databases[0].tables, tuiTable{name: "events_raw"})

	m = pressKey(t, m, "tab")
	m = pressKey(t, m, "_")
	if m.completion.active() {
		t.Error("typing should have thrown the candidates away")
	}
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics.events_raw" {
		t.Errorf("got = %q, want the completion started again from what was typed", got)
	}
}

// The stem is replaced, not the whole line: what follows the cursor stays put.
func TestCompletionKeepsTheRestOfTheLine(t *testing.T) {
	m := editorTUI(t, "SELECT * FROM anal WHERE x = 1")
	for range len(" WHERE x = 1") {
		m = pressKey(t, m, "left")
	}
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics WHERE x = 1" {
		t.Errorf("got = %q", got)
	}
}

func TestCommonPrefixFold(t *testing.T) {
	tests := []struct {
		candidates []string
		want       string
	}{
		{[]string{"events", "events_raw"}, "events"},
		{[]string{"Events", "events_raw"}, "Events"},
		{[]string{"events", "sessions"}, ""},
		{[]string{"only"}, "only"},
		{nil, ""},
	}
	for _, tt := range tests {
		if got := commonPrefixFold(tt.candidates); got != tt.want {
			t.Errorf("%v: got = %q, want %q", tt.candidates, got, tt.want)
		}
	}
}
