package athq

import (
	"strings"
	"unicode"
)

// The query editor is modal, the way vi is: keys are commands until i (or one
// of its relatives) starts inserting, and esc goes back to commanding. It is
// what makes copying and pasting work the same everywhere — y and p never
// have to travel through the terminal as a key combination it might keep for
// itself — and it is turned off with --vim=false or ATHQ_VIM=0, which leaves
// the plain text area of before.
//
// The buffer itself still belongs to bubbles' textarea; this layer reads its
// value, works on the lines as runes and writes the result back with
// [tuiModel.vimSetBuffer]. Only insert mode passes keys to the text area,
// which is why every emacs style binding it has (ctrl+a, ctrl+k, alt+f …)
// still works while typing.

type vimMode int

const (
	vimInsert vimMode = iota
	vimNormal
	vimVisual
	vimVisualLine
)

func (v vimMode) label() string {
	switch v {
	case vimNormal:
		return "NORMAL"
	case vimVisual:
		return "VISUAL"
	case vimVisualLine:
		return "V-LINE"
	default:
		return "INSERT"
	}
}

// visual reports whether the mode has a selection hanging off it.
func (v vimMode) visual() bool { return v == vimVisual || v == vimVisualLine }

// vimPos is a place in the buffer: a line and a rune within it.
type vimPos struct {
	row, col int
}

func (p vimPos) before(q vimPos) bool {
	if p.row != q.row {
		return p.row < q.row
	}
	return p.col < q.col
}

// vimRegister is what the last yank, delete or change put aside for p. A
// linewise register is put back as whole lines, the way vim does it.
type vimRegister struct {
	text     string
	linewise bool
}

// vimUndoStep is the whole buffer before one change. Queries are small, so
// keeping copies is cheaper than tracking what each command touched.
type vimUndoStep struct {
	text string
	cur  vimPos
}

// vimUndoLimit bounds how far back u can go.
const vimUndoLimit = 200

// vimState is the editor's modal state. cur is where the cursor is in every
// mode but insert, where the text area moves it and [tuiModel.vimLeaveInsert]
// reads it back.
type vimState struct {
	on     bool
	mode   vimMode
	cur    vimPos
	anchor vimPos // where visual mode started
	count  int    // the numeric prefix being typed, 0 when there is none
	op     string // the operator waiting for a motion: d, c or y
	prefix string // a key that takes another one after it: g or r
	reg    vimRegister
	undo   []vimUndoStep
}

// clearPending throws away a half typed command, which is what esc and any
// unknown key do.
func (v *vimState) clearPending() {
	v.count, v.op, v.prefix = 0, "", ""
}

// vimMotionKind says how a range up to a motion's target is measured: vim
// calls them exclusive (the target is left out), inclusive (it is taken
// along) and linewise (whole lines).
type vimMotionKind int

const (
	motionNone vimMotionKind = iota
	motionExclusive
	motionInclusive
	motionLinewise
)

func vimLines(s string) [][]rune {
	parts := strings.Split(s, "\n")
	lines := make([][]rune, len(parts))
	for i, p := range parts {
		lines[i] = []rune(p)
	}
	return lines
}

func vimFirstNonBlank(line []rune) int {
	for i, r := range line {
		if !unicode.IsSpace(r) {
			return i
		}
	}
	return 0
}

// vimClamp keeps a position inside the buffer. In normal mode the cursor sits
// on a character, so it stops one short of the end of the line; while
// inserting it may stand past the last one.
func vimClamp(lines [][]rune, p vimPos, allowEnd bool) vimPos {
	if len(lines) == 0 {
		return vimPos{}
	}
	p.row = clampIndex(p.row, len(lines))
	limit := len(lines[p.row])
	if !allowEnd {
		limit = max(0, limit-1)
	}
	p.col = min(max(p.col, 0), limit)
	return p
}

// --- word motions -----------------------------------------------------------

// vimClass groups runes the way vim does: whitespace, word characters and
// punctuation. W, B and E ask for big words, where everything but whitespace
// counts as one class.
func vimClass(r rune, big bool) int {
	switch {
	case r == '\n' || unicode.IsSpace(r):
		return 0
	case big:
		return 1
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	default:
		return 2
	}
}

// vimFlatten lays the buffer out as one rune slice with newlines between the
// lines, which is what lets the word motions cross them without special
// cases. starts holds the offset each line begins at.
func vimFlatten(lines [][]rune) (buf []rune, starts []int) {
	starts = make([]int, len(lines))
	for i, line := range lines {
		starts[i] = len(buf)
		buf = append(buf, line...)
		if i < len(lines)-1 {
			buf = append(buf, '\n')
		}
	}
	return buf, starts
}

func vimOffset(starts []int, p vimPos) int {
	if len(starts) == 0 {
		return 0
	}
	row := clampIndex(p.row, len(starts))
	return starts[row] + max(0, p.col)
}

func vimPosAt(lines [][]rune, starts []int, off int) vimPos {
	if len(lines) == 0 {
		return vimPos{}
	}
	row := len(starts) - 1
	for i := 1; i < len(starts); i++ {
		if off < starts[i] {
			row = i - 1
			break
		}
	}
	return vimPos{row: row, col: min(max(0, off-starts[row]), len(lines[row]))}
}

func vimWordForward(buf []rune, off int, big bool) int {
	i := off
	if i < len(buf) {
		if c := vimClass(buf[i], big); c != 0 {
			for i < len(buf) && vimClass(buf[i], big) == c {
				i++
			}
		}
	}
	for i < len(buf) && vimClass(buf[i], big) == 0 {
		i++
	}
	return min(i, max(0, len(buf)))
}

func vimWordBackward(buf []rune, off int, big bool) int {
	i := off - 1
	for i >= 0 && vimClass(buf[i], big) == 0 {
		i--
	}
	if i < 0 {
		return 0
	}
	c := vimClass(buf[i], big)
	for i > 0 && vimClass(buf[i-1], big) == c {
		i--
	}
	return i
}

func vimWordEnd(buf []rune, off int, big bool) int {
	i := off + 1
	for i < len(buf) && vimClass(buf[i], big) == 0 {
		i++
	}
	if i >= len(buf) {
		return max(0, len(buf)-1)
	}
	c := vimClass(buf[i], big)
	for i+1 < len(buf) && vimClass(buf[i+1], big) == c {
		i++
	}
	return i
}

// vimParagraph is { and }: the next or previous empty line, which in a query
// is the gap between statements.
func vimParagraph(lines [][]rune, row, step, count int) int {
	for range max(count, 1) {
		row += step
		for row > 0 && row < len(lines)-1 && len(lines[row]) != 0 {
			row += step
		}
	}
	return clampIndex(row, len(lines))
}

// --- motions ----------------------------------------------------------------

// vimMotion resolves a motion key against the buffer. page is how many lines
// the editor shows, for the scrolling motions.
func vimMotion(lines [][]rune, cur vimPos, k string, count, page int) (vimPos, vimMotionKind) {
	if len(lines) == 0 {
		return cur, motionNone
	}
	n := max(count, 1)
	last := len(lines) - 1
	cur = vimClamp(lines, cur, true)
	half := max(1, page/2)

	switch k {
	case "h", "left":
		return vimPos{cur.row, max(0, cur.col-n)}, motionExclusive
	case "l", "right", "space":
		return vimPos{cur.row, min(len(lines[cur.row]), cur.col+n)}, motionExclusive
	case "j", "down":
		return vimPos{min(last, cur.row+n), cur.col}, motionLinewise
	case "k", "up":
		return vimPos{max(0, cur.row-n), cur.col}, motionLinewise
	case "0", "home":
		return vimPos{cur.row, 0}, motionExclusive
	case "^":
		return vimPos{cur.row, vimFirstNonBlank(lines[cur.row])}, motionExclusive
	case "$", "end":
		row := min(last, cur.row+n-1)
		return vimPos{row, max(0, len(lines[row])-1)}, motionInclusive
	case "gg":
		row := 0
		if count > 0 {
			row = clampIndex(count-1, len(lines))
		}
		return vimPos{row, vimFirstNonBlank(lines[row])}, motionLinewise
	case "G":
		row := last
		if count > 0 {
			row = clampIndex(count-1, len(lines))
		}
		return vimPos{row, vimFirstNonBlank(lines[row])}, motionLinewise
	case "ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b", "pgdown", "pgup":
		step := map[string]int{
			"ctrl+d": half, "ctrl+u": -half,
			"ctrl+f": max(1, page), "ctrl+b": -max(1, page),
			"pgdown": max(1, page), "pgup": -max(1, page),
		}[k]
		return vimPos{clampIndex(cur.row+step*n, len(lines)), cur.col}, motionLinewise
	case "{", "}":
		step := 1
		if k == "{" {
			step = -1
		}
		return vimPos{vimParagraph(lines, cur.row, step, n), 0}, motionExclusive
	case "w", "W", "b", "B", "e", "E":
		buf, starts := vimFlatten(lines)
		off := vimOffset(starts, cur)
		big := k == "W" || k == "B" || k == "E"
		kind := motionExclusive
		for range n {
			switch k {
			case "w", "W":
				off = vimWordForward(buf, off, big)
			case "b", "B":
				off = vimWordBackward(buf, off, big)
			default:
				off = vimWordEnd(buf, off, big)
				kind = motionInclusive
			}
		}
		return vimPosAt(lines, starts, off), kind
	}
	return cur, motionNone
}

// --- ranges -----------------------------------------------------------------

// vimSlice returns the text between two positions, the end being left out.
func vimSlice(lines [][]rune, from, to vimPos) string {
	from, to = vimClamp(lines, from, true), vimClamp(lines, to, true)
	if to.before(from) {
		from, to = to, from
	}
	if from.row == to.row {
		return string(lines[from.row][from.col:to.col])
	}
	var b strings.Builder
	b.WriteString(string(lines[from.row][from.col:]))
	for row := from.row + 1; row < to.row; row++ {
		b.WriteByte('\n')
		b.WriteString(string(lines[row]))
	}
	b.WriteByte('\n')
	b.WriteString(string(lines[to.row][:to.col]))
	return b.String()
}

// vimCut removes the text between two positions, the end being left out, and
// returns the buffer that is left.
func vimCut(lines [][]rune, from, to vimPos) [][]rune {
	from, to = vimClamp(lines, from, true), vimClamp(lines, to, true)
	if to.before(from) {
		from, to = to, from
	}
	head := append([]rune{}, lines[from.row][:from.col]...)
	tail := append([]rune{}, lines[to.row][to.col:]...)
	out := make([][]rune, 0, len(lines))
	out = append(out, lines[:from.row]...)
	out = append(out, append(head, tail...))
	out = append(out, lines[to.row+1:]...)
	return out
}

// vimLineRange returns the whole lines between two rows as a linewise
// register value, i.e. with a newline after the last one.
func vimLineText(lines [][]rune, first, last int) string {
	var b strings.Builder
	for row := first; row <= last; row++ {
		b.WriteString(string(lines[row]))
		b.WriteByte('\n')
	}
	return b.String()
}

func vimCutLines(lines [][]rune, first, last int) [][]rune {
	out := make([][]rune, 0, len(lines))
	out = append(out, lines[:first]...)
	out = append(out, lines[last+1:]...)
	if len(out) == 0 {
		out = [][]rune{{}}
	}
	return out
}
