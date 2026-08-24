package athq

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type msgTUIQuerySaved struct {
	name     string
	replaced bool
	err      error
}

type msgTUISavedQueries struct {
	queries []savedQuery
	err     error
}

// startSaveQuery opens the two step prompt: the name first, the description
// after it.
func (m tuiModel) startSaveQuery() (tea.Model, tea.Cmd) {
	if isBlankQuery(strings.TrimSpace(m.editor.Value())) {
		return m.fail(fmt.Errorf("there is no query to save")), nil
	}
	m.mode = modeQueryName
	// The name and the description of the query that was last saved or opened
	// are offered again, so editing a stored query keeps its entry.
	m.nameIn.SetValue(m.savedName)
	m.nameIn.CursorEnd()
	m.descIn.SetValue(m.savedDesc)
	m.descIn.CursorEnd()
	return m, m.nameIn.Focus()
}

func (m tuiModel) handleSaveQueryKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.nameIn.Blur()
		m.descIn.Blur()
		return m, nil

	case "enter":
		if m.mode == modeQueryName {
			// The name is what the query is found by later, so an empty one
			// leaves the prompt open instead of storing something unpickable.
			if strings.TrimSpace(m.nameIn.Value()) == "" {
				return m, nil
			}
			m.mode = modeQueryDesc
			m.nameIn.Blur()
			return m, m.descIn.Focus()
		}

		name := strings.TrimSpace(m.nameIn.Value())
		description := strings.TrimSpace(m.descIn.Value())
		m.mode = modeNormal
		m.nameIn.Blur()
		m.descIn.Blur()
		m.savedName, m.savedDesc = name, description
		m.status = "saving the query as " + name + "…"
		m.statusErr = false
		return m, saveQueryCmd(name, description, strings.TrimSpace(m.editor.Value()))
	}

	var cmd tea.Cmd
	if m.mode == modeQueryName {
		m.nameIn, cmd = m.nameIn.Update(msg)
	} else {
		m.descIn, cmd = m.descIn.Update(msg)
	}
	return m, cmd
}

// startOpenQuery reads the stored queries; the picker opens once they arrive.
func (m tuiModel) startOpenQuery() (tea.Model, tea.Cmd) {
	return m, loadSavedQueriesCmd()
}

func (m tuiModel) showSavedQueries(queries []savedQuery) tuiModel {
	if len(queries) == 0 {
		m.status = "no saved queries yet (^w saves the one in the editor)"
		m.statusErr = false
		return m
	}
	m.savedQueries = queries
	m.savedCursor, m.savedOffset = 0, 0
	// Start on the query that was last saved or opened; it is the likely one.
	for i, q := range queries {
		if q.Name == m.savedName {
			m.savedCursor = i
			break
		}
	}
	m.savedOffset = scrollOffset(m.savedCursor, 0, m.savedVisibleRows())
	m.mode = modeOpenQuery
	m.status = fmt.Sprintf("%d saved queries", len(queries))
	m.statusErr = false
	return m
}

func (m tuiModel) handleOpenQueryKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, tuiKeys.Escape), key.Matches(msg, tuiKeys.Cancel), key.Matches(msg, tuiKeys.Quit):
		m.mode = modeNormal
		return m, nil
	case key.Matches(msg, tuiKeys.Toggle):
		return m.openSavedQuery()
	case key.Matches(msg, tuiKeys.Up):
		m.savedCursor--
	case key.Matches(msg, tuiKeys.Down):
		m.savedCursor++
	case key.Matches(msg, tuiKeys.PageUp):
		m.savedCursor -= m.savedVisibleRows()
	case key.Matches(msg, tuiKeys.PageDown):
		m.savedCursor += m.savedVisibleRows()
	}
	if m.savedCursor >= len(m.savedQueries) {
		m.savedCursor = len(m.savedQueries) - 1
	}
	if m.savedCursor < 0 {
		m.savedCursor = 0
	}
	m.savedOffset = scrollOffset(m.savedCursor, m.savedOffset, m.savedVisibleRows())
	return m, nil
}

// openSavedQuery replaces the editor content with the selected query.
func (m tuiModel) openSavedQuery() (tea.Model, tea.Cmd) {
	q, ok := m.currentSavedQuery()
	if !ok {
		m.mode = modeNormal
		return m, nil
	}
	m.mode = modeNormal
	m.editor.SetValue(q.Query)
	m.editor.MoveToEnd()
	m.savedName, m.savedDesc = q.Name, q.Description
	m.status = "opened " + q.Name
	m.statusErr = false
	m.focus = paneEditor
	return m, m.editor.Focus()
}

func (m tuiModel) currentSavedQuery() (savedQuery, bool) {
	if m.savedCursor < 0 || m.savedCursor >= len(m.savedQueries) {
		return savedQuery{}, false
	}
	return m.savedQueries[m.savedCursor], true
}

func saveQueryCmd(name, description, query string) tea.Cmd {
	return func() tea.Msg {
		path, err := savedQueriesPath()
		if err != nil {
			return msgTUIQuerySaved{name: name, err: err}
		}
		replaced, err := putSavedQuery(path, savedQuery{
			Name:        name,
			Description: description,
			SavedAt:     time.Now(),
			Query:       query,
		})
		return msgTUIQuerySaved{name: name, replaced: replaced, err: err}
	}
}

func loadSavedQueriesCmd() tea.Cmd {
	return func() tea.Msg {
		path, err := savedQueriesPath()
		if err != nil {
			return msgTUISavedQueries{err: err}
		}
		queries, err := readSavedQueries(path)
		return msgTUISavedQueries{queries: queries, err: err}
	}
}

// The picker is drawn instead of the panes: a list on top and the whole entry
// below it, so the query is read before it replaces what is in the editor.
func (m tuiModel) savedListHeight() int {
	usable := m.height - 1 // the help line
	h := usable * 45 / 100
	if h < 5 {
		h = 5
	}
	if h > usable-5 {
		h = usable - 5
	}
	return h
}

func (m tuiModel) savedVisibleRows() int { return max(1, m.savedListHeight()-3) }

func (m tuiModel) savedPickerView() string {
	usable := m.height - 1
	listHeight := m.savedListHeight()

	title := "saved queries"
	if q, ok := m.currentSavedQuery(); ok {
		title = "saved query: " + q.Name
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.pane(true, fmt.Sprintf("saved queries (%d)", len(m.savedQueries)), m.savedListContent(), m.width, listHeight),
		m.pane(false, title, m.savedDetailContent(usable-listHeight-3), m.width, usable-listHeight),
		styleTUIHelp.Render(truncatePad("↑↓/click move · enter open · esc cancel", m.width-2)),
	)
}

func (m tuiModel) savedListContent() string {
	width := m.width - 2
	nameWidth := min(30, max(10, width/3))

	visible := m.savedVisibleRows()
	lines := make([]string, 0, visible)
	for i := m.savedOffset; i < len(m.savedQueries) && len(lines) < visible; i++ {
		q := m.savedQueries[i]
		text := truncatePad(q.Name, nameWidth) + "  " +
			q.SavedAt.Local().Format("2006-01-02 15:04") + "  " + q.Description
		line := truncatePad(text, width)
		if i == m.savedCursor {
			line = styleTUIRowSelected.Render(line)
		} else {
			line = styleTUIRow.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// savedDetailContent shows the whole entry in at most rows lines; a query too
// long for the pane is cut with a marker rather than pushing the box open.
func (m tuiModel) savedDetailContent(rows int) string {
	q, ok := m.currentSavedQuery()
	if !ok {
		return styleTUIDim.Render("(no saved query)")
	}
	width := m.width - 2

	description := q.Description
	if description == "" {
		description = "(no description)"
	}
	head := []string{
		styleTUIDim.Render(truncatePad("saved  "+q.SavedAt.Local().Format("2006-01-02 15:04:05"), width)),
		styleTUIDim.Render(truncatePad(description, width)),
		"",
	}

	lines := head
	for _, line := range strings.Split(wrapText(q.Query, width), "\n") {
		if len(lines) >= rows {
			lines[len(lines)-1] = styleTUIDim.Render(truncatePad("… (the query is longer)", width))
			break
		}
		lines = append(lines, truncatePad(line, width))
	}
	if len(lines) > rows {
		lines = lines[:rows]
	}
	return strings.Join(lines, "\n")
}
