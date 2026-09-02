package athq

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// vimRepositionMsg is a message the text area does not act on. Sending it
// through Update is how the view is scrolled back to the cursor after this
// layer has moved it: the text area only repositions itself at the end of
// Update, and that step is not exported on its own.
type vimRepositionMsg struct{}

// --- the buffer and the cursor ----------------------------------------------

// vimBuffer is what the editor holds right now, as lines of runes.
func (m tuiModel) vimBuffer() [][]rune { return vimLines(m.editor.Value()) }

// vimSetBuffer replaces the editor's text and puts the cursor on a given
// line and column.
//
// The text area can set the column but not the line, so the value goes in as
// two halves — everything after the cursor first, then everything before it —
// which leaves the cursor exactly between them.
func (m *tuiModel) vimSetBuffer(lines [][]rune, cur vimPos) {
	if len(lines) == 0 {
		lines = [][]rune{{}}
	}
	cur = vimClamp(lines, cur, m.vim.mode == vimInsert)
	buf, starts := vimFlatten(lines)
	off := vimOffset(starts, cur)

	m.editor.Reset()
	if tail := string(buf[off:]); tail != "" {
		m.editor.InsertString(tail)
	}
	m.editor.MoveToBegin()
	if head := string(buf[:off]); head != "" {
		m.editor.InsertString(head)
	}
	m.vim.cur = cur
	m.vimAfterMove(lines)
}

// vimMoveTo only moves the cursor. Staying on the same line is the common
// case and needs no rebuilding.
func (m *tuiModel) vimMoveTo(lines [][]rune, cur vimPos) {
	cur = vimClamp(lines, cur, m.vim.mode == vimInsert)
	if cur.row == m.editor.Line() {
		m.editor.SetCursorColumn(cur.col)
		m.vim.cur = cur
		m.vimAfterMove(lines)
		return
	}
	m.vimSetBuffer(lines, cur)
}

// vimCursorStyle makes the cursor stand still while commanding and blink
// while typing, which is the other half of saying what mode the editor is in
// — the pane title being the first.
func (m *tuiModel) vimCursorStyle() {
	if !m.vim.on {
		return
	}
	styles := m.editor.Styles()
	blink := m.vim.mode == vimInsert
	if styles.Cursor.Blink == blink {
		return
	}
	styles.Cursor.Blink = blink
	m.editor.SetStyles(styles)
}

// vimAfterMove redraws what depends on where the cursor now is: the selection
// in visual mode, and the scroll position in every mode.
func (m *tuiModel) vimAfterMove(lines [][]rune) {
	m.vimCursorStyle()
	if m.vim.mode.visual() {
		m.vimSelect(lines)
	} else {
		m.editor.ClearSelection()
	}
	m.editor, _ = m.editor.Update(vimRepositionMsg{})
}

// vimSelect highlights what visual mode covers. Vim's selection takes the
// character the cursor is on with it, and the text area's does not, so the
// end goes one further.
func (m *tuiModel) vimSelect(lines [][]rune) {
	if len(lines) == 0 {
		return
	}
	from, to := m.vim.anchor, m.vim.cur
	if to.before(from) {
		from, to = to, from
	}
	from, to = vimClamp(lines, from, true), vimClamp(lines, to, true)
	if m.vim.mode == vimVisualLine {
		from.col = 0
		to.col = len(lines[to.row])
	} else {
		to.col = min(to.col+1, len(lines[to.row]))
	}

	bound := vimDisplayLines(lines, m.editor.Width())
	x1, y1 := vimEditorCoords(m.editor, textarea.Position{Row: from.row, Col: from.col}, bound)
	x2, y2 := vimEditorCoords(m.editor, textarea.Position{Row: to.row, Col: to.col}, bound)
	m.editor.BeginSelection(x1, y1)
	m.editor.ExtendSelection(x2, y2)
	m.editor.EndSelection()
}

// vimDisplayLines is an upper bound on the rows the buffer takes on screen,
// counting the soft wrapped ones.
func vimDisplayLines(lines [][]rune, width int) int {
	if width <= 0 {
		width = 1
	}
	total := 0
	for _, line := range lines {
		total += 1 + len(line)/width
	}
	return total + 1
}

// vimEditorCoords finds the coordinates inside the text area that map back to
// pos. Nothing exported turns a position into coordinates, and the selection
// can only be set through them, so the display line is found by walking down
// from the top of the buffer and the column by halving the line.
func vimEditorCoords(ta textarea.Model, pos textarea.Position, bound int) (int, int) {
	after := func(p textarea.Position) bool {
		return pos.Row < p.Row || (pos.Row == p.Row && pos.Col < p.Col)
	}
	off := ta.ScrollYOffset()
	line := 0
	for d := 0; d <= bound; d++ {
		if after(ta.PositionAt(0, d-off)) {
			break
		}
		line = d
	}

	y := line - off
	lo, hi := 0, ta.Width()+32
	for lo < hi {
		mid := (lo + hi) / 2
		if after(ta.PositionAt(mid, y)) {
			hi = mid
		} else if ta.PositionAt(mid, y) == pos {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo, y
}

// --- modes ------------------------------------------------------------------

// vimEnterInsert starts inserting at a given place, which is what i, a, o and
// the change commands all end with.
func (m *tuiModel) vimEnterInsert(lines [][]rune, cur vimPos) {
	m.vim.mode = vimInsert
	m.vim.clearPending()
	m.editor.ClearSelection()
	m.vimSetBuffer(lines, cur)
}

// vimLeaveInsert is esc while typing: the cursor steps back onto the last
// character typed, the way vi leaves insert mode.
func (m *tuiModel) vimLeaveInsert() {
	m.vim.mode = vimNormal
	m.vim.clearPending()
	m.editor.ClearSelection()
	lines := m.vimBuffer()
	m.vimMoveTo(lines, vimPos{row: m.editor.Line(), col: m.editor.Column() - 1})
}

// vimSyncFromEditor picks the cursor up from the text area, for the places
// that move it without going through this layer: a mouse click, and inserting
// a name from the catalog.
func (m *tuiModel) vimSyncFromEditor() {
	if !m.vim.on {
		return
	}
	m.vim.cur = vimClamp(m.vimBuffer(), vimPos{row: m.editor.Line(), col: m.editor.Column()}, m.vim.mode == vimInsert)
	m.vimCursorStyle()
}

// vimPushUndo remembers the buffer as it is before a command changes it.
func (m *tuiModel) vimPushUndo() {
	m.vim.undo = append(m.vim.undo, vimUndoStep{text: m.editor.Value(), cur: m.vim.cur})
	if len(m.vim.undo) > vimUndoLimit {
		m.vim.undo = m.vim.undo[len(m.vim.undo)-vimUndoLimit:]
	}
}

func (m tuiModel) vimUndo() (tuiModel, tea.Cmd) {
	if len(m.vim.undo) == 0 {
		m.status = "already at the oldest change"
		m.statusErr = false
		return m, nil
	}
	step := m.vim.undo[len(m.vim.undo)-1]
	m.vim.undo = m.vim.undo[:len(m.vim.undo)-1]
	m.vim.mode = vimNormal
	m.vimSetBuffer(vimLines(step.text), step.cur)
	return m, nil
}

// --- the key dispatcher -----------------------------------------------------

// vimKey handles one key in normal or visual mode. Insert mode never reaches
// here: its keys go straight to the text area.
func (m tuiModel) vimKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	lines := m.vimBuffer()
	m.vim.cur = vimClamp(lines, m.vim.cur, false)

	// A key the one before it takes literally.
	switch m.vim.prefix {
	case "r":
		m.vim.prefix = ""
		r, ok := singleRune(msg)
		if !ok {
			return m, nil
		}
		return m.vimReplaceChar(lines, r), nil
	case "g":
		m.vim.prefix = ""
		if k != "g" {
			m.vim.clearPending()
			return m, nil
		}
		k = "gg"
	}

	// Digits build up a count; 0 is a motion unless one is already going.
	if len(k) == 1 && k[0] >= '0' && k[0] <= '9' && (k != "0" || m.vim.count > 0) {
		m.vim.count = min(m.vim.count*10+int(k[0]-'0'), 99999)
		return m, nil
	}
	switch k {
	case "g":
		m.vim.prefix = "g"
		return m, nil
	case "r":
		if m.vim.op == "" && !m.vim.mode.visual() {
			m.vim.prefix = "r"
			return m, nil
		}
	}

	count := m.vim.count

	// An operator typed twice over takes whole lines: dd, yy, cc.
	if m.vim.op != "" && k == m.vim.op {
		op := m.vim.op
		m.vim.clearPending()
		last := clampIndex(m.vim.cur.row+max(count, 1)-1, len(lines))
		return m.vimOperate(lines, op, vimPos{row: m.vim.cur.row}, vimPos{row: last}, motionLinewise)
	}

	if target, kind := vimMotion(lines, m.vim.cur, k, count, m.editor.Height()); kind != motionNone {
		op := m.vim.op
		m.vim.clearPending()
		if op != "" {
			return m.vimOperate(lines, op, m.vim.cur, target, kind)
		}
		m.vimMoveTo(lines, target)
		return m, nil
	}

	// In visual mode an operator acts at once, on the selection.
	if m.vim.mode.visual() {
		switch k {
		case "d", "x", "y", "c", "s":
			op := k
			switch k {
			case "x":
				op = "d"
			case "s":
				op = "c"
			}
			kind := motionInclusive
			if m.vim.mode == vimVisualLine {
				kind = motionLinewise
			}
			anchor := m.vim.anchor
			m.vim.mode = vimNormal
			m.vim.clearPending()
			return m.vimOperate(lines, op, anchor, m.vim.cur, kind)
		}
	}

	switch k {
	case "esc":
		switch {
		case m.vim.mode.visual():
			m.vim.mode = vimNormal
			m.vim.clearPending()
			m.vimMoveTo(lines, m.vim.cur)
		case m.vim.count > 0 || m.vim.op != "" || m.vim.prefix != "":
			m.vim.clearPending()
		default:
			// Nothing half typed: esc leaves the pane, as it does elsewhere.
			return m.setFocus(paneCatalog)
		}
		return m, nil

	case "d", "c", "y":
		m.vim.op = k
		return m, nil

	case "i":
		m.vimEnterInsert(lines, m.vim.cur)
	case "I":
		m.vimEnterInsert(lines, vimPos{m.vim.cur.row, vimFirstNonBlank(lines[m.vim.cur.row])})
	case "a":
		m.vimEnterInsert(lines, vimPos{m.vim.cur.row, m.vim.cur.col + 1})
	case "A":
		m.vimEnterInsert(lines, vimPos{m.vim.cur.row, len(lines[m.vim.cur.row])})
	case "o", "O":
		row := m.vim.cur.row
		if k == "o" {
			row++
		}
		indent := vimIndent(lines[m.vim.cur.row])
		m.vimPushUndo()
		next := make([][]rune, 0, len(lines)+1)
		next = append(next, lines[:row]...)
		next = append(next, append([]rune{}, indent...))
		next = append(next, lines[row:]...)
		m.vimEnterInsert(next, vimPos{row, len(indent)})

	case "v", "V":
		mode := vimVisual
		if k == "V" {
			mode = vimVisualLine
		}
		if m.vim.mode == mode {
			mode = vimNormal
		} else if !m.vim.mode.visual() {
			m.vim.anchor = m.vim.cur
		}
		m.vim.mode = mode
		m.vim.clearPending()
		m.vimMoveTo(lines, m.vim.cur)

	case "x", "X":
		from, to := m.vim.cur, vimPos{m.vim.cur.row, m.vim.cur.col + max(count, 1)}
		if k == "X" {
			from, to = vimPos{m.vim.cur.row, max(0, m.vim.cur.col-max(count, 1))}, m.vim.cur
		}
		m.vim.clearPending()
		if from == to {
			return m, nil
		}
		return m.vimOperate(lines, "d", from, to, motionExclusive)

	case "D", "C":
		op := "d"
		if k == "C" {
			op = "c"
		}
		m.vim.clearPending()
		return m.vimOperate(lines, op, m.vim.cur, vimPos{m.vim.cur.row, len(lines[m.vim.cur.row])}, motionExclusive)

	case "Y":
		m.vim.clearPending()
		last := clampIndex(m.vim.cur.row+max(count, 1)-1, len(lines))
		return m.vimOperate(lines, "y", vimPos{row: m.vim.cur.row}, vimPos{row: last}, motionLinewise)

	case "s":
		m.vim.clearPending()
		return m.vimOperate(lines, "c", m.vim.cur, vimPos{m.vim.cur.row, m.vim.cur.col + max(count, 1)}, motionExclusive)

	case "S":
		m.vim.clearPending()
		last := clampIndex(m.vim.cur.row+max(count, 1)-1, len(lines))
		return m.vimOperate(lines, "c", vimPos{row: m.vim.cur.row}, vimPos{row: last}, motionLinewise)

	case "J":
		return m.vimJoin(lines, max(count, 1)), nil

	case "p", "P":
		m.vim.clearPending()
		if m.vim.reg.text == "" {
			return m, nil
		}
		return m.vimPut(m.vim.reg.text, m.vim.reg.linewise, k == "p", max(count, 1)), nil

	case "u":
		m.vim.clearPending()
		return m.vimUndo()

	default:
		m.vim.clearPending()
	}
	return m, nil
}

// singleRune is the character a key stands for, for the commands that take
// one literally.
func singleRune(msg tea.KeyPressMsg) (rune, bool) {
	r := []rune(msg.Text)
	if len(r) != 1 {
		return 0, false
	}
	return r[0], true
}

func vimIndent(line []rune) []rune {
	n := 0
	for n < len(line) && unicode.IsSpace(line[n]) {
		n++
	}
	return line[:n]
}

// --- the commands that change the buffer ------------------------------------

// vimOperate applies d, c or y over a range and leaves the editor where vim
// would leave it. Yanking also fills the system clipboard, so that what was
// yanked can be pasted into another window.
func (m tuiModel) vimOperate(lines [][]rune, op string, from, to vimPos, kind vimMotionKind) (tuiModel, tea.Cmd) {
	if to.before(from) {
		from, to = to, from
	}
	from, to = vimClamp(lines, from, true), vimClamp(lines, to, true)

	var (
		text     string
		linewise bool
		next     [][]rune
		cur      vimPos
	)
	if kind == motionLinewise {
		linewise = true
		text = vimLineText(lines, from.row, to.row)
		switch op {
		case "y":
			cur = vimPos{from.row, vimFirstNonBlank(lines[from.row])}
		case "c":
			// The lines go, but one empty one stays to type on, with the
			// indentation of the first, which is what a query wants.
			indent := append([]rune{}, vimIndent(lines[from.row])...)
			next = make([][]rune, 0, len(lines))
			next = append(next, lines[:from.row]...)
			next = append(next, indent)
			next = append(next, lines[to.row+1:]...)
			cur = vimPos{from.row, len(indent)}
		default:
			next = vimCutLines(lines, from.row, to.row)
			row := clampIndex(from.row, len(next))
			cur = vimPos{row, vimFirstNonBlank(next[row])}
		}
	} else {
		if kind == motionInclusive {
			to.col = min(to.col+1, len(lines[to.row]))
		}
		text = vimSlice(lines, from, to)
		if op != "y" {
			next = vimCut(lines, from, to)
		}
		cur = from
	}

	m.vim.reg = vimRegister{text: text, linewise: linewise}
	if next != nil {
		m.vimPushUndo()
	}
	if m.vim.mode.visual() {
		m.vim.mode = vimNormal
	}

	switch {
	case op == "c":
		m.vim.mode = vimInsert
		m.vimSetBuffer(next, cur)
	case next != nil:
		m.vimSetBuffer(next, cur)
	default:
		m.vimMoveTo(lines, cur)
	}

	if op == "y" {
		return m, copySelectionCmd(text)
	}
	return m, nil
}

// vimReplaceChar is r: the character under the cursor becomes another one.
func (m tuiModel) vimReplaceChar(lines [][]rune, r rune) tuiModel {
	cur := m.vim.cur
	if cur.col >= len(lines[cur.row]) {
		return m
	}
	m.vimPushUndo()
	next := vimCopyLines(lines)
	next[cur.row][cur.col] = r
	m.vim.clearPending()
	m.vimSetBuffer(next, cur)
	return m
}

// vimJoin is J: the next line comes up onto this one, with a single space
// where the break was.
func (m tuiModel) vimJoin(lines [][]rune, count int) tuiModel {
	m.vim.clearPending()
	row := m.vim.cur.row
	if row+1 >= len(lines) {
		return m
	}
	m.vimPushUndo()
	next := vimCopyLines(lines)
	col := 0
	for range max(count, 1) {
		if row+1 >= len(next) {
			break
		}
		joined := next[row]
		tail := next[row+1][vimFirstNonBlank(next[row+1]):]
		if len(joined) > 0 && len(tail) > 0 {
			col = len(joined)
			joined = append(joined, ' ')
		}
		next[row] = append(append([]rune{}, joined...), tail...)
		next = append(next[:row+1], next[row+2:]...)
	}
	m.vimSetBuffer(next, vimPos{row, col})
	return m
}

// vimPut is p and P, and what a paste does outside insert mode. Lines yanked
// whole go back as lines, above or below the cursor; anything else goes in
// beside it.
func (m tuiModel) vimPut(text string, linewise, after bool, count int) tuiModel {
	if text == "" {
		return m
	}
	lines := m.vimBuffer()
	m.vimPushUndo()
	m.vim.mode = vimNormal

	if linewise {
		put := vimLines(strings.TrimSuffix(text, "\n"))
		row := m.vim.cur.row
		if after {
			row++
		}
		row = min(row, len(lines))
		next := make([][]rune, 0, len(lines)+len(put)*count)
		next = append(next, lines[:row]...)
		for range count {
			next = append(next, vimCopyLines(put)...)
		}
		next = append(next, lines[row:]...)
		m.vimSetBuffer(next, vimPos{row, vimFirstNonBlank(next[row])})
		return m
	}

	col := m.vim.cur.col
	if after && len(lines[m.vim.cur.row]) > 0 {
		col++
	}
	buf, starts := vimFlatten(lines)
	off := vimOffset(starts, vimPos{m.vim.cur.row, col})
	ins := []rune(strings.Repeat(text, count))
	merged := make([]rune, 0, len(buf)+len(ins))
	merged = append(merged, buf[:off]...)
	merged = append(merged, ins...)
	merged = append(merged, buf[off:]...)

	next := vimLines(string(merged))
	_, nstarts := vimFlatten(next)
	m.vimSetBuffer(next, vimPosAt(next, nstarts, off+len(ins)-1))
	return m
}

func vimCopyLines(lines [][]rune) [][]rune {
	out := make([][]rune, len(lines))
	for i, line := range lines {
		out[i] = append([]rune{}, line...)
	}
	return out
}

// vimAfterDrag turns a selection made with the mouse into a visual mode one,
// so that the keys that act on a selection — y, d, c — carry on from where
// the pointer left off.
func (m *tuiModel) vimAfterDrag() {
	if !m.vim.on || m.vim.mode == vimInsert {
		m.vimSyncFromEditor()
		return
	}
	start, end, ok := m.editor.Selection()
	if !ok {
		m.vim.mode = vimNormal
		m.vimSyncFromEditor()
		return
	}
	m.vim.mode = vimVisual
	m.vim.anchor = vimPos{row: start.Row, col: start.Col}
	m.vim.cur = vimPos{row: end.Row, col: max(0, end.Col-1)}
	m.vimSelect(m.vimBuffer())
}
