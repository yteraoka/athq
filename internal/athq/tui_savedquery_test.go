package athq

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func pressCtrl(t *testing.T, m tuiModel, r rune) tuiModel {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl})
	return next.(tuiModel)
}

func typeText(t *testing.T, m tuiModel, s string) tuiModel {
	t.Helper()
	for _, r := range s {
		m = pressKey(t, m, string(r))
	}
	return m
}

// savedQueriesTUI is a model whose store is a file of its own.
func savedQueriesTUI(t *testing.T) (tuiModel, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "queries.jsonl")
	t.Setenv(envSavedQueriesFile, path)
	return loadedTUI(t), path
}

func TestSaveQueryAsksForANameAndADescription(t *testing.T) {
	m, path := savedQueriesTUI(t)
	m.editor.SetValue("SELECT 1")

	m = pressCtrl(t, m, 'w')
	if m.mode != modeQueryName {
		t.Fatalf("mode: got = %v, want the name prompt", m.mode)
	}
	m = typeText(t, m, "users")
	m = pressKey(t, m, "enter")
	if m.mode != modeQueryDesc {
		t.Fatalf("mode: got = %v, want the description prompt", m.mode)
	}
	m = typeText(t, m, "everyone")

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(tuiModel)
	if m.mode != modeNormal {
		t.Errorf("mode: got = %v, want the prompt closed", m.mode)
	}
	if cmd == nil {
		t.Fatal("got no command, want the query being written")
	}

	msg, ok := cmd().(msgTUIQuerySaved)
	if !ok {
		t.Fatalf("got %T, want msgTUIQuerySaved", msg)
	}
	if msg.err != nil {
		t.Fatalf("got error %v, want none", msg.err)
	}

	queries, err := readSavedQueries(path)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(queries) != 1 {
		t.Fatalf("count: got = %d, want 1", len(queries))
	}
	if queries[0].Name != "users" || queries[0].Description != "everyone" || queries[0].Query != "SELECT 1" {
		t.Errorf("got = %+v, want the name, the description and the query", queries[0])
	}
}

func TestSaveQueryPromptShowsWhatItAsksFor(t *testing.T) {
	m, _ := savedQueriesTUI(t)
	m.editor.SetValue("SELECT 1")

	m = pressCtrl(t, m, 'w')
	if content := stripANSI(m.View().Content); !strings.Contains(content, "save the query as:") {
		t.Error("the name prompt is not on the status line")
	}
	m = typeText(t, m, "users")
	m = pressKey(t, m, "enter")
	if content := stripANSI(m.View().Content); !strings.Contains(content, "description:") {
		t.Error("the description prompt is not on the status line")
	}
}

func TestSaveQueryPromptStaysInsideTheTerminal(t *testing.T) {
	m, _ := savedQueriesTUI(t)
	m.editor.SetValue("SELECT 1")

	m = pressCtrl(t, m, 'w')
	m = typeText(t, m, strings.Repeat("n", 200))
	assertViewFits(t, m, 100)

	m = pressKey(t, m, "enter")
	m = typeText(t, m, strings.Repeat("d", 200))
	assertViewFits(t, m, 100)
}

func assertViewFits(t *testing.T, m tuiModel, width int) {
	t.Helper()
	for i, line := range strings.Split(m.View().Content, "\n") {
		if w := tuiWidth.StringWidth(stripANSI(line)); w > width {
			t.Errorf("line %d is %d cells wide, want at most %d", i, w, width)
		}
	}
}

func TestSaveQueryNeedsAQuery(t *testing.T) {
	m, _ := savedQueriesTUI(t)

	m = pressCtrl(t, m, 'w')
	if m.mode != modeNormal {
		t.Errorf("mode: got = %v, want the prompt refused", m.mode)
	}
	if !m.statusErr {
		t.Errorf("status: got = %q, want an error", m.status)
	}
}

func TestSaveQueryNeedsANameToGoOn(t *testing.T) {
	m, _ := savedQueriesTUI(t)
	m.editor.SetValue("SELECT 1")

	m = pressCtrl(t, m, 'w')
	m = pressKey(t, m, "enter")
	if m.mode != modeQueryName {
		t.Errorf("mode: got = %v, want the name still being asked for", m.mode)
	}
}

func TestSaveQueryIsCancelledByEscape(t *testing.T) {
	m, path := savedQueriesTUI(t)
	m.editor.SetValue("SELECT 1")

	m = pressCtrl(t, m, 'w')
	m = typeText(t, m, "users")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(tuiModel)

	if m.mode != modeNormal {
		t.Errorf("mode: got = %v, want the prompt closed", m.mode)
	}
	queries, err := readSavedQueries(path)
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if len(queries) != 0 {
		t.Errorf("got = %v, want nothing stored", queries)
	}
}

func TestSaveQuerySuggestsTheNameOfTheOpenedOne(t *testing.T) {
	m, _ := savedQueriesTUI(t)
	m.editor.SetValue("SELECT 1")
	m.savedName, m.savedDesc = "users", "everyone"

	m = pressCtrl(t, m, 'w')
	if got := m.nameIn.Value(); got != "users" {
		t.Errorf("name: got = %q, want the previous one offered again", got)
	}
	m = pressKey(t, m, "enter")
	if got := m.descIn.Value(); got != "everyone" {
		t.Errorf("description: got = %q, want the previous one offered again", got)
	}
}

func TestOpenQueryPutsTheSelectedQueryIntoTheEditor(t *testing.T) {
	m, path := savedQueriesTUI(t)
	seedSavedQueries(t, path)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = next.(tuiModel)
	if cmd == nil {
		t.Fatal("got no command, want the store being read")
	}
	next, _ = m.Update(cmd())
	m = next.(tuiModel)

	if m.mode != modeOpenQuery {
		t.Fatalf("mode: got = %v, want the picker", m.mode)
	}
	if len(m.savedQueries) != 2 {
		t.Fatalf("count: got = %d, want 2", len(m.savedQueries))
	}

	// The listing is by name, so the cursor starts on "events".
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")

	if m.mode != modeNormal {
		t.Errorf("mode: got = %v, want the picker closed", m.mode)
	}
	if got := m.editor.Value(); got != "SELECT * FROM users" {
		t.Errorf("editor: got = %q, want the selected query", got)
	}
	if m.focus != paneEditor {
		t.Errorf("focus: got = %v, want the editor", m.focus)
	}
	if m.savedName != "users" {
		t.Errorf("savedName: got = %q, want the opened name remembered", m.savedName)
	}
}

func TestOpenQueryShowsTheNameDescriptionTimeAndQuery(t *testing.T) {
	m, path := savedQueriesTUI(t)
	seedSavedQueries(t, path)

	m = m.showSavedQueries(mustReadSavedQueries(t, path))
	content := stripANSI(m.View().Content)
	for _, want := range []string{"users", "everyone", "events", "2024-03-04 05:06", "SELECT * FROM events"} {
		if !strings.Contains(content, want) {
			t.Errorf("the picker does not show %q", want)
		}
	}
}

func TestOpenQueryPickerFitsTheTerminal(t *testing.T) {
	m, path := savedQueriesTUI(t)
	seedSavedQueries(t, path)
	m = m.showSavedQueries(mustReadSavedQueries(t, path))

	if lines := strings.Split(m.View().Content, "\n"); len(lines) > 40 {
		t.Errorf("line count: got = %d, want at most 40", len(lines))
	}
	assertViewFits(t, m, 100)
}

func TestOpenQueryKeepsTheCursorInTheList(t *testing.T) {
	m, path := savedQueriesTUI(t)
	seedSavedQueries(t, path)
	m = m.showSavedQueries(mustReadSavedQueries(t, path))

	m = pressKey(t, m, "k")
	if m.savedCursor != 0 {
		t.Errorf("above the first entry: got = %d, want 0", m.savedCursor)
	}
	for i := 0; i < 5; i++ {
		m = pressKey(t, m, "j")
	}
	if want := len(m.savedQueries) - 1; m.savedCursor != want {
		t.Errorf("below the last entry: got = %d, want %d", m.savedCursor, want)
	}
}

func TestOpenQueryIsCancelledByEscape(t *testing.T) {
	m, path := savedQueriesTUI(t)
	seedSavedQueries(t, path)
	m = m.showSavedQueries(mustReadSavedQueries(t, path))
	m.editor.SetValue("SELECT 1")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(tuiModel)

	if m.mode != modeNormal {
		t.Errorf("mode: got = %v, want the picker closed", m.mode)
	}
	if got := m.editor.Value(); got != "SELECT 1" {
		t.Errorf("editor: got = %q, want it untouched", got)
	}
}

func TestOpenQuerySaysWhenThereIsNothingStored(t *testing.T) {
	m, _ := savedQueriesTUI(t)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = next.(tuiModel)
	next, _ = m.Update(cmd())
	m = next.(tuiModel)

	if m.mode != modeNormal {
		t.Errorf("mode: got = %v, want no picker", m.mode)
	}
	if !strings.Contains(m.status, "no saved queries") {
		t.Errorf("status: got = %q, want it saying the store is empty", m.status)
	}
}

func seedSavedQueries(t *testing.T, path string) {
	t.Helper()
	at := time.Date(2024, 3, 4, 5, 6, 7, 0, time.Local)
	entries := []savedQuery{
		{Name: "users", Description: "everyone", SavedAt: at, Query: "SELECT * FROM users"},
		{Name: "events", Description: "what happened", SavedAt: at, Query: "SELECT * FROM events"},
	}
	for _, e := range entries {
		if _, err := putSavedQuery(path, e); err != nil {
			t.Fatal(err)
		}
	}
}

func mustReadSavedQueries(t *testing.T, path string) []savedQuery {
	t.Helper()
	queries, err := readSavedQueries(path)
	if err != nil {
		t.Fatal(err)
	}
	return queries
}
