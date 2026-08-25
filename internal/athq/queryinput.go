package athq

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const editorTemplate = `-- Write the query to run and save this file.
-- Lines starting with -- are sent to Athena as comments.
`

// resolveQuery decides where the SQL comes from. Exactly one source may be
// given; when none is, a piped stdin is used.
func resolveQuery(args []string, file string, useEditor bool, stdin *os.File) (string, error) {
	sources := 0
	if len(args) > 0 {
		sources++
	}
	if file != "" {
		sources++
	}
	if useEditor {
		sources++
	}
	if sources > 1 {
		return "", errors.New("give the query in only one way: an argument, --file or --editor")
	}

	switch {
	case useEditor:
		return queryFromEditor()
	case file != "":
		return queryFromFile(file)
	case len(args) > 0:
		return checkQuery(strings.Join(args, " "))
	case isPiped(stdin):
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read the query from stdin: %w", err)
		}
		return checkQuery(string(b))
	}
	return "", errors.New("no query given: pass it as an argument, with --file, with --editor or on stdin")
}

// initialTUIQuery resolves the query to preload into the TUI editor. Unlike
// resolveQuery, having no source at all is not an error: the editor just
// starts empty.
func initialTUIQuery(args []string, file string, useEditor bool, stdin *os.File) (string, error) {
	if len(args) == 0 && file == "" && !useEditor && !isPiped(stdin) {
		return "", nil
	}
	return resolveQuery(args, file, useEditor, stdin)
}

func queryFromFile(path string) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read the query from stdin: %w", err)
		}
		return checkQuery(string(b))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}
	return checkQuery(string(b))
}

// queryFromEditor opens $EDITOR on a temporary file and runs whatever was
// saved.
func queryFromEditor() (string, error) {
	f, err := os.CreateTemp("", "athq-*.sql")
	if err != nil {
		return "", fmt.Errorf("failed to create a temporary file: %w", err)
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()

	if _, err := f.WriteString(editorTemplate); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("failed to write %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", name, err)
	}

	editor := editorCommand()
	cmd := exec.Command(editor[0], append(editor[1:], name)...) // #nosec G204 -- the editor comes from the user's own environment
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %s failed: %w", editor[0], err)
	}

	b, err := os.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", name, err)
	}
	return checkQuery(string(b))
}

func editorCommand() []string {
	for _, key := range []string{"ATHQ_EDITOR", "VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return strings.Fields(v)
		}
	}
	return []string{"vi"}
}

func checkQuery(s string) (string, error) {
	q := strings.TrimSpace(s)
	if isBlankQuery(q) {
		return "", errors.New("the query is empty")
	}
	return q, nil
}

// isBlankQuery reports whether the text holds nothing but blank lines and
// comments, which is what an editor session left untouched looks like.
func isBlankQuery(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		return false
	}
	return true
}

func isPiped(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}
