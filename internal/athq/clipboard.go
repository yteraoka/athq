package athq

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The system clipboard is reached in two ways at once, because no single one
// works in every terminal athq is used from:
//
//   - OSC 52, an escape sequence the terminal itself acts on. It is the only
//     way that survives an ssh session, and Windows Terminal and Ghostty do
//     it; macOS Terminal.app ignores the sequence, and some terminals ship
//     with it turned off.
//   - a helper program on the machine athq runs on (pbcopy, wl-copy, xclip,
//     xsel, or clip.exe under WSL), which is what covers Terminal.app and any
//     terminal without OSC 52.
//
// Doing both is harmless — the text lands in the same clipboard either way —
// and between them one of them works. Reading is a different matter: OSC 52
// reads are refused by most terminals, so pasting only ever asks the helper
// program, and the terminal's own paste (a bracketed paste, see
// [tuiModel.handlePaste]) covers the rest.

// clipboardTimeout is how long a helper program is given before it is killed.
// pbcopy and friends answer at once; powershell.exe under WSL is the slow one.
const clipboardTimeout = 5 * time.Second

// clipboardTool is one helper program, with the arguments that make it write
// stdin to the clipboard and the ones that make it print the clipboard back.
// read is nil for a tool that can only write.
type clipboardTool struct {
	name  string
	write []string
	read  []string
}

// clipboardTools are tried in order, and the first one on PATH wins. The
// native tools come before the Windows ones so that a Linux desktop is not
// served by clip.exe just because WSL interop put it on PATH.
var clipboardTools = []clipboardTool{
	{name: "pbcopy", write: []string{"pbcopy"}, read: []string{"pbpaste"}},
	{name: "wl-copy", write: []string{"wl-copy"}, read: []string{"wl-paste", "--no-newline"}},
	{name: "xclip", write: []string{"xclip", "-selection", "clipboard"}, read: []string{"xclip", "-selection", "clipboard", "-out"}},
	{name: "xsel", write: []string{"xsel", "--clipboard", "--input"}, read: []string{"xsel", "--clipboard", "--output"}},
	{name: "termux-clipboard-set", write: []string{"termux-clipboard-set"}, read: []string{"termux-clipboard-get"}},
	// Windows, including WSL where the .exe programs are on PATH through
	// interop. Get-Clipboard is a separate program from clip.exe, which can
	// only write.
	{name: "clip.exe", write: []string{"clip.exe"}, read: []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard -Raw"}},
	{name: "clip", write: []string{"clip"}, read: []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard -Raw"}},
}

// errNoClipboardTool is what both directions report when the machine has no
// helper program at all. The message names the ones that were looked for, so
// the status line tells the user what to install.
var errNoClipboardTool = errors.New("no clipboard program found (tried pbcopy, wl-copy, xclip, xsel, clip.exe)")

// lookupClipboardTool finds the first helper program on PATH. write says
// whether it has to be able to write; a reader is needed for pasting.
func lookupClipboardTool(write bool) (clipboardTool, string, bool) {
	for _, tool := range clipboardTools {
		argv := tool.read
		if write {
			argv = tool.write
		}
		if len(argv) == 0 {
			continue
		}
		path, err := exec.LookPath(argv[0])
		if err != nil {
			continue
		}
		return tool, path, true
	}
	return clipboardTool{}, "", false
}

// writeSystemClipboard hands the text to a helper program. It is only half of
// copying: see [copySelectionCmd].
func writeSystemClipboard(text string) error {
	tool, path, ok := lookupClipboardTool(true)
	if !ok {
		return errNoClipboardTool
	}

	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, tool.write[1:]...) // #nosec G204 -- the argv comes from clipboardTools, not from the user
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", tool.name, err)
	}
	return nil
}

// readSystemClipboard asks a helper program for the clipboard.
func readSystemClipboard() (string, error) {
	tool, path, ok := lookupClipboardTool(false)
	if !ok {
		return "", errNoClipboardTool
	}

	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, tool.read[1:]...) // #nosec G204 -- the argv comes from clipboardTools, not from the user
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w", tool.name, err)
	}
	return cleanClipboardText(string(out)), nil
}

// cleanClipboardText normalizes what a helper program printed: Windows line
// endings become plain ones, and the newline Get-Clipboard adds at the very
// end is dropped so that a single line does not paste as two.
func cleanClipboardText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSuffix(s, "\n")
}

// msgTUICopied reports what copying the selection did. err is only about the
// helper program: OSC 52 gives no answer back, so the text may well have
// reached the clipboard even when this is set.
type msgTUICopied struct {
	runes int
	err   error
}

// msgTUIPasted carries the clipboard back to the editor.
type msgTUIPasted struct {
	text string
	err  error
}

// copySelectionCmd sends the text to the terminal and to the local helper
// program at the same time; see the note at the top of this file.
func copySelectionCmd(text string) tea.Cmd {
	return tea.Batch(
		tea.SetClipboard(text),
		func() tea.Msg {
			return msgTUICopied{runes: len([]rune(text)), err: writeSystemClipboard(text)}
		},
	)
}

// pasteFromClipboardCmd reads the clipboard for ctrl+v. A terminal paste
// arrives as a [tea.PasteMsg] instead and needs none of this.
func pasteFromClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		text, err := readSystemClipboard()
		return msgTUIPasted{text: text, err: err}
	}
}
