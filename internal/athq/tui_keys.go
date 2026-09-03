package athq

import "charm.land/bubbles/v2/key"

// tuiKeyMap holds the bindings that work outside the editor. The editor keeps
// its own bindings; only the ones listed here are taken away from it.
type tuiKeyMap struct {
	Up           key.Binding
	Down         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	Left         key.Binding
	Right        key.Binding
	Toggle       key.Binding
	Insert       key.Binding
	NextPane     key.Binding
	PrevPane     key.Binding
	Complete     key.Binding
	Run          key.Binding
	Save         key.Binding
	SaveQuery    key.Binding
	OpenQuery    key.Binding
	EditExternal key.Binding
	Reload       key.Binding
	Escape       key.Binding
	Quit         key.Binding
	Cancel       key.Binding

	Copy        key.Binding
	Paste       key.Binding
	ToggleMouse key.Binding
}

var tuiKeys = tuiKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+b"),
		key.WithHelp("pgup", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+f"),
		key.WithHelp("pgdn", "page down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	Toggle: key.NewBinding(
		key.WithKeys("enter", " "),
		key.WithHelp("enter", "expand"),
	),
	Insert: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "insert into the query"),
	),
	NextPane: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next pane"),
	),
	PrevPane: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "previous pane"),
	),
	// tab belongs to the editor while it has the focus; shift+tab and esc are
	// what leave it then.
	Complete: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "complete the name"),
	),
	Run: key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("^r", "run"),
	),
	Save: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("^s", "save the result"),
	),
	// ^w writes the query away under a name and ^o opens one back. ^w is also
	// the editor's second binding for delete word backward; alt+backspace
	// still does that.
	SaveQuery: key.NewBinding(
		key.WithKeys("ctrl+w"),
		key.WithHelp("^w", "save the query"),
	),
	OpenQuery: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("^o", "open a saved query"),
	),
	// ^e hands the query to $EDITOR and reloads it once the process exits, for
	// whatever the built-in vim keys do not cover. It takes ctrl+e away from
	// the plain text area's own binding for that combination (move to the end
	// of the line), which the end key still does.
	EditExternal: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("^e", "edit in $EDITOR"),
	),
	Reload: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "reload the catalog"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "leave the editor"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("^c", "cancel/quit"),
	),

	// Copying is ^y, not the ctrl+shift+c that terminals show in their own
	// menus: Windows Terminal and Ghostty keep that combination for
	// themselves, and Terminal.app cannot even send it — it arrives as a bare
	// ^c, which would quit athq. It is still listed here for the terminals
	// that do pass it through. In vim mode y is the usual way.
	Copy: key.NewBinding(
		key.WithKeys("ctrl+y", "ctrl+shift+c"),
		key.WithHelp("^y", "copy the selection"),
	),
	Paste: key.NewBinding(
		key.WithKeys("ctrl+v"),
		key.WithHelp("^v", "paste"),
	),
	// Releasing the mouse hands selecting back to the terminal, which is how
	// text is copied out of the panes athq does not select in.
	ToggleMouse: key.NewBinding(
		key.WithKeys("f2"),
		key.WithHelp("f2", "release/take the mouse"),
	),
}
