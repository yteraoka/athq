package athq

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Tab completion works entirely off the catalog the TUI already holds, so it
// runs no query and calls no API: what has not been fetched yet simply has no
// candidates. The word before the cursor is read as an optional qualifier and
// the stem that follows it, and only the stem is ever replaced.

// tuiCompletion is what the last tab found, kept so that the tabs after it can
// cycle through the candidates. It is thrown away by any other key, and the
// cursor position is checked as well in case something else moved it.
type tuiCompletion struct {
	candidates []string
	// index is the candidate sitting in the editor, or -1 while the common
	// prefix is there instead.
	index     int
	length    int // runes of the stem currently in the editor
	line, col int // where the cursor was left
}

func (c tuiCompletion) active() bool { return len(c.candidates) > 0 }

// completeWord is tab in the editor. The first press completes as far as the
// candidates agree and lists them; the ones after it insert each candidate in
// turn.
func (m tuiModel) completeWord() (tea.Model, tea.Cmd) {
	if m.completion.active() && m.completion.line == m.editor.Line() && m.completion.col == m.editor.Column() {
		return m.cycleCompletion()
	}
	m.completion = tuiCompletion{}

	stem, candidates := m.completionAtCursor()
	if len(candidates) == 0 {
		return m, nil
	}

	if len(candidates) == 1 {
		return m, m.replaceStem(len([]rune(stem)), candidates[0])
	}

	// More than one: put in as much as they all share, and show the rest.
	var cmd tea.Cmd
	insert := stem
	if common := commonPrefixFold(candidates); len([]rune(common)) > len([]rune(stem)) {
		insert = common
		cmd = m.replaceStem(len([]rune(stem)), insert)
	}
	m.completion = tuiCompletion{
		candidates: candidates,
		index:      -1,
		length:     len([]rune(insert)),
		line:       m.editor.Line(),
		col:        m.editor.Column(),
	}
	m.status = completionStatus(candidates)
	m.statusErr = false
	return m, cmd
}

// cycleCompletion swaps the candidate in the editor for the next one.
func (m tuiModel) cycleCompletion() (tea.Model, tea.Cmd) {
	c := m.completion
	c.index = (c.index + 1) % len(c.candidates)
	next := c.candidates[c.index]
	cmd := m.replaceStem(c.length, next)
	c.length = len([]rune(next))
	c.line, c.col = m.editor.Line(), m.editor.Column()
	m.completion = c
	m.status = completionStatus(c.candidates)
	m.statusErr = false
	return m, cmd
}

// replaceStem swaps the n runes before the cursor for text. The editor has the
// focus whenever this runs, so its own key handling can do the deleting and
// the view is repositioned the way typing would.
func (m *tuiModel) replaceStem(n int, text string) tea.Cmd {
	cmds := make([]tea.Cmd, 0, n)
	for range n {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		cmds = append(cmds, cmd)
	}
	m.editor.InsertString(text)
	return tea.Batch(cmds...)
}

// completionAtCursor returns the stem being typed and what the catalog offers
// for it.
func (m tuiModel) completionAtCursor() (string, []string) {
	line := m.editorLine()
	col := min(m.editor.Column(), len(line))
	qualifier, stem := completionStem(line, col)
	return stem, m.completionCandidates(qualifier, stem)
}

// editorLine is the line the cursor is on, as runes: Column() counts runes too.
func (m tuiModel) editorLine() []rune {
	lines := strings.Split(m.editor.Value(), "\n")
	i := m.editor.Line()
	if i < 0 || i >= len(lines) {
		return nil
	}
	return []rune(lines[i])
}

// completionStem splits the name being typed before col into the part up to
// the last dot and the part after it. "db.ev" gives "db" and "ev", "ev" gives
// "" and "ev", and "db." gives "db" and "".
func completionStem(line []rune, col int) (qualifier, stem string) {
	i := col
	for i > 0 && isNameRune(line[i-1]) {
		i--
	}
	word := string(line[i:col])
	if dot := strings.LastIndex(word, "."); dot >= 0 {
		return word[:dot], word[dot+1:]
	}
	return "", word
}

// isNameRune says whether r can be part of an unquoted name. The dot is
// included so that the qualifier comes along with the stem.
func isNameRune(r rune) bool {
	return r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// completionCandidates collects the names the qualifier allows and keeps the
// ones the stem begins. Nothing is fetched: a database whose tables have not
// been read yet simply offers none.
func (m tuiModel) completionCandidates(qualifier, stem string) []string {
	var names, parts []string
	if qualifier != "" {
		parts = strings.Split(qualifier, ".")
	}

	switch len(parts) {
	case 0:
		// A bare word can be a database, a table of one that is already
		// loaded, or a column of the table being looked at.
		for _, db := range m.databases {
			names = append(names, db.name)
			if !db.loaded {
				continue
			}
			for _, t := range db.tables {
				names = append(names, t.name)
			}
		}
		for _, c := range m.currentColumns() {
			names = append(names, c.name)
		}
	case 1:
		if db, ok := m.loadedDatabase(parts[0]); ok {
			for _, t := range db.tables {
				names = append(names, t.name)
			}
		}
	case 2:
		if db, ok := m.loadedDatabase(parts[0]); ok {
			if t, ok := findTable(db, parts[1]); ok {
				for _, c := range t.columns {
					names = append(names, c.name)
				}
			}
		}
	}
	return matchPrefix(names, stem)
}

// loadedDatabase finds a database by name, and only reports it when its tables
// are already in memory: completing must not send athq off to fetch them.
func (m tuiModel) loadedDatabase(name string) (tuiDatabase, bool) {
	for _, db := range m.databases {
		if strings.EqualFold(db.name, name) {
			return db, db.loaded
		}
	}
	return tuiDatabase{}, false
}

func findTable(db tuiDatabase, name string) (tuiTable, bool) {
	for _, t := range db.tables {
		if strings.EqualFold(t.name, name) {
			return t, true
		}
	}
	return tuiTable{}, false
}

// matchPrefix keeps the names beginning with stem, sorted and without
// duplicates. The comparison ignores case so that a name typed in capitals
// still finds the lower case one in the catalog.
func matchPrefix(names []string, stem string) []string {
	lower := strings.ToLower(stem)
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" || seen[name] || !strings.HasPrefix(strings.ToLower(name), lower) {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// commonPrefixFold is the longest prefix all the candidates share, compared
// without case and taken from the first one.
func commonPrefixFold(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	first := []rune(candidates[0])
	n := len(first)
	for _, candidate := range candidates[1:] {
		other := []rune(candidate)
		n = min(n, len(other))
		for i := 0; i < n; i++ {
			if unicode.ToLower(first[i]) != unicode.ToLower(other[i]) {
				n = i
				break
			}
		}
	}
	return string(first[:n])
}

// completionStatus lists the candidates on the status line; the line truncates
// what does not fit.
func completionStatus(candidates []string) string {
	return fmt.Sprintf("%d candidates: %s", len(candidates), strings.Join(candidates, "  "))
}
