package athq

import "charm.land/bubbles/v2/key"

// tuiKeyMap holds the bindings that work outside the editor. The editor keeps
// its own bindings; only the ones listed here are taken away from it.
type tuiKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Left     key.Binding
	Right    key.Binding
	Toggle   key.Binding
	Insert   key.Binding
	NextPane key.Binding
	PrevPane key.Binding
	Run      key.Binding
	Save     key.Binding
	Reload   key.Binding
	Escape   key.Binding
	Quit     key.Binding
	Cancel   key.Binding
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
	Run: key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("^r", "run"),
	),
	Save: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("^s", "save"),
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
}
