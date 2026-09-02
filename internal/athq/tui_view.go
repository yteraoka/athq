package athq

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The screen is laid out as a header with the work group and where its results
// go, two panes below it (the catalog tree and the columns of the selected
// table), the editor, the result, and finally a status and a help line.
func (m *tuiModel) layout() {
	if m.width < minTUIWidth || m.height < minTUIHeight {
		return
	}

	usable := m.height - 3 // header + status + help

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
	// "save the query as: " plus the input's own "> " and the padding of the
	// status line have to fit next to them.
	m.nameIn.SetWidth(m.width - 24)
	m.descIn.SetWidth(m.width - 24)

	m.catOffset = scrollOffset(m.catCursor, m.catOffset, m.catalogVisibleRows())
	m.colOffset = scrollOffset(m.colCursor, m.colOffset, m.columnsVisibleRows())
}

// Each pane spends two lines on its border and one on its title.
func (m tuiModel) catalogVisibleRows() int { return max(1, m.topHeight-3) }
func (m tuiModel) columnsVisibleRows() int { return max(1, m.topHeight-3) }

func (m tuiModel) View() tea.View {
	var v tea.View
	v.AltScreen = true
	// Clicking a table or a column puts its name into the query, which saves
	// walking there with tab and i. f2 hands the mouse back to the terminal
	// so its own selection works; see [tuiModel.toggleMouse].
	v.MouseMode = tea.MouseModeCellMotion
	if m.mouseOff {
		v.MouseMode = tea.MouseModeNone
	}

	if !m.ready {
		v.SetContent("\n  starting…\n")
		return v
	}
	if m.width < minTUIWidth || m.height < minTUIHeight {
		v.SetContent(fmt.Sprintf("\n  the terminal is too small (want at least %dx%d)\n", minTUIWidth, minTUIHeight))
		return v
	}

	// Picking a saved query takes the whole screen: the list and the entry
	// under it need more room than the status line has.
	if m.mode == modeOpenQuery {
		v.SetContent(m.savedPickerView())
		return v
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.pane(m.focus == paneCatalog, "databases / tables", m.catalogContent(), m.catalogWidth, m.topHeight),
		m.pane(m.focus == paneColumns, m.columnsTitle(), m.columnsContent(), m.columnsWidth, m.topHeight),
	)

	v.SetContent(lipgloss.JoinVertical(lipgloss.Left,
		m.headerLine(),
		top,
		m.pane(m.focus == paneEditor, m.editorTitle(), m.editor.View(), m.width, m.editHeight),
		m.pane(m.focus == paneResult, m.resultTitle(), m.resultVP.View(), m.width, m.resHeight),
		m.statusLine(),
		m.helpLine(),
	))
	return v
}

// editorTitle names the editor pane, with the vim mode it is in when it has
// the focus: which keys do what depends on it, so it has to be on screen.
func (m tuiModel) editorTitle() string {
	if !m.vim.on || m.focus != paneEditor {
		return "query"
	}
	return "query — " + m.vim.mode.label()
}

// pane draws one bordered box with a title line inside it.
func (m tuiModel) pane(focused bool, title, content string, width, height int) string {
	frame, titleStyle := styleTUIPane, styleTUITitle
	if focused {
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
			text += "  (partition"
			if c.projection != "" {
				text += ": " + c.projection
			}
			text += ")"
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

// headerLine names the work group the queries run in and the S3 location their
// results are written to, which is otherwise only visible in `athq wg desc`.
func (m tuiModel) headerLine() string {
	text := "work group: " + m.workGroupName + "   output: " + m.outputText()
	return styleTUIHeader.Render(truncatePad(text, m.width-2))
}

func (m tuiModel) outputText() string {
	switch {
	case m.wgLoading:
		return "…"
	case m.wgFailed && m.output.location == "":
		return "unknown (the work group could not be read)"
	case m.output.location == "":
		return "not set (the work group has none)"
	case m.output.source != "":
		return m.output.location + " (" + m.output.source + ")"
	default:
		return m.output.location
	}
}

func (m tuiModel) statusLine() string {
	width := m.width
	switch {
	case m.mode == modeSaveResult:
		return styleTUIStatus.Render(padANSI("save to: "+m.saveIn.View(), width-2))
	case m.mode == modeQueryName:
		return styleTUIStatus.Render(padANSI("save the query as: "+m.nameIn.View(), width-2))
	case m.mode == modeQueryDesc:
		return styleTUIStatus.Render(padANSI("description: "+m.descIn.View(), width-2))
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
	case m.mode == modeSaveResult:
		hints = "enter save · esc cancel"
	case m.mode == modeQueryName:
		hints = "enter next (description) · esc cancel"
	case m.mode == modeQueryDesc:
		hints = "enter save the query · esc cancel"
	case m.focus == paneCatalog:
		hints = "tab pane · enter expand · i/click insert · r reload · ^r run · ^s save · ^w/^o saved query · q quit"
	case m.focus == paneColumns:
		hints = "tab pane · i/click insert · ←back · ^r run · ^s save · ^w/^o saved query · q quit"
	case m.focus == paneEditor && m.vim.on && m.vim.mode.visual():
		hints = "y copy · d delete · c change · V lines · esc normal · ^r run · ^c cancel/quit"
	case m.focus == paneEditor && m.vim.on && m.vim.mode == vimNormal:
		hints = "i insert · v visual · yy/dd/p copy/cut/paste · u undo · tab pane · esc leave editor · ^r run · ^s save · ^w/^o saved query"
	case m.focus == paneEditor:
		// q is a valid character to type here, so it cannot quit like it does
		// elsewhere; ^c is the only way out without leaving the editor first.
		hints = "tab complete · esc " + m.escapeHint() + " · ^y copy · ^v paste · ^r run · ^s save · ^w/^o saved query · ^c cancel/quit"
	default:
		hints = "↑↓←→ scroll · tab pane · ^s save · ^w/^o saved query · q quit"
	}
	return styleTUIHelp.Render(truncatePad(hints, m.width-2))
}

// escapeHint says what esc does in the editor, which depends on whether the
// modal layer is there to catch it first.
func (m tuiModel) escapeHint() string {
	if m.vim.on {
		return "normal mode"
	}
	return "leave editor"
}
