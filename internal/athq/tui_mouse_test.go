package athq

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The rows of a pane start two lines into it: the border, then the title. In a
// 100x40 test model the top panes begin on line 1, so their first row is on
// line 3, and the columns pane starts at m.catalogWidth.
const firstTopPaneRow = 3

func click(t *testing.T, m tuiModel, x, y int) tuiModel {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(tuiModel)
}

func wheel(t *testing.T, m tuiModel, x, y int, button tea.MouseButton) tuiModel {
	t.Helper()
	next, _ := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: button})
	return next.(tuiModel)
}

func TestContentRowSkipsTheBorderAndTheTitle(t *testing.T) {
	if got := contentRow(1, 1, 5); got != -1 {
		t.Errorf("the border: got = %d, want -1", got)
	}
	if got := contentRow(2, 1, 5); got != -1 {
		t.Errorf("the title: got = %d, want -1", got)
	}
	if got := contentRow(3, 1, 5); got != 0 {
		t.Errorf("the first row: got = %d, want 0", got)
	}
	if got := contentRow(7, 1, 5); got != 4 {
		t.Errorf("the last row: got = %d, want 4", got)
	}
	if got := contentRow(8, 1, 5); got != -1 {
		t.Errorf("below the rows: got = %d, want -1", got)
	}
}

func TestTheViewAsksForMouseEvents(t *testing.T) {
	m := loadedTUI(t)
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("got = %v, want the mouse enabled", got)
	}
}

func TestClickingATableSelectsItAndClickingItAgainInsertsIt(t *testing.T) {
	m := loadedTUI(t)

	// row 0 is the analytics database, row 1 its events table.
	m = click(t, m, 2, firstTopPaneRow+1)
	if m.focus != paneCatalog || m.catCursor != 1 {
		t.Fatalf("focus = %v, cursor = %d, want the catalog on the events table", m.focus, m.catCursor)
	}
	if got := m.editor.Value(); got != "" {
		t.Errorf("editor: got = %q, want the first click to only select", got)
	}

	m = click(t, m, 2, firstTopPaneRow+1)
	if got := m.editor.Value(); got != "analytics.events" {
		t.Errorf("editor: got = %q, want the table inserted", got)
	}
}

func TestClickingADatabaseOpensAndClosesIt(t *testing.T) {
	m := loadedTUI(t)
	if !m.databases[0].expanded {
		t.Fatal("the database starts collapsed, want it expanded")
	}

	m = click(t, m, 2, firstTopPaneRow)
	if m.databases[0].expanded {
		t.Error("got expanded = true, want the click to close the database")
	}
	m = click(t, m, 2, firstTopPaneRow)
	if !m.databases[0].expanded {
		t.Error("got expanded = false, want the click to open it again")
	}
}

func TestClickingADatabaseFetchesItsTables(t *testing.T) {
	m := newTestTUI(t, 100, 40)
	next, _ := m.Update(msgTUIDatabases{databases: []string{"analytics"}})
	m = next.(tuiModel)

	m = click(t, m, 2, firstTopPaneRow)
	if !m.databases[0].expanded || !m.databases[0].loading {
		t.Errorf("got expanded = %v, loading = %v, want both", m.databases[0].expanded, m.databases[0].loading)
	}
}

func TestClickingAColumnInsertsIt(t *testing.T) {
	m := loadedTUI(t)
	m.catCursor = 1 // the events table
	m.clampCatalogCursor()

	m = click(t, m, m.catalogWidth+2, firstTopPaneRow+1)
	if got := m.editor.Value(); got != "dt" {
		t.Errorf("editor: got = %q, want the clicked column inserted", got)
	}
	if m.focus != paneColumns || m.colCursor != 1 {
		t.Errorf("focus = %v, cursor = %d, want the columns pane on the clicked row", m.focus, m.colCursor)
	}
}

func TestClickingBelowTheRowsOnlyMovesTheFocus(t *testing.T) {
	m := loadedTUI(t)

	// The catalog has four rows; this lands on the empty space below them.
	m = click(t, m, 2, firstTopPaneRow+6)
	if m.focus != paneCatalog {
		t.Errorf("focus: got = %v, want the catalog", m.focus)
	}
	if m.catCursor != 0 || m.editor.Value() != "" {
		t.Errorf("cursor = %d, editor = %q, want neither touched", m.catCursor, m.editor.Value())
	}
}

func TestClickingTheTitleOfAPaneOnlyMovesTheFocus(t *testing.T) {
	m := loadedTUI(t)
	m.catCursor = 1
	m.clampCatalogCursor()

	m = click(t, m, m.catalogWidth+2, 2) // the title line of the columns pane
	if m.focus != paneColumns {
		t.Errorf("focus: got = %v, want the columns pane", m.focus)
	}
	if got := m.editor.Value(); got != "" {
		t.Errorf("editor: got = %q, want nothing inserted", got)
	}
}

func TestClickingTheEditorAndTheResultMovesTheFocus(t *testing.T) {
	m := loadedTUI(t)

	m = click(t, m, 10, 1+m.topHeight+1)
	if m.focus != paneEditor {
		t.Errorf("focus: got = %v, want the editor", m.focus)
	}
	m = click(t, m, 10, 1+m.topHeight+m.editHeight+1)
	if m.focus != paneResult {
		t.Errorf("focus: got = %v, want the result", m.focus)
	}
}

func TestClicksAreIgnoredWhileAPromptIsOpen(t *testing.T) {
	m := loadedTUI(t)
	m.editor.SetValue("SELECT 1")
	m = pressCtrl(t, m, 'w') // the name prompt

	m = click(t, m, 2, firstTopPaneRow+1)
	if m.mode != modeQueryName {
		t.Errorf("mode: got = %v, want the prompt still open", m.mode)
	}
	if got := m.editor.Value(); got != "SELECT 1" {
		t.Errorf("editor: got = %q, want it untouched", got)
	}
}

func TestTheWheelMovesTheCatalogCursor(t *testing.T) {
	m := loadedTUI(t)

	m = wheel(t, m, 2, firstTopPaneRow, tea.MouseWheelDown)
	if m.catCursor != mouseWheelRows {
		t.Errorf("cursor: got = %d, want %d", m.catCursor, mouseWheelRows)
	}
	m = wheel(t, m, 2, firstTopPaneRow, tea.MouseWheelUp)
	if m.catCursor != 0 {
		t.Errorf("cursor: got = %d, want it back at the top", m.catCursor)
	}
	// The catalog has four rows, so a second notch cannot go further.
	m = wheel(t, m, 2, firstTopPaneRow, tea.MouseWheelDown)
	m = wheel(t, m, 2, firstTopPaneRow, tea.MouseWheelDown)
	if want := len(m.rows) - 1; m.catCursor != want {
		t.Errorf("cursor: got = %d, want %d", m.catCursor, want)
	}
}

func TestTheWheelScrollsTheResultPane(t *testing.T) {
	m := loadedTUI(t)
	m.resultVP.SetContent(longResultContent())

	m = wheel(t, m, 10, 1+m.topHeight+m.editHeight+1, tea.MouseWheelDown)
	if m.resultVP.YOffset() == 0 {
		t.Error("got the result pane still at the top, want it scrolled")
	}
}

func TestClickingTheSavedQueryPickerSelectsThenOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.jsonl")
	t.Setenv(envSavedQueriesFile, path)
	m := loadedTUI(t)
	seedSavedQueries(t, path)
	m = m.showSavedQueries(mustReadSavedQueries(t, path))

	// The list starts on the third line of the screen: border, title, rows.
	m = click(t, m, 4, 3)
	if m.savedCursor != 1 {
		t.Fatalf("cursor: got = %d, want the clicked entry", m.savedCursor)
	}
	if m.mode != modeOpenQuery {
		t.Fatalf("mode: got = %v, want the picker still open", m.mode)
	}

	m = click(t, m, 4, 3)
	if m.mode != modeNormal {
		t.Errorf("mode: got = %v, want the picker closed", m.mode)
	}
	if got := m.editor.Value(); got != "SELECT * FROM users" {
		t.Errorf("editor: got = %q, want the clicked query", got)
	}
}

func TestTheWheelMovesTheSavedQueryPicker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.jsonl")
	t.Setenv(envSavedQueriesFile, path)
	m := loadedTUI(t)
	seedSavedQueries(t, path)
	m = m.showSavedQueries(mustReadSavedQueries(t, path))

	m = wheel(t, m, 4, 3, tea.MouseWheelDown)
	if want := len(m.savedQueries) - 1; m.savedCursor != want {
		t.Errorf("cursor: got = %d, want %d", m.savedCursor, want)
	}
}

func longResultContent() string {
	var sb []byte
	for i := 0; i < 100; i++ {
		sb = append(sb, []byte("a row of the result\n")...)
	}
	return string(sb)
}
