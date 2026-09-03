package athq

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/mattn/go-runewidth"
)

type tuiPane int

const (
	paneCatalog tuiPane = iota
	paneColumns
	paneEditor
	paneResult
	paneCount
)

// tuiMode says which prompt or overlay is on top of the panes. Everything but
// modeNormal takes the keyboard away from them.
type tuiMode int

const (
	modeNormal tuiMode = iota
	modeSaveResult
	modeQueryName
	modeQueryDesc
	modeOpenQuery
)

const (
	minTUIWidth  = 60
	minTUIHeight = 16
)

type msgTUIQueryDone struct {
	qe     *types.QueryExecution
	result *resultTable
	err    error
}

type msgTUISaved struct {
	path string
	err  error
}

// msgTUIExternalEdited is what ctrl+e resolves to once $EDITOR exits: path is
// the temporary file it was pointed at, which holds the new query unless err
// says the process itself failed.
type msgTUIExternalEdited struct {
	path string
	err  error
}

type tuiModel struct {
	ctx     context.Context
	clients *clients

	width, height int
	ready         bool

	focus tuiPane

	// The work group and where its results go are shown in the header; the
	// location has to be read from the work group unless it is overridden.
	workGroupName string
	output        outputSetting
	wgLoading     bool
	wgFailed      bool

	databases  []tuiDatabase
	rows       []catalogRow
	catCursor  int
	catOffset  int
	catLoading bool

	colCursor int
	colOffset int

	editor   textarea.Model
	resultVP viewport.Model

	// vim is the modal layer over the editor; see tui_vim.go. mouseOff is
	// f2: athq lets go of the mouse so the terminal can select text itself.
	vim      vimState
	mouseOff bool

	qe      *types.QueryExecution
	result  *resultTable
	errText string
	maxRows int

	running   bool
	runStart  time.Time
	cancelRun context.CancelFunc
	spinner   spinner.Model

	mode   tuiMode
	saveIn textinput.Model

	// The prompt for saving the query under a name, and the picker that opens
	// one back. savedName and savedDesc are what the last save or open used,
	// so saving the same query again suggests them.
	nameIn       textinput.Model
	descIn       textinput.Model
	savedName    string
	savedDesc    string
	savedQueries []savedQuery
	savedCursor  int
	savedOffset  int

	status    string
	statusErr bool

	// what the last tab in the editor completed, so the next one can offer
	// the following candidate
	completion tuiCompletion

	// pane geometry, recomputed on every resize
	catalogWidth, columnsWidth       int
	topHeight, editHeight, resHeight int
}

func newTUIModel(ctx context.Context, c *clients, initialSQL string, maxRows int) tuiModel {
	ta := textarea.New()
	ta.Placeholder = "SELECT ..."
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.SetValue(initialSQL)
	ta.MoveToEnd()

	si := textinput.New()
	si.Placeholder = "result.csv"

	ni := textinput.New()
	ni.Placeholder = "daily active users"

	di := textinput.New()
	di.Placeholder = "what the query answers (optional)"

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	vim := vimState{on: vimEnabled(), mode: vimInsert}
	if vim.on {
		vim.mode = vimNormal
	}

	m := tuiModel{
		ctx:           ctx,
		clients:       c,
		focus:         paneCatalog,
		workGroupName: workGroup(),
		wgLoading:     true,
		editor:        ta,
		resultVP:      viewport.New(),
		saveIn:        si,
		nameIn:        ni,
		descIn:        di,
		spinner:       sp,
		maxRows:       maxRows,
		catLoading:    true,
		status:        "loading the catalog…",
		vim:           vim,
	}
	if m.vim.on {
		// The cursor sits on a character in normal mode, not past the last
		// one where SetValue left it.
		m.vimMoveTo(m.vimBuffer(), vimPos{row: ta.Line(), col: ta.Column()})
	}
	return m
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, loadDatabasesCmd(m.ctx, m.clients), loadWorkGroupCmd(m.ctx, m.clients))
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.layout()
		return m, nil

	case spinner.TickMsg:
		if !m.running && !m.catLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case msgTUIWorkGroup:
		m.wgLoading = false
		if msg.err != nil {
			// Queries can still be run, so this only takes the status line.
			// An explicit location is still known; without one the header has
			// to admit it does not know where the results go.
			m.wgFailed = true
			if loc := outputLocation(); loc != "" {
				m.output = outputSetting{location: loc, source: outputLocationSource()}
			}
			return m.fail(msg.err), nil
		}
		m.output = msg.output
		return m, nil

	case msgTUIDatabases:
		m.catLoading = false
		if msg.err != nil {
			return m.fail(msg.err), nil
		}
		m.databases = make([]tuiDatabase, 0, len(msg.databases))
		for _, name := range msg.databases {
			m.databases = append(m.databases, tuiDatabase{name: name})
		}
		m.rows = catalogRows(m.databases)
		m.status = fmt.Sprintf("%d databases", len(m.databases))
		m.statusErr = false
		// Expand the configured database right away; it is the one the query
		// will run against.
		if db := database(); db != "" {
			for i := range m.databases {
				if m.databases[i].name == db {
					m.catCursor = i
					return m.toggleDatabase(i)
				}
			}
		}
		return m, nil

	case msgTUITables:
		for i := range m.databases {
			if m.databases[i].name != msg.database {
				continue
			}
			m.databases[i].loading = false
			m.databases[i].loaded = msg.err == nil
			if msg.err != nil {
				m.databases[i].expanded = false
				m.databases[i].loadErr = msg.err.Error()
				m.status = msg.err.Error()
				m.statusErr = true
			} else {
				m.databases[i].tables = msg.tables
				m.status = fmt.Sprintf("%s: %d tables", msg.database, len(msg.tables))
				m.statusErr = false
			}
			break
		}
		m.rows = catalogRows(m.databases)
		m.clampCatalogCursor()
		return m, nil

	case msgTUIQueryDone:
		m.running = false
		m.cancelRun = nil
		if msg.err != nil {
			m.result, m.qe = nil, msg.qe
			return m.showError(msg.err, msg.qe), nil
		}
		m.qe, m.result, m.errText = msg.qe, msg.result, ""
		m.refreshResultPane()
		m.resultVP.SetYOffset(0)
		m.resultVP.SetXOffset(0)
		m.status = queryStatsLine(msg.qe, msg.result)
		m.statusErr = false
		m.focus = paneResult
		m.editor.Blur()
		return m, nil

	case msgTUISaved:
		if msg.err != nil {
			return m.fail(msg.err), nil
		}
		m.status = "saved to " + msg.path
		m.statusErr = false
		return m, nil

	case msgTUIExternalEdited:
		return m.finishExternalEdit(msg)

	case msgTUIQuerySaved:
		if msg.err != nil {
			return m.fail(msg.err), nil
		}
		verb := "saved the query as "
		if msg.replaced {
			verb = "replaced the saved query "
		}
		m.status = verb + msg.name
		m.statusErr = false
		return m, nil

	case msgTUISavedQueries:
		if msg.err != nil {
			return m.fail(msg.err), nil
		}
		return m.showSavedQueries(msg.queries), nil

	case msgTUICopied:
		return m.showCopied(msg), nil

	case msgTUIPasted:
		return m.showPasted(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.PasteMsg:
		return m.handlePaste(msg)

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)

	case tea.MouseMotionMsg:
		return m.handleMouseMotion(msg)

	case tea.MouseReleaseMsg:
		return m.handleMouseRelease(msg)

	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	}

	return m, nil
}

func (m tuiModel) fail(err error) tuiModel {
	m.status = err.Error()
	m.statusErr = true
	return m
}

// showError puts the whole message in the result pane. Athena's reasons run
// well past one line, and the status bar can only ever show their beginning.
func (m tuiModel) showError(err error, qe *types.QueryExecution) tuiModel {
	m.errText = errorDetail(err, qe)
	m.refreshResultPane()
	m.resultVP.SetYOffset(0)
	m.resultVP.SetXOffset(0)
	m.focus = paneResult
	m.editor.Blur()
	return m.fail(err)
}

// errorDetail is the text of the error plus the execution id, which is what
// the console and the AWS support need to look the failure up. There is no id
// when the query never started.
func errorDetail(err error, qe *types.QueryExecution) string {
	text := err.Error()
	if qe == nil {
		return text
	}
	if id := aws.ToString(qe.QueryExecutionId); id != "" {
		text += "\n\nexecution id: " + id
	}
	return text
}

// refreshResultPane fills the viewport with the error, or with the result when
// there is none. It runs again on a resize so the text is rewrapped.
func (m *tuiModel) refreshResultPane() {
	if m.errText != "" {
		m.resultVP.SetContent(styleTUIError.Render(wrapText(m.errText, m.resultVP.Width())))
		return
	}
	m.resultVP.SetContent(renderResultContent(m.result))
}

// wrapText breaks s to fit width cells. A word longer than the line is split
// rather than left hanging off the edge.
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			for tuiWidth.StringWidth(word) > width {
				head := tuiWidth.Truncate(word, width, "")
				if line != "" {
					out = append(out, line)
					line = ""
				}
				out = append(out, head)
				word = word[len(head):]
			}
			switch {
			case line == "":
				line = word
			case tuiWidth.StringWidth(line)+1+tuiWidth.StringWidth(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func (m tuiModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal {
		return m.handleModeKey(msg)
	}

	// While a query runs, Ctrl-C cancels it instead of quitting; the Athena
	// side is stopped as well.
	if key.Matches(msg, tuiKeys.Cancel) {
		if m.running && m.cancelRun != nil {
			m.cancelRun()
			m.status = "cancelling…"
			m.statusErr = false
			return m, nil
		}
		return m, tea.Quit
	}

	// Letting go of the mouse works everywhere, and is how text is selected
	// out of the panes athq does not select in itself.
	if key.Matches(msg, tuiKeys.ToggleMouse) {
		return m.toggleMouse(), nil
	}

	// In the editor tab completes the name being typed instead of moving on;
	// shift+tab and esc still leave the pane. Any other key ends a run of
	// completions. In vim's normal mode nothing is being typed, so tab moves
	// between the panes there like everywhere else.
	if m.focus == paneEditor && m.editorTyping() && key.Matches(msg, tuiKeys.Complete) {
		return m.completeWord()
	}
	m.completion = tuiCompletion{}

	switch {
	case key.Matches(msg, tuiKeys.NextPane):
		return m.setFocus((m.focus + 1) % paneCount)
	case key.Matches(msg, tuiKeys.PrevPane):
		return m.setFocus((m.focus + paneCount - 1) % paneCount)
	case key.Matches(msg, tuiKeys.Run):
		return m.startQuery()
	case key.Matches(msg, tuiKeys.Save):
		return m.startSave()
	case key.Matches(msg, tuiKeys.SaveQuery):
		return m.startSaveQuery()
	case key.Matches(msg, tuiKeys.OpenQuery):
		return m.startOpenQuery()
	case key.Matches(msg, tuiKeys.EditExternal):
		return m.startExternalEdit()
	}

	if m.focus == paneEditor {
		return m.handleEditorKey(msg)
	}

	if key.Matches(msg, tuiKeys.Quit) {
		return m, tea.Quit
	}

	switch m.focus {
	case paneCatalog:
		return m.handleCatalogKey(msg)
	case paneColumns:
		return m.handleColumnsKey(msg)
	case paneResult:
		var cmd tea.Cmd
		m.resultVP, cmd = m.resultVP.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handlePaste inserts a terminal-native (bracketed) paste into the editor.
// This is what most terminals actually send for the platform paste shortcut
// (e.g. cmd+v on a Mac) instead of a literal ctrl+v key press, which is why
// pasting needs handling here in addition to textarea's own ctrl+v binding.
// Pasting only means something in the editor, and only outside a prompt,
// which does not change m.focus while it is open.
func (m tuiModel) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal || m.focus != paneEditor {
		return m, nil
	}
	return m.insertPastedText(msg.Content), nil
}

// insertPastedText puts pasted text into the editor. While inserting it goes
// in at the cursor; in vim's normal mode it is put beside it, the way p does,
// since there is no insertion point there.
func (m tuiModel) insertPastedText(text string) tuiModel {
	if text == "" {
		return m
	}
	if m.vim.on && m.vim.mode != vimInsert {
		return m.vimPut(text, strings.HasSuffix(text, "\n"), true, 1)
	}
	m.editor, _ = m.editor.Update(tea.PasteMsg{Content: text})
	m.vimSyncFromEditor()
	return m
}

// editorTyping reports whether keys typed in the editor are text rather than
// commands, which is always so unless vim mode is waiting in normal or visual
// mode.
func (m tuiModel) editorTyping() bool { return !m.vim.on || m.vim.mode == vimInsert }

// handleEditorKey routes a key that landed in the query editor. Copying and
// pasting are handled first because they mean the same thing in every mode;
// the modal layer takes the rest unless the editor is inserting, where the
// text area's own bindings (ctrl+a, ctrl+k, alt+f …) still apply.
func (m tuiModel) handleEditorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, tuiKeys.Copy):
		return m.copySelection()
	case key.Matches(msg, tuiKeys.Paste):
		return m, pasteFromClipboardCmd()
	}

	if m.vim.on && m.vim.mode != vimInsert {
		return m.vimKey(msg)
	}

	if key.Matches(msg, tuiKeys.Escape) {
		if m.vim.on {
			// esc stops the typing; a second one leaves the pane.
			m.vimLeaveInsert()
			return m, nil
		}
		return m.setFocus(paneCatalog)
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	m.vimSyncFromEditor()
	return m, cmd
}

// copySelection puts whatever is selected in the editor on the system
// clipboard. In visual mode it is y, so that the register is filled as well.
func (m tuiModel) copySelection() (tea.Model, tea.Cmd) {
	if m.vim.on && m.vim.mode.visual() {
		return m.vimKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	}
	text := m.editor.SelectedText()
	if text == "" {
		m.status = "nothing is selected"
		m.statusErr = false
		return m, nil
	}
	return m, copySelectionCmd(text)
}

// showCopied reports what copying did. A helper program that failed is not an
// error as such: the same text went out as an OSC 52 sequence, which the
// terminal may well have acted on.
func (m tuiModel) showCopied(msg msgTUICopied) tuiModel {
	m.statusErr = false
	m.status = fmt.Sprintf("copied %s", plural(msg.runes, "character"))
	if msg.err != nil {
		m.status += fmt.Sprintf(" through the terminal (%s)", msg.err)
	}
	return m
}

// showPasted puts what the clipboard held into the editor.
func (m tuiModel) showPasted(msg msgTUIPasted) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = "cannot read the clipboard: " + msg.err.Error() +
			"; the terminal's own paste (ctrl+shift+v, cmd+v) works without one"
		m.statusErr = true
		return m, nil
	}
	if m.mode != modeNormal || m.focus != paneEditor {
		return m, nil
	}
	return m.insertPastedText(msg.text), nil
}

// toggleMouse hands the mouse to the terminal and back. While athq holds it,
// selecting text with the pointer needs the terminal's own modifier (shift in
// Windows Terminal and Ghostty, fn in Terminal.app), which is easy to forget.
func (m tuiModel) toggleMouse() tuiModel {
	m.mouseOff = !m.mouseOff
	m.statusErr = false
	if m.mouseOff {
		m.status = "the mouse is the terminal's now: select and copy as usual, f2 takes it back"
	} else {
		m.status = "the mouse is athq's again"
	}
	return m
}

// plural is "1 character" or "2 characters".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// handleModeKey routes the keyboard to whatever prompt or overlay is open.
func (m tuiModel) handleModeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSaveResult:
		return m.handleSaveKey(msg)
	case modeQueryName, modeQueryDesc:
		return m.handleSaveQueryKey(msg)
	case modeOpenQuery:
		return m.handleOpenQueryKey(msg)
	}
	return m, nil
}

func (m tuiModel) handleSaveKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.saveIn.Blur()
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.saveIn.Value())
		m.mode = modeNormal
		m.saveIn.Blur()
		if path == "" {
			return m, nil
		}
		if m.qe == nil {
			return m.fail(fmt.Errorf("there is no result to save")), nil
		}
		m.status = "saving to " + path + "…"
		m.statusErr = false
		return m, saveResultCmd(m.ctx, m.clients, m.qe, path)
	}
	var cmd tea.Cmd
	m.saveIn, cmd = m.saveIn.Update(msg)
	return m, cmd
}

func (m tuiModel) handleCatalogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, tuiKeys.Up):
		if m.catCursor > 0 {
			m.catCursor--
		}
	case key.Matches(msg, tuiKeys.Down):
		if m.catCursor < len(m.rows)-1 {
			m.catCursor++
		}
	case key.Matches(msg, tuiKeys.PageUp):
		m.catCursor -= m.catalogVisibleRows()
	case key.Matches(msg, tuiKeys.PageDown):
		m.catCursor += m.catalogVisibleRows()
	case key.Matches(msg, tuiKeys.Toggle):
		if row, ok := m.currentRow(); ok && row.isDatabase() {
			return m.toggleDatabase(row.db)
		}
		return m.setFocus(paneColumns)
	case key.Matches(msg, tuiKeys.Right):
		if row, ok := m.currentRow(); ok && row.isDatabase() && !m.databases[row.db].expanded {
			return m.toggleDatabase(row.db)
		}
		return m.setFocus(paneColumns)
	case key.Matches(msg, tuiKeys.Left):
		if row, ok := m.currentRow(); ok && row.isDatabase() && m.databases[row.db].expanded {
			return m.toggleDatabase(row.db)
		}
	case key.Matches(msg, tuiKeys.Insert):
		return m.insertCurrentName()
	case key.Matches(msg, tuiKeys.Reload):
		m.catLoading = true
		m.databases = nil
		m.rows = nil
		m.catCursor, m.catOffset = 0, 0
		m.status = "reloading the catalog…"
		m.statusErr = false
		return m, tea.Batch(m.spinner.Tick, loadDatabasesCmd(m.ctx, m.clients))
	}
	m.clampCatalogCursor()
	return m, nil
}

func (m tuiModel) handleColumnsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	columns := m.currentColumns()
	switch {
	case key.Matches(msg, tuiKeys.Up):
		if m.colCursor > 0 {
			m.colCursor--
		}
	case key.Matches(msg, tuiKeys.Down):
		if m.colCursor < len(columns)-1 {
			m.colCursor++
		}
	case key.Matches(msg, tuiKeys.PageUp):
		m.colCursor -= m.columnsVisibleRows()
	case key.Matches(msg, tuiKeys.PageDown):
		m.colCursor += m.columnsVisibleRows()
	case key.Matches(msg, tuiKeys.Left):
		return m.setFocus(paneCatalog)
	case key.Matches(msg, tuiKeys.Insert), key.Matches(msg, tuiKeys.Toggle):
		if m.colCursor < len(columns) {
			return m.insert(columns[m.colCursor].name)
		}
	}
	if m.colCursor >= len(columns) {
		m.colCursor = max(0, len(columns)-1)
	}
	if m.colCursor < 0 {
		m.colCursor = 0
	}
	m.colOffset = scrollOffset(m.colCursor, m.colOffset, m.columnsVisibleRows())
	return m, nil
}

func (m tuiModel) setFocus(p tuiPane) (tea.Model, tea.Cmd) {
	m.focus = p
	m.completion = tuiCompletion{}
	if p == paneEditor {
		// The pane is entered in normal mode, the way vi opens a file.
		if m.vim.on {
			m.vim.mode = vimNormal
			m.vim.clearPending()
			m.editor.ClearSelection()
			m.vimSyncFromEditor()
		}
		return m, m.editor.Focus()
	}
	m.editor.Blur()
	return m, nil
}

func (m tuiModel) toggleDatabase(i int) (tea.Model, tea.Cmd) {
	db := &m.databases[i]
	db.expanded = !db.expanded
	var cmd tea.Cmd
	if db.expanded && !db.loaded && !db.loading {
		db.loading = true
		cmd = loadTablesCmd(m.ctx, m.clients, db.name)
	}
	m.rows = catalogRows(m.databases)
	m.clampCatalogCursor()
	return m, cmd
}

// insertCurrentName puts the selected database or table name into the editor.
func (m tuiModel) insertCurrentName() (tea.Model, tea.Cmd) {
	row, ok := m.currentRow()
	if !ok {
		return m, nil
	}
	if row.isDatabase() {
		return m.insert(m.databases[row.db].name)
	}
	db := m.databases[row.db]
	return m.insert(db.name + "." + db.tables[row.table].name)
}

func (m tuiModel) insert(text string) (tea.Model, tea.Cmd) {
	m.completion = tuiCompletion{}
	m.editor.InsertString(text)
	m.vimSyncFromEditor()
	m.status = "inserted " + text
	m.statusErr = false
	return m, nil
}

func (m tuiModel) startQuery() (tea.Model, tea.Cmd) {
	if m.running {
		return m, nil
	}
	sql := strings.TrimSpace(m.editor.Value())
	if isBlankQuery(sql) {
		return m.fail(fmt.Errorf("the query is empty")), nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelRun = cancel
	m.running = true
	m.errText = ""
	m.runStart = time.Now()
	m.status = ""
	m.statusErr = false
	return m, tea.Batch(m.spinner.Tick, runQueryTUICmd(ctx, m.clients, sql, m.maxRows))
}

func (m tuiModel) startSave() (tea.Model, tea.Cmd) {
	if m.qe == nil {
		return m.fail(fmt.Errorf("there is no result to save")), nil
	}
	m.mode = modeSaveResult
	m.saveIn.SetValue("")
	return m, m.saveIn.Focus()
}

// startExternalEdit hands the query in the editor to $EDITOR, for whatever
// the built-in vim keys cannot do. The terminal is let go for the editor's
// own screen the way [tea.ExecProcess] does it, and [tuiModel.finishExternalEdit]
// picks the result back up once the process exits. Available from any pane,
// like ctrl+r, since it always acts on the query in the editor rather than
// whichever pane has the focus.
func (m tuiModel) startExternalEdit() (tea.Model, tea.Cmd) {
	path, err := tempQueryFile(m.editor.Value())
	if err != nil {
		return m.fail(err), nil
	}
	editor := editorCommand()
	cmd := exec.Command(editor[0], append(editor[1:], path)...) // #nosec G204 -- the editor comes from the user's own environment
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return msgTUIExternalEdited{path: path, err: err}
	})
}

// finishExternalEdit loads what $EDITOR saved back into the buffer, in normal
// mode with the cursor at the top left, the way vi opens a file. A change
// that came in this way is still one keypress from undone: the buffer before
// it is pushed like any other command that replaces it.
func (m tuiModel) finishExternalEdit(msg msgTUIExternalEdited) (tea.Model, tea.Cmd) {
	defer func() { _ = os.Remove(msg.path) }()
	if msg.err != nil {
		return m.fail(fmt.Errorf("the editor failed: %w", msg.err)), nil
	}
	b, err := os.ReadFile(msg.path)
	if err != nil {
		return m.fail(err), nil
	}

	if m.vim.on {
		m.vimPushUndo()
	}
	m.editor.SetValue(strings.TrimSuffix(string(b), "\n"))
	m.editor.MoveToBegin()
	m.status = "updated the query from the editor"
	m.statusErr = false
	return m.setFocus(paneEditor)
}

func (m tuiModel) currentRow() (catalogRow, bool) {
	if m.catCursor < 0 || m.catCursor >= len(m.rows) {
		return catalogRow{}, false
	}
	return m.rows[m.catCursor], true
}

// currentColumns returns the columns of the table under the catalog cursor.
func (m tuiModel) currentColumns() []tuiColumn {
	row, ok := m.currentRow()
	if !ok || row.isDatabase() {
		return nil
	}
	return m.databases[row.db].tables[row.table].columns
}

func (m tuiModel) currentTableName() string {
	row, ok := m.currentRow()
	if !ok || row.isDatabase() {
		return ""
	}
	db := m.databases[row.db]
	return db.name + "." + db.tables[row.table].name
}

func (m *tuiModel) clampCatalogCursor() {
	if m.catCursor >= len(m.rows) {
		m.catCursor = len(m.rows) - 1
	}
	if m.catCursor < 0 {
		m.catCursor = 0
	}
	m.catOffset = scrollOffset(m.catCursor, m.catOffset, m.catalogVisibleRows())
	m.colCursor, m.colOffset = 0, 0
}

// scrollOffset keeps the cursor inside the visible window.
func scrollOffset(cursor, offset, visible int) int {
	if visible <= 0 {
		return 0
	}
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+visible {
		return cursor - visible + 1
	}
	return offset
}

func runQueryTUICmd(ctx context.Context, c *clients, sql string, maxRows int) tea.Cmd {
	return func() tea.Msg {
		qe, err := runStatement(ctx, c, sql, newDiscardProgress())
		if err != nil {
			return msgTUIQueryDone{qe: qe, err: err}
		}
		rt, err := fetchResults(ctx, c.athena, aws.ToString(qe.QueryExecutionId), qe.StatementType, maxRows)
		return msgTUIQueryDone{qe: qe, result: rt, err: err}
	}
}

func saveResultCmd(ctx context.Context, c *clients, qe *types.QueryExecution, path string) tea.Cmd {
	return func() tea.Msg {
		// Saving always writes every row, not just the ones on screen.
		err := emitResults(ctx, c, io.Discard, qe, path, formatFromPath(path), 0)
		return msgTUISaved{path: path, err: err}
	}
}

func queryStatsLine(qe *types.QueryExecution, rt *resultTable) string {
	var scanned, millis int64
	if qe != nil && qe.Statistics != nil {
		scanned = aws.ToInt64(qe.Statistics.DataScannedInBytes)
		millis = aws.ToInt64(qe.Statistics.TotalExecutionTimeInMillis)
	}
	rows := 0
	if rt != nil {
		rows = len(rt.rows)
	}
	more := ""
	if rt != nil && rt.truncated {
		more = "+"
	}
	return fmt.Sprintf("%d%s rows in %s, scanned %s (~$%.4f)",
		rows, more,
		formatDuration(time.Duration(millis)*time.Millisecond),
		humanBytes(scanned),
		estimateCost(scanned, pricePerTB()),
	)
}

func renderResultContent(rt *resultTable) string {
	if rt == nil || len(rt.columns) == 0 {
		return styleTUIDim.Render("(no result)")
	}
	var sb strings.Builder
	if err := writeTable(&sb, rt, 0); err != nil {
		return err.Error()
	}
	if len(rt.rows) == 0 {
		sb.WriteString(styleTUIDim.Render("(no rows)") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// tuiWidth measures the way lipgloss does, with East Asian ambiguous
// characters counting as one cell. The locale aware default would make athq
// and lipgloss disagree about characters such as "…" and the tree markers, and
// the panes would stop lining up on a Japanese locale.
var tuiWidth = &runewidth.Condition{EastAsianWidth: false}

// truncatePad fits plain text to exactly w cells so that a selected row's
// background covers the whole line.
func truncatePad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if tuiWidth.StringWidth(s) > w {
		// Cutting between the halves of a wide character leaves the line one
		// cell short, so pad afterwards either way.
		s = tuiWidth.Truncate(s, w, "…")
	}
	return tuiWidth.FillRight(s, w)
}

// padANSI pads already styled text, whose escape sequences must not be counted
// as content.
func padANSI(s string, w int) string {
	if pad := w - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
