package athq

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
)

// testWorkGroup is what the TUI header shows in the tests; the environment of
// whoever runs them must not decide it.
const testWorkGroup = "primary"

func newTestTUI(t *testing.T, width, height int) tuiModel {
	t.Helper()
	t.Setenv(envDatabase, "")
	t.Setenv(envWorkGroup, testWorkGroup)
	t.Setenv(envOutputLocation, "")
	opts.database = ""
	opts.workGroup = ""
	opts.outputLocation = ""

	m := newTUIModel(context.Background(), nil, "", 100)
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(tuiModel)
}

// loadedTUI is a model with two databases, the first one expanded with two
// tables.
func loadedTUI(t *testing.T) tuiModel {
	t.Helper()
	m := newTestTUI(t, 100, 40)

	next, _ := m.Update(msgTUIDatabases{databases: []string{"analytics", "logs"}})
	m = next.(tuiModel)

	next, _ = m.Update(msgTUITables{
		database: "analytics",
		tables: []tuiTable{
			{name: "events", columns: []tuiColumn{
				{name: "event_id", typ: "varchar"},
				{name: "dt", typ: "varchar", partition: true},
			}},
			{name: "sessions"},
		},
	})
	m = next.(tuiModel)

	// The tables arrive only after the database has been expanded.
	m.databases[0].expanded = true
	m.databases[0].loaded = true
	m.rows = catalogRows(m.databases)
	return m
}

func pressKey(t *testing.T, m tuiModel, s string) tuiModel {
	t.Helper()
	var msg tea.KeyPressMsg
	switch s {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		r := []rune(s)[0]
		msg = tea.KeyPressMsg{Code: r, Text: s}
	}
	next, _ := m.Update(msg)
	return next.(tuiModel)
}

func TestCatalogRowsShowsTablesOfExpandedDatabasesOnly(t *testing.T) {
	databases := []tuiDatabase{
		{name: "a", expanded: true, tables: []tuiTable{{name: "t1"}, {name: "t2"}}},
		{name: "b", tables: []tuiTable{{name: "t3"}}},
	}
	rows := catalogRows(databases)
	if len(rows) != 4 {
		t.Fatalf("row count: got = %d, want 4", len(rows))
	}
	if !rows[0].isDatabase() || rows[1].isDatabase() || rows[2].isDatabase() || !rows[3].isDatabase() {
		t.Errorf("got = %v, want a database row, two table rows and a database row", rows)
	}
}

func TestScrollOffsetKeepsTheCursorVisible(t *testing.T) {
	if got := scrollOffset(0, 5, 10); got != 0 {
		t.Errorf("cursor above the window: got = %d, want 0", got)
	}
	if got := scrollOffset(12, 0, 10); got != 3 {
		t.Errorf("cursor below the window: got = %d, want 3", got)
	}
	if got := scrollOffset(4, 2, 10); got != 2 {
		t.Errorf("cursor inside the window: got = %d, want 2", got)
	}
}

func TestViewFitsTheTerminal(t *testing.T) {
	m := loadedTUI(t)
	content := m.View().Content

	lines := strings.Split(content, "\n")
	if len(lines) > 40 {
		t.Errorf("line count: got = %d, want at most 40", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 100 {
			t.Errorf("line %d is %d cells wide, want at most 100", i, w)
		}
	}
}

func TestViewShowsTheCatalogAndTheColumnsOfTheSelectedTable(t *testing.T) {
	m := loadedTUI(t)
	m.catCursor = 1 // the events table
	m.clampCatalogCursor()

	content := stripANSI(m.View().Content)
	for _, want := range []string{"analytics", "events", "sessions", "logs", "event_id", "varchar", "columns: analytics.events"} {
		if !strings.Contains(content, want) {
			t.Errorf("the view does not contain %q", want)
		}
	}
}

func TestViewFitsTheSmallestSupportedTerminal(t *testing.T) {
	m := newTestTUI(t, minTUIWidth, minTUIHeight)
	next, _ := m.Update(msgTUIDatabases{databases: []string{"analytics"}})
	m = next.(tuiModel)

	lines := strings.Split(m.View().Content, "\n")
	if len(lines) != minTUIHeight {
		t.Errorf("line count: got = %d, want %d", len(lines), minTUIHeight)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > minTUIWidth {
			t.Errorf("line %d is %d cells wide, want at most %d", i, w, minTUIWidth)
		}
	}
}

func TestViewComplainsWhenTheTerminalIsTooSmall(t *testing.T) {
	m := newTestTUI(t, 30, 10)
	if !strings.Contains(m.View().Content, "too small") {
		t.Errorf("got = %q, want a complaint about the size", m.View().Content)
	}
}

func TestInsertPutsTheQualifiedTableNameIntoTheEditor(t *testing.T) {
	m := loadedTUI(t)
	m.catCursor = 1 // the events table

	next, _ := m.insertCurrentName()
	m = next.(tuiModel)

	if got := m.editor.Value(); got != "analytics.events" {
		t.Errorf("got = %q, want %q", got, "analytics.events")
	}
}

func TestInsertOnADatabaseRowPutsJustTheDatabaseName(t *testing.T) {
	m := loadedTUI(t)
	m.catCursor = 0

	next, _ := m.insertCurrentName()
	if got := next.(tuiModel).editor.Value(); got != "analytics" {
		t.Errorf("got = %q, want %q", got, "analytics")
	}
}

func TestInsertFromTheColumnsPane(t *testing.T) {
	m := loadedTUI(t)
	m.catCursor = 1
	m.clampCatalogCursor()
	m.focus = paneColumns
	m.colCursor = 1 // the dt partition key

	m = pressKey(t, m, "i")
	if got := m.editor.Value(); got != "dt" {
		t.Errorf("got = %q, want %q", got, "dt")
	}
}

func TestTabCyclesThroughThePanes(t *testing.T) {
	m := loadedTUI(t)
	if m.focus != paneCatalog {
		t.Fatalf("initial focus: got = %v, want the catalog", m.focus)
	}
	m = pressKey(t, m, "tab")
	if m.focus != paneColumns {
		t.Errorf("after one tab: got = %v, want the columns", m.focus)
	}
	m = pressKey(t, m, "tab")
	if m.focus != paneEditor {
		t.Errorf("after two tabs: got = %v, want the editor", m.focus)
	}
	m = pressKey(t, m, "tab")
	if m.focus != paneResult {
		t.Errorf("after three tabs: got = %v, want the result", m.focus)
	}
	m = pressKey(t, m, "tab")
	if m.focus != paneCatalog {
		t.Errorf("after four tabs: got = %v, want the catalog again", m.focus)
	}
}

func TestKeysGoToTheEditorWhenItIsFocused(t *testing.T) {
	m := loadedTUI(t)
	m.focus = paneEditor
	m.editor.Focus()

	for _, s := range []string{"q", "i"} {
		m = pressKey(t, m, s)
	}
	if got := m.editor.Value(); got != "qi" {
		t.Errorf("got = %q, want the characters typed into the editor", got)
	}
}

func TestEnterExpandsADatabase(t *testing.T) {
	m := newTestTUI(t, 100, 40)
	next, _ := m.Update(msgTUIDatabases{databases: []string{"analytics"}})
	m = next.(tuiModel)

	if m.databases[0].expanded {
		t.Fatal("the database starts expanded, want it collapsed")
	}
	m = pressKey(t, m, "enter")
	if !m.databases[0].expanded {
		t.Error("got expanded = false, want the database expanded")
	}
	if !m.databases[0].loading {
		t.Error("got loading = false, want the tables being fetched")
	}
}

func TestRunRefusesAnEmptyQuery(t *testing.T) {
	m := loadedTUI(t)
	next, cmd := m.startQuery()
	m = next.(tuiModel)

	if cmd != nil {
		t.Error("got a command, want none for an empty query")
	}
	if !m.statusErr || !strings.Contains(m.status, "empty") {
		t.Errorf("status: got = %q (err=%v), want a complaint about the empty query", m.status, m.statusErr)
	}
}

func TestSaveNeedsAResult(t *testing.T) {
	m := loadedTUI(t)
	next, _ := m.startSave()
	m = next.(tuiModel)

	if m.mode != modeNormal {
		t.Error("got the save prompt, want it refused without a result")
	}
	if !m.statusErr {
		t.Errorf("status: got = %q, want an error", m.status)
	}
}

func TestQueryDoneFillsTheResultPane(t *testing.T) {
	m := loadedTUI(t)
	m.running = true

	rt := &resultTable{
		columns: []column{{name: "n", typ: "bigint"}},
		rows:    [][]*string{{aws.String("42")}},
	}
	qe := &types.QueryExecution{
		QueryExecutionId: aws.String("abc"),
		Status:           &types.QueryExecutionStatus{State: types.QueryExecutionStateSucceeded},
		Statistics: &types.QueryExecutionStatistics{
			DataScannedInBytes:         aws.Int64(1024 * 1024),
			TotalExecutionTimeInMillis: aws.Int64(1200),
		},
	}

	next, _ := m.Update(msgTUIQueryDone{qe: qe, result: rt})
	m = next.(tuiModel)

	if m.running {
		t.Error("got running = true, want the query finished")
	}
	if m.focus != paneResult {
		t.Errorf("focus: got = %v, want the result pane", m.focus)
	}
	if !strings.Contains(m.status, "1 rows") || !strings.Contains(m.status, "1.00 MiB") {
		t.Errorf("status: got = %q, want the row count and the scanned bytes", m.status)
	}
	if content := stripANSI(m.View().Content); !strings.Contains(content, "42") {
		t.Error("the view does not show the result row")
	}
}

func TestQueryFailureIsShownInTheStatus(t *testing.T) {
	m := loadedTUI(t)
	m.running = true

	next, _ := m.Update(msgTUIQueryDone{err: errTest})
	m = next.(tuiModel)

	if m.running {
		t.Error("got running = true, want the query finished")
	}
	if !m.statusErr || !strings.Contains(m.status, errTest.Error()) {
		t.Errorf("status: got = %q, want the error", m.status)
	}
}

func TestQueryFailureShowsTheWholeMessageInTheResultPane(t *testing.T) {
	m := loadedTUI(t)
	m.running = true

	long := errors.New("COLUMN_NOT_FOUND: line 1:8: Column 'nosuchcolumn' cannot be " +
		"resolved or requester is not authorized to access requested resources")
	qe := &types.QueryExecution{QueryExecutionId: aws.String("abc-123")}

	next, _ := m.Update(msgTUIQueryDone{qe: qe, err: long})
	m = next.(tuiModel)

	if m.focus != paneResult {
		t.Errorf("focus: got = %v, want the result pane so the text can be scrolled", m.focus)
	}
	content := stripANSI(m.View().Content)
	// The status line can only hold the beginning, so every word has to be
	// readable in the pane instead.
	for _, word := range strings.Fields(long.Error()) {
		if !strings.Contains(content, word) {
			t.Errorf("the view is missing %q of the error message", word)
		}
	}
	if !strings.Contains(content, "abc-123") {
		t.Error("the view does not show the execution id")
	}
	if !strings.Contains(content, "error") {
		t.Error("the result pane is not titled as an error")
	}
}

func TestARunClearsThePreviousError(t *testing.T) {
	m := loadedTUI(t)
	m.errText = "an old failure"
	m.editor.SetValue("SELECT 1")

	next, _ := m.startQuery()
	if got := next.(tuiModel).errText; got != "" {
		t.Errorf("got = %q, want the error cleared when a new query starts", got)
	}
}

func TestErrorDetailWithoutAnExecution(t *testing.T) {
	if got := errorDetail(errTest, nil); got != errTest.Error() {
		t.Errorf("got = %q, want just the message", got)
	}
}

func TestWrapTextBreaksWordsLongerThanTheLine(t *testing.T) {
	got := wrapText("short "+strings.Repeat("x", 25)+" tail", 10)
	for _, line := range strings.Split(got, "\n") {
		if w := tuiWidth.StringWidth(line); w > 10 {
			t.Errorf("line %q is %d wide, want at most 10", line, w)
		}
	}
	if strings.Count(strings.ReplaceAll(got, "\n", ""), "x") != 25 {
		t.Errorf("got = %q, want every character kept", got)
	}
}

func TestWrapTextKeepsParagraphs(t *testing.T) {
	got := wrapText("one\n\ntwo", 20)
	if got != "one\n\ntwo" {
		t.Errorf("got = %q, want the blank line kept", got)
	}
}

func TestTruncatePadFitsExactly(t *testing.T) {
	if got := truncatePad("ab", 5); got != "ab   " {
		t.Errorf("padding: got = %q, want %q", got, "ab   ")
	}
	if got := tuiWidth.StringWidth(truncatePad("abcdefgh", 4)); got != 4 {
		t.Errorf("truncation: got width %d, want 4", got)
	}
	if got := truncatePad("あいう", 4); tuiWidth.StringWidth(got) != 4 {
		t.Errorf("wide characters: got %q (%d cells), want 4", got, tuiWidth.StringWidth(got))
	}
}

// stripANSI removes the escape sequences lipgloss adds so tests can look at the
// text alone.
func stripANSI(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String()
}

var errTest = errors.New("SYNTAX_ERROR: line 1:8 mismatched input")
