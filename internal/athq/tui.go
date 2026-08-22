package athq

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

// runTUI opens the three pane browser: the catalog on top, the editor in the
// middle and the result at the bottom.
func runTUI(ctx context.Context, c *clients, initialSQL string, maxRows int) error {
	var options []tea.ProgramOption

	// The query may have arrived on a pipe, in which case stdin is exhausted
	// and cannot drive the UI; read the keyboard from the terminal instead.
	if isPiped(os.Stdin) {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return fmt.Errorf("cannot open the terminal for the TUI: %w", err)
		}
		defer func() { _ = tty.Close() }()
		options = append(options, tea.WithInput(tty))
	}

	p := tea.NewProgram(newTUIModel(ctx, c, initialSQL, maxRows), options...)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("the TUI failed: %w", err)
	}
	return nil
}
