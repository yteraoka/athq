package athq

import "charm.land/lipgloss/v2"

var (
	// Pane frames. The focused pane is the one with the bright border.
	styleTUIPane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))

	styleTUIPaneFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("212"))

	styleTUITitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("243"))

	styleTUITitleFocused = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212"))

	// Rows in the catalog and column lists.
	styleTUIRow = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	styleTUIRowSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				Background(lipgloss.Color("236"))

	styleTUIPartition = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214"))

	styleTUIDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	styleTUIError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	styleTUIHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("249")).
			Background(lipgloss.Color("234")).
			Padding(0, 1)

	styleTUIStatus = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("234")).
			Padding(0, 1)

	styleTUIStatusErr = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("9")).
				Background(lipgloss.Color("234")).
				Padding(0, 1)

	styleTUIHelp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Padding(0, 1)
)
