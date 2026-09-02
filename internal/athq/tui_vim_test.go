package athq

import (
	"math/rand"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// vimTUI is a loaded model with the query in the editor, in normal mode and
// with the cursor at the top left, which is where vi opens a file.
func vimTUI(t *testing.T, sql string) tuiModel {
	t.Helper()
	m := loadedTUI(t)
	m.focus = paneEditor
	m.editor.Focus()
	m.vim.mode = vimNormal
	m.vimSetBuffer(vimLines(sql), vimPos{})
	if !m.vim.on {
		t.Fatal("setup: vim mode should be on by default")
	}
	return m
}

// vimType presses the keys one after another, the way they would be typed.
func vimType(t *testing.T, m tuiModel, keys string) tuiModel {
	t.Helper()
	for _, r := range keys {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(tuiModel)
	}
	return m
}

func vimCmd(t *testing.T, m tuiModel, keys string) (tuiModel, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, r := range keys {
		next, c := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m, cmd = next.(tuiModel), c
	}
	return m, cmd
}

func wantValue(t *testing.T, m tuiModel, want string) {
	t.Helper()
	if got := m.editor.Value(); got != want {
		t.Errorf("editor: got = %q, want %q", got, want)
	}
}

func wantCursor(t *testing.T, m tuiModel, row, col int) {
	t.Helper()
	if m.vim.cur != (vimPos{row, col}) {
		t.Errorf("cursor: got = %+v, want {%d %d}", m.vim.cur, row, col)
	}
}

func TestVimStartsInNormalModeAndTypingNeedsInsert(t *testing.T) {
	m := vimTUI(t, "")
	m = vimType(t, m, "iSELECT 1")
	if m.vim.mode != vimInsert {
		t.Errorf("mode: got = %v, want insert", m.vim.mode.label())
	}
	wantValue(t, m, "SELECT 1")
}

func TestVimEscLeavesInsertAndStepsBackOntoTheLastCharacter(t *testing.T) {
	m := vimTUI(t, "")
	m = vimType(t, m, "iab")
	m = pressKey(t, m, "esc")
	if m.vim.mode != vimNormal {
		t.Errorf("mode: got = %v, want normal", m.vim.mode.label())
	}
	wantCursor(t, m, 0, 1)
}

// esc in normal mode has nothing left to leave but the pane, which is what it
// does everywhere else in the TUI.
func TestVimEscInNormalModeLeavesThePane(t *testing.T) {
	m := vimTUI(t, "SELECT 1")
	m = pressKey(t, m, "esc")
	if m.focus != paneCatalog {
		t.Errorf("focus: got = %v, want the catalog", m.focus)
	}
}

func TestVimMotions(t *testing.T) {
	const sql = "SELECT a, b\nFROM events\nWHERE x = 1"
	tests := []struct {
		keys     string
		row, col int
	}{
		{"", 0, 0},
		{"l", 0, 1},
		{"3l", 0, 3},
		{"j", 1, 0},
		{"jjk", 1, 0},
		{"$", 0, 10},
		{"j$", 1, 10},
		{"w", 0, 7},
		{"ww", 0, 8}, // the comma is a word of its own, as in vim
		{"b", 0, 0},
		{"$b", 0, 8},
		{"e", 0, 5},
		{"G", 2, 0},
		{"Gk", 1, 0},
		{"Ggg", 0, 0},
		{"2G", 1, 0},
		{"$hh", 0, 8},
		{"j^", 1, 0},
		{"jlll0", 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.keys, func(t *testing.T) {
			m := vimType(t, vimTUI(t, sql), tt.keys)
			wantCursor(t, m, tt.row, tt.col)
			wantValue(t, m, sql)
		})
	}
}

func TestVimEditingCommands(t *testing.T) {
	const sql = "SELECT a\nFROM t\nWHERE x"
	tests := []struct {
		name string
		keys string
		want string
	}{
		{"x deletes the character under the cursor", "x", "ELECT a\nFROM t\nWHERE x"},
		{"a count repeats it", "3x", "ECT a\nFROM t\nWHERE x"},
		{"X deletes the one before", "llX", "SLECT a\nFROM t\nWHERE x"},
		{"dd takes the line", "dd", "FROM t\nWHERE x"},
		{"a count takes several", "2dd", "WHERE x"},
		{"dw takes the word", "dw", "a\nFROM t\nWHERE x"},
		{"D takes the rest of the line", "lllD", "SEL\nFROM t\nWHERE x"},
		{"D on the second line", "jD", "SELECT a\n\nWHERE x"},
		{"cc empties the line and inserts", "ccnew", "new\nFROM t\nWHERE x"},
		{"C changes the rest of the line", "lllCX", "SELX\nFROM t\nWHERE x"},
		{"o opens a line below", "oX", "SELECT a\nX\nFROM t\nWHERE x"},
		{"O opens one above", "OX", "X\nSELECT a\nFROM t\nWHERE x"},
		{"J joins the next line up", "J", "SELECT a FROM t\nWHERE x"},
		{"r replaces one character", "rX", "XELECT a\nFROM t\nWHERE x"},
		{"yy and p put the line back below", "yyp", "SELECT a\nSELECT a\nFROM t\nWHERE x"},
		{"P puts it above", "yyP", "SELECT a\nSELECT a\nFROM t\nWHERE x"},
		{"dd and p move a line down", "ddp", "FROM t\nSELECT a\nWHERE x"},
		{"a yanked word is put beside the cursor", "ywP", "SELECT SELECT a\nFROM t\nWHERE x"},
		{"u takes the last change back", "ddu", sql},
		{"u takes several back", "dddduu", sql},
		{"an unknown key does nothing", "z", sql},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := vimType(t, vimTUI(t, sql), tt.keys)
			wantValue(t, m, tt.want)
		})
	}
}

// cc has to leave exactly one line to type on, whether or not the query had
// any other lines to begin with.
func TestVimChangeLineLeavesOneEmptyLine(t *testing.T) {
	m := vimType(t, vimTUI(t, "SELECT a"), "cc")
	wantValue(t, m, "")
	if m.vim.mode != vimInsert {
		t.Errorf("mode: got = %v, want insert", m.vim.mode.label())
	}
	m = vimType(t, vimTUI(t, "  SELECT a\nFROM t"), "ccX")
	wantValue(t, m, "  X\nFROM t")
	m = vimType(t, vimTUI(t, "one\ntwo\nthree"), "2ccX")
	wantValue(t, m, "X\nthree")
}

func TestVimVisualModeSelectsAndYanks(t *testing.T) {
	m := vimTUI(t, "SELECT a\nFROM t")
	m = vimType(t, m, "v") // start selecting
	if m.vim.mode != vimVisual {
		t.Fatalf("mode: got = %v, want visual", m.vim.mode.label())
	}
	m = vimType(t, m, "e") // to the end of SELECT
	if got := m.editor.SelectedText(); got != "SELECT" {
		t.Errorf("selection: got = %q, want %q", got, "SELECT")
	}

	m, cmd := vimCmd(t, m, "y")
	if cmd == nil {
		t.Fatal("got no command, want the clipboard one")
	}
	if m.vim.mode != vimNormal {
		t.Errorf("mode: got = %v, want normal after yanking", m.vim.mode.label())
	}
	if m.vim.reg.text != "SELECT" || m.vim.reg.linewise {
		t.Errorf("register: got = %+v, want SELECT charwise", m.vim.reg)
	}
}

func TestVimVisualLineTakesWholeLines(t *testing.T) {
	m := vimTUI(t, "one\ntwo\nthree")
	m = vimType(t, m, "Vj")
	if got := m.editor.SelectedText(); got != "one\ntwo" {
		t.Errorf("selection: got = %q, want the two lines", got)
	}
	m = vimType(t, m, "d")
	wantValue(t, m, "three")
	if !m.vim.reg.linewise {
		t.Error("register: got charwise, want linewise")
	}
	m = vimType(t, m, "p")
	wantValue(t, m, "three\none\ntwo")
}

func TestVimVisualEscKeepsTheText(t *testing.T) {
	m := vimTUI(t, "SELECT a")
	m = vimType(t, m, "vll")
	m = pressKey(t, m, "esc")
	if m.vim.mode != vimNormal {
		t.Errorf("mode: got = %v, want normal", m.vim.mode.label())
	}
	if m.editor.HasSelection() {
		t.Error("got a selection, want esc to have dropped it")
	}
	wantValue(t, m, "SELECT a")
}

// Yanking fills the system clipboard as well, so what was yanked can be
// pasted into another window. The command is never run here: it would touch
// the real clipboard.
func TestVimYankAlsoCopiesToTheClipboard(t *testing.T) {
	m := vimTUI(t, "SELECT a\nFROM t")
	_, cmd := vimCmd(t, m, "yy")
	if cmd == nil {
		t.Error("got no command, want the clipboard one")
	}
}

func TestCtrlYCopiesTheSelection(t *testing.T) {
	m := vimTUI(t, "SELECT a")
	m = vimType(t, m, "ve")
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	m = next.(tuiModel)
	if cmd == nil {
		t.Fatal("got no command, want the clipboard one")
	}
	if m.vim.reg.text != "SELECT" {
		t.Errorf("register: got = %q, want it filled like y does", m.vim.reg.text)
	}
}

func TestCtrlYWithoutASelectionSaysSo(t *testing.T) {
	m := insertingEditor(loadedTUI(t))
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	m = next.(tuiModel)
	if cmd != nil {
		t.Error("got a command, want none when nothing is selected")
	}
	if !strings.Contains(m.status, "nothing is selected") {
		t.Errorf("status: got = %q, want it to say nothing is selected", m.status)
	}
}

// A terminal paste arrives whatever mode the editor is in, and in normal mode
// there is no insertion point, so it is put beside the cursor like p.
func TestPasteInNormalModePutsTheTextBesideTheCursor(t *testing.T) {
	m := vimTUI(t, "ab")
	next, _ := m.Update(tea.PasteMsg{Content: "XY"})
	m = next.(tuiModel)
	wantValue(t, m, "aXYb")
}

func TestPasteOfWholeLinesInNormalModeGoesBelow(t *testing.T) {
	m := vimTUI(t, "one\ntwo")
	next, _ := m.Update(tea.PasteMsg{Content: "new\n"})
	m = next.(tuiModel)
	wantValue(t, m, "one\nnew\ntwo")
}

func TestClipboardPasteReportsAFailure(t *testing.T) {
	m := vimTUI(t, "")
	next, _ := m.Update(msgTUIPasted{err: errNoClipboardTool})
	m = next.(tuiModel)
	if !m.statusErr || !strings.Contains(m.status, "clipboard") {
		t.Errorf("status: got = %q (error=%v), want it to explain the failure", m.status, m.statusErr)
	}
}

// Copying goes out as an OSC 52 sequence as well, so a missing helper program
// is worth mentioning but is not an error.
func TestCopyReportsWhatItDid(t *testing.T) {
	m := vimTUI(t, "")
	next, _ := m.Update(msgTUICopied{runes: 6})
	m = next.(tuiModel)
	if m.statusErr || m.status != "copied 6 characters" {
		t.Errorf("status: got = %q (error=%v)", m.status, m.statusErr)
	}

	next, _ = m.Update(msgTUICopied{runes: 1, err: errNoClipboardTool})
	m = next.(tuiModel)
	if m.statusErr || !strings.HasPrefix(m.status, "copied 1 character through the terminal") {
		t.Errorf("status: got = %q (error=%v)", m.status, m.statusErr)
	}
}

// tab is the completion key while typing and the pane key when commanding,
// since nothing is half typed in normal mode.
func TestTabMovesPanesInNormalModeAndCompletesInInsertMode(t *testing.T) {
	m := vimTUI(t, "SELECT * FROM anal")
	m = pressKey(t, m, "tab")
	if m.focus != paneResult {
		t.Errorf("focus: got = %v, want the next pane", m.focus)
	}

	m = vimTUI(t, "SELECT * FROM anal")
	m = vimType(t, m, "A") // insert at the end of the line
	m = pressKey(t, m, "tab")
	if got := m.editor.Value(); got != "SELECT * FROM analytics" {
		t.Errorf("got = %q, want the name completed", got)
	}
}

func TestVimModeIsShownInThePaneTitle(t *testing.T) {
	m := vimTUI(t, "SELECT 1")
	if got := m.editorTitle(); got != "query — NORMAL" {
		t.Errorf("title: got = %q", got)
	}
	m = vimType(t, m, "i")
	if got := m.editorTitle(); got != "query — INSERT" {
		t.Errorf("title: got = %q", got)
	}
}

func TestVimCanBeTurnedOff(t *testing.T) {
	t.Setenv(envVim, "0")
	m := newTestTUI(t, 100, 40)
	t.Setenv(envVim, "0")
	m2 := newTUIModel(m.ctx, nil, "", 100)
	if m2.vim.on {
		t.Error("got vim mode on, want ATHQ_VIM=0 to have turned it off")
	}
	if m2.vim.mode != vimInsert {
		t.Errorf("mode: got = %v, want the plain text area", m2.vim.mode.label())
	}
}

func TestF2HandsTheMouseToTheTerminalAndBack(t *testing.T) {
	m := loadedTUI(t)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	m = next.(tuiModel)
	if !m.mouseOff {
		t.Fatal("got the mouse still taken, want f2 to have released it")
	}
	if m.View().MouseMode != tea.MouseModeNone {
		t.Error("view: got the mouse still on, want it off")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	m = next.(tuiModel)
	if m.mouseOff || m.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("got the mouse still released, want f2 to have taken it back")
	}
}

// The commands index into lines and runes in a good many places, so they are
// also run at random against buffers with empty lines, wide characters and a
// single character of text, which is where the off-by-ones live.
func TestVimSurvivesRandomKeys(t *testing.T) {
	keys := []string{
		"h", "j", "k", "l", "w", "b", "e", "0", "$", "^", "g", "G", "{", "}",
		"i", "a", "A", "I", "o", "O", "v", "V", "r", "s", "S",
		"x", "X", "d", "c", "y", "p", "P", "u", "D", "C", "Y", "J",
		"1", "2", "W", "B", "E", "z", " ",
	}
	texts := []string{"", "a", "SELECT 1", "one\ntwo\nthree", "\n\n", "  indented\n\nx", "日本語\nSELECT *"}
	rng := rand.New(rand.NewSource(7))
	for i := range 300 {
		m := vimTUI(t, texts[i%len(texts)])
		var typed strings.Builder
		for range 12 {
			k := keys[rng.Intn(len(keys))]
			typed.WriteString(k)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%q panicked: %v", typed.String(), r)
					}
				}()
				next, _ := m.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
				m = next.(tuiModel)
				_ = m.View() // the view has to stay renderable too
			}()
		}
	}
}
