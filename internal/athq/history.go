package athq

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	maxHistoryEntries = 1000
	envHistoryFile    = "ATHQ_HISTORY_FILE"
)

type historyEntry struct {
	Time               time.Time `json:"time"`
	QueryExecutionID   string    `json:"query_execution_id"`
	WorkGroup          string    `json:"work_group,omitempty"`
	Database           string    `json:"database,omitempty"`
	State              string    `json:"state"`
	DataScannedInBytes int64     `json:"data_scanned_in_bytes"`
	Query              string    `json:"query"`
}

func historyPath() (string, error) {
	if v := os.Getenv(envHistoryFile); v != "" {
		return v, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to locate the cache directory: %w", err)
	}
	return filepath.Join(dir, "athq", "history.jsonl"), nil
}

// appendHistory adds one entry, keeping at most maxHistoryEntries lines. The
// file can hold query text, so it is written user readable only.
func appendHistory(path string, e historyEntry) error {
	entries, err := readHistoryFile(path)
	if err != nil {
		return err
	}
	entries = append(entries, e)
	if len(entries) > maxHistoryEntries {
		entries = entries[len(entries)-maxHistoryEntries:]
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".history-*")
	if err != nil {
		return fmt.Errorf("failed to write the history: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write the history: %w", err)
	}
	w := bufio.NewWriter(tmp)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("failed to write the history: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write the history: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write the history: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to write the history: %w", err)
	}
	return nil
}

func readHistoryFile(path string) ([]historyEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var entries []historyEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e historyEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			// A damaged line should not make the whole command fail.
			continue
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return entries, nil
}

// readHistory returns the newest n entries, oldest first. n <= 0 returns all.
func readHistory(path string, n int) ([]historyEntry, error) {
	entries, err := readHistoryFile(path)
	if err != nil {
		return nil, err
	}
	if n > 0 && len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries, nil
}

// lastSucceeded returns the most recent successful execution.
func lastSucceeded(entries []historyEntry) (historyEntry, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].State == "SUCCEEDED" {
			return entries[i], true
		}
	}
	return historyEntry{}, false
}

func recordHistory(e historyEntry) {
	path, err := historyPath()
	if err != nil {
		return
	}
	if err := appendHistory(path, e); err != nil {
		fmt.Fprintln(os.Stderr, "athq: warning:", err)
	}
}

var historyOpts struct {
	count int
	full  bool
}

var historyCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"hist"},
	Short:   "Show recently run queries",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := historyPath()
		if err != nil {
			return err
		}
		entries, err := readHistory(path, historyOpts.count)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "no query history yet")
			return nil
		}

		// --full prints each entry as a block so the query can be copied back
		// out unchanged; a table would have to fold it onto one line.
		if historyOpts.full {
			w := cmd.OutOrStdout()
			for i := len(entries) - 1; i >= 0; i-- {
				e := entries[i]
				_, _ = fmt.Fprintf(w, "%s  %s  %s  %s\n%s\n\n",
					e.Time.Local().Format("2006-01-02 15:04:05"),
					e.QueryExecutionID,
					e.State,
					humanBytes(e.DataScannedInBytes),
					e.Query,
				)
			}
			return nil
		}

		headers := []string{"TIME", "EXECUTION ID", "STATE", "SCANNED", "QUERY"}
		rows := make([][]string, 0, len(entries))
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			rows = append(rows, []string{
				e.Time.Local().Format("2006-01-02 15:04:05"),
				e.QueryExecutionID,
				e.State,
				humanBytes(e.DataScannedInBytes),
				summarizeQuery(e.Query),
			})
		}
		return renderTable(cmd.OutOrStdout(), headers, rows, []bool{false, false, false, true, false}, terminalWidth(os.Stdout))
	},
}

// summarizeQuery folds a query onto one line for the listing.
func summarizeQuery(q string) string {
	fields := strings.Fields(q)
	return strings.Join(fields, " ")
}

func init() {
	historyCmd.Flags().IntVarP(&historyOpts.count, "count", "n", 20, "number of entries to show (0 for all)")
	historyCmd.Flags().BoolVar(&historyOpts.full, "full", false, "print each entry as a block with the full query")
	rootCmd.AddCommand(historyCmd)
}
