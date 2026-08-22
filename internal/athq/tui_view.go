package athq

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The screen is laid out as two panes on top (the catalog tree and the columns
// of the selected table), the editor below them, then the result, and finally
// a status and a help line.
func (m *tuiModel) layout() {
	if m.width < minTUIWidth || m.height < minTUIHeight {
		return
	}

	usable := m.height - 2 // status + help

	m.topHeight = usable * 40 / 100
	if m.topHeight < 6 {
		m.topHeight = 6
	}
	m.editHeight = usable * 30 / 100
	if m.editHeight < 5 {
		m.editHeight = 5
	}
	m.resHeight = usable - m.topHeight - m.editHeight
	if m.resHeight < 4 {
		m.resHeight = 4
		m.topHeight = usable - m.editHeight - m.resHeight
	}

	m.catalogWidth = m.width * 35 / 100
	if m.catalogWidth < 24 {
		m.catalogWidth = 24
	}
	if m.catalogWidth > m.width/2 {
		m.catalogWidth = m.width / 2
	}
	m.columnsWidth = m.width - m.catalogWidth

	m.editor.SetWidth(m.width - 2)
	m.editor.SetHeight(m.editHeight - 3)
	m.resultVP.SetWidth(m.width - 2)
	m.resultVP.SetHeight(m.resHeight - 3)
	m.refreshResultPane()
	m.saveIn.SetWidth(m.width - 12)

	m.catOffset = scrollOffset(m.catCursor, m.catOffset, m.catalogVisibleRows())
	m.colOffset = scrollOffset(m.colCursor, m.colOffset, m.columnsVisibleRows())
}

// Each pane spends two lines on its border and one on its title.
func (m tuiModel) catalogVisibleRows() int { return max(1, m.topHeight-3) }
func (m tuiModel) columnsVisibleRows() int { return max(1, m.topHeight-3) }

func (m tuiModel) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if !m.ready {
		v.SetContent("\n  starting…\n")
		return v
	}
	if m.width < minTUIWidth || m.height < minTUIHeight {
		v.SetContent(fmt.Sprintf("\n  the terminal is too small (want at least %dx%d)\n", minTUIWidth, minTUIHeight))
		return v
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.pane(paneCatalog, "databases / tables", m.catalogContent(), m.catalogWidth, m.topHeight),
		m.pane(paneColumns, m.columnsTitle(), m.columnsContent(), m.columnsWidth, m.topHeight),
	)

	v.SetContent(lipgloss.JoinVertical(lipgloss.Left,
		top,
		m.pane(paneEditor, "query", m.editor.View(), m.width, m.editHeight),
		m.pane(paneResult, m.resultTitle(), m.resultVP.View(), m.width, m.resHeight),
		m.statusLine(),
		m.helpLine(),
	))
	return v
}

// pane draws one bordered box with a title line inside it.
func (m tuiModel) pane(id tuiPane, title, content string, width, height int) string {
	frame, titleStyle := styleTUIPane, styleTUITitle
	if m.focus == id {
		frame, titleStyle = styleTUIPaneFocused, styleTUITitleFocused
	}
	inner := width - 2
	head := titleStyle.Render(truncatePad(title, inner))
	// lipgloss counts the border in Width and Height, so these are the outer
	// dimensions of the box.
	return frame.Width(width).Height(height).Render(head + "\n" + content)
}

func (m tuiModel) catalogContent() string {
	width := m.catalogWidth - 2
	if m.catLoading {
		return padANSI(m.spinner.View()+styleTUIDim.Render(" loading…"), width)
	}
	if len(m.rows) == 0 {
		return styleTUIDim.Render("(no databases)")
	}

	visible := m.catalogVisibleRows()
	lines := make([]string, 0, visible)
	for i := m.catOffset; i < len(m.rows) && len(lines) < visible; i++ {
		row := m.rows[i]
		db := m.databases[row.db]

		var text string
		if row.isDatabase() {
			marker := "▸"
			switch {
			case db.loading:
				marker = "⋯"
			case db.expanded:
				marker = "▾"
			}
			text = marker + " " + db.name
		} else {
			text = "    " + db.tables[row.table].name
		}

		line := truncatePad(text, width)
		if i == m.catCursor {
			line = styleTUIRowSelected.Render(line)
		} else {
			line = styleTUIRow.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) columnsTitle() string {
	if name := m.currentTableName(); name != "" {
		return "columns: " + name
	}
	return "columns"
}

func (m tuiModel) columnsContent() string {
	width := m.columnsWidth - 2
	columns := m.currentColumns()
	if len(columns) == 0 {
		row, ok := m.currentRow()
		switch {
		case !ok:
			return ""
		case row.isDatabase() && !m.databases[row.db].expanded:
			return styleTUIDim.Render("(enter to expand the database)")
		case row.isDatabase():
			return styleTUIDim.Render("(select a table)")
		default:
			return styleTUIDim.Render("(no columns)")
		}
	}

	nameWidth := 0
	for _, c := range columns {
		if n := tuiWidth.StringWidth(c.name); n > nameWidth {
			nameWidth = n
		}
	}
	if nameWidth > width/2 {
		nameWidth = width / 2
	}

	visible := m.columnsVisibleRows()
	lines := make([]string, 0, visible)
	for i := m.colOffset; i < len(columns) && len(lines) < visible; i++ {
		c := columns[i]
		text := truncatePad(c.name, nameWidth) + "  " + c.typ
		if c.partition {
			text += "  (partition)"
		}
		line := truncatePad(text, width)
		switch {
		case i == m.colCursor && m.focus == paneColumns:
			line = styleTUIRowSelected.Render(line)
		case c.partition:
			line = styleTUIPartition.Render(line)
		default:
			line = styleTUIRow.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) resultTitle() string {
	if m.errText != "" {
		return "error"
	}
	if m.result == nil {
		return "result"
	}
	suffix := ""
	if m.result.truncated {
		suffix = fmt.Sprintf(" (first %d)", m.maxRows)
	}
	return fmt.Sprintf("result: %d rows%s", len(m.result.rows), suffix)
}

func (m tuiModel) statusLine() string {
	width := m.width
	switch {
	case m.saving:
		return styleTUIStatus.Render(padANSI("save to: "+m.saveIn.View(), width-2))
	case m.running:
		text := fmt.Sprintf("%s running %s  (^c to cancel)", m.spinner.View(), formatDuration(time.Since(m.runStart)))
		return styleTUIStatus.Render(padANSI(text, width-2))
	case m.statusErr:
		return styleTUIStatusErr.Render(truncatePad(m.status, width-2))
	default:
		return styleTUIStatus.Render(truncatePad(m.status, width-2))
	}
}

func (m tuiModel) helpLine() string {
	var hints string
	switch {
	case m.saving:
		hints = "enter save · esc cancel"
	case m.focus == paneCatalog:
		hints = "tab pane · enter expand · i insert · r reload · ^r run · ^s save · q quit"
	case m.focus == paneColumns:
		hints = "tab pane · i insert · ←back · ^r run · ^s save · q quit"
	case m.focus == paneEditor:
		hints = "esc leave editor · ^r run · ^s save · tab pane"
	default:
		hints = "↑↓←→ scroll · tab pane · ^s save · q quit"
	}
	return styleTUIHelp.Render(truncatePad(hints, m.width-2))
}
