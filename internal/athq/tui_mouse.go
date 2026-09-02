package athq

import (
	tea "charm.land/bubbletea/v2"
)

// mouseWheelRows is how far one notch of the wheel moves a list.
const mouseWheelRows = 3

// paneHit is where a click landed: the pane, and the index of the row of that
// pane under the pointer. row is -1 on the border and the title line, and on
// the panes that have no rows to speak of.
type paneHit struct {
	pane tuiPane
	row  int
}

// editorTop and resultTop are the screen lines the editor and result panes
// start on. The header takes the first line, then the two top panes, the
// editor and the result follow one another.
func (m tuiModel) editorTop() int { return 1 + m.topHeight }
func (m tuiModel) resultTop() int { return m.editorTop() + m.editHeight }

// hitTest turns screen coordinates back into a pane and a row. The panes are
// drawn at places layout() computed, so the same numbers give the answer.
func (m tuiModel) hitTest(y int) (paneHit, bool) {
	if !m.ready || m.width < minTUIWidth || m.height < minTUIHeight {
		return paneHit{}, false
	}
	topTop := 1
	editTop := m.editorTop()
	resTop := m.resultTop()

	switch {
	case y >= topTop && y < editTop:
		return paneHit{row: contentRow(y, topTop, m.catalogVisibleRows())}, true
	case y >= editTop && y < resTop:
		return paneHit{pane: paneEditor, row: -1}, true
	case y >= resTop && y < resTop+m.resHeight:
		return paneHit{pane: paneResult, row: -1}, true
	}
	return paneHit{}, false
}

// editorPos converts screen coordinates into ones relative to the editor's
// own content area, i.e. past its border and title line, for
// [textarea.Model.BeginSelection] and [textarea.Model.ExtendSelection].
func (m tuiModel) editorPos(x, y int) (int, int) {
	return x - 1, y - m.editorTop() - 2
}

// contentRow maps a screen line onto the row of the pane drawn there. A pane
// spends its first line on the border and its second on the title.
func contentRow(y, paneTop, visible int) int {
	row := y - paneTop - 2
	if row < 0 || row >= visible {
		return -1
	}
	return row
}

func (m tuiModel) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if m.mode == modeOpenQuery {
		return m.clickSavedQuery(msg.Y)
	}
	// The prompts take the keyboard, and a stray click should not act on the
	// panes underneath them.
	if m.mode != modeNormal {
		return m, nil
	}

	hit, ok := m.hitTest(msg.Y)
	if !ok {
		return m, nil
	}
	switch hit.pane {
	case paneEditor:
		// A fresh click always starts a new selection at that point, which
		// also places the cursor there; a plain click without a drag leaves
		// no selection, same as textarea's own keyboard selection.
		m.focus = paneEditor
		ex, ey := m.editorPos(msg.X, msg.Y)
		m.editor.BeginSelection(ex, ey)
		m.vimSyncFromEditor()
		return m, m.editor.Focus()
	case paneResult:
		return m.setFocus(hit.pane)
	}
	// The top of the screen is split between the catalog and the columns.
	if msg.X >= m.catalogWidth {
		return m.clickColumn(hit.row)
	}
	return m.clickCatalogRow(hit.row)
}

// clickCatalogRow opens or closes a database, and inserts the table that is
// already selected. A table is selected first so its columns can be read
// before its name goes into the query.
func (m tuiModel) clickCatalogRow(row int) (tea.Model, tea.Cmd) {
	if row < 0 {
		return m.setFocus(paneCatalog)
	}
	i := m.catOffset + row
	if i >= len(m.rows) {
		return m.setFocus(paneCatalog)
	}

	wasSelected := m.focus == paneCatalog && i == m.catCursor
	m.focus = paneCatalog
	m.editor.Blur()
	if i != m.catCursor {
		m.catCursor = i
		m.clampCatalogCursor()
	}

	r, ok := m.currentRow()
	switch {
	case !ok:
		return m, nil
	case r.isDatabase():
		return m.toggleDatabase(r.db)
	case wasSelected:
		return m.insertCurrentName()
	}
	return m, nil
}

// clickColumn inserts the column that was clicked. Unlike a table, selecting a
// column on its own shows nothing more, so one click is enough.
func (m tuiModel) clickColumn(row int) (tea.Model, tea.Cmd) {
	if row < 0 {
		return m.setFocus(paneColumns)
	}
	columns := m.currentColumns()
	i := m.colOffset + row
	if i >= len(columns) {
		return m.setFocus(paneColumns)
	}

	m.focus = paneColumns
	m.editor.Blur()
	m.colCursor = i
	m.colOffset = scrollOffset(m.colCursor, m.colOffset, m.columnsVisibleRows())
	return m.insert(columns[i].name)
}

// clickSavedQuery selects an entry of the picker, and opens the one that is
// already selected, so the query can be read below the list first.
func (m tuiModel) clickSavedQuery(y int) (tea.Model, tea.Cmd) {
	row := contentRow(y, 0, m.savedVisibleRows())
	if row < 0 {
		return m, nil
	}
	i := m.savedOffset + row
	if i >= len(m.savedQueries) {
		return m, nil
	}
	if i == m.savedCursor {
		return m.openSavedQuery()
	}
	m.savedCursor = i
	m.savedOffset = scrollOffset(m.savedCursor, m.savedOffset, m.savedVisibleRows())
	return m, nil
}

// handleMouseMotion extends a drag started in the editor by handleMouseClick.
// It is a no-op when no such drag is in progress, so every motion message can
// be routed here regardless of where the pointer currently is; a drag that
// leaves the editor's rows still resolves to the nearest line, the same way
// PositionAt treats any other out-of-range coordinate.
func (m tuiModel) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	ex, ey := m.editorPos(msg.X, msg.Y)
	m.editor.ExtendSelection(ex, ey)
	return m, nil
}

// handleMouseRelease ends a drag started in the editor. Like ExtendSelection,
// EndSelection is a no-op when there was no drag to finish.
func (m tuiModel) handleMouseRelease(_ tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	m.editor.EndSelection()
	m.vimAfterDrag()
	return m, nil
}

func (m tuiModel) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	var delta int
	switch msg.Button {
	case tea.MouseWheelUp:
		delta = -mouseWheelRows
	case tea.MouseWheelDown:
		delta = mouseWheelRows
	default:
		return m, nil
	}

	if m.mode == modeOpenQuery {
		m.savedCursor = clampIndex(m.savedCursor+delta, len(m.savedQueries))
		m.savedOffset = scrollOffset(m.savedCursor, m.savedOffset, m.savedVisibleRows())
		return m, nil
	}
	if m.mode != modeNormal {
		return m, nil
	}

	hit, ok := m.hitTest(msg.Y)
	if !ok {
		return m, nil
	}
	switch {
	case hit.pane == paneResult:
		// The viewport scrolls itself, focused or not.
		var cmd tea.Cmd
		m.resultVP, cmd = m.resultVP.Update(msg)
		return m, cmd
	case hit.pane == paneEditor:
		return m, nil
	case msg.X >= m.catalogWidth:
		m.colCursor = clampIndex(m.colCursor+delta, len(m.currentColumns()))
		m.colOffset = scrollOffset(m.colCursor, m.colOffset, m.columnsVisibleRows())
	default:
		m.catCursor += delta
		m.clampCatalogCursor()
	}
	return m, nil
}

// clampIndex keeps i inside a list of n entries.
func clampIndex(i, n int) int {
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	return i
}
