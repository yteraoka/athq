package athq

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const envSavedQueriesFile = "ATHQ_SAVED_QUERIES_FILE"

// savedQuery is one query the TUI has put aside under a name.
type savedQuery struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	SavedAt     time.Time `json:"saved_at"`
	Query       string    `json:"query"`
}

// athqConfigDir is $HOME/.config/athq, or the same directory under
// XDG_CONFIG_HOME when that is set. os.UserConfigDir is not used because it
// points at ~/Library/Application Support on macOS, and athq keeps its
// configuration in the same place on every platform.
func athqConfigDir() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "athq"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to locate the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "athq"), nil
}

func savedQueriesPath() (string, error) {
	if v := os.Getenv(envSavedQueriesFile); v != "" {
		return v, nil
	}
	dir, err := athqConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "queries.jsonl"), nil
}

// readSavedQueries returns the stored queries by name. Entries without a name
// cannot be picked, so they are dropped.
func readSavedQueries(path string) ([]savedQuery, error) {
	items, err := readJSONLFile[savedQuery](path)
	if err != nil {
		return nil, err
	}
	queries := make([]savedQuery, 0, len(items))
	for _, q := range items {
		if strings.TrimSpace(q.Name) == "" {
			continue
		}
		queries = append(queries, q)
	}
	sortSavedQueries(queries)
	return queries, nil
}

func writeSavedQueries(path string, queries []savedQuery) error {
	return writeJSONLAtomic(path, func(enc *json.Encoder) error {
		for _, q := range queries {
			if err := enc.Encode(q); err != nil {
				return err
			}
		}
		return nil
	})
}

// putSavedQuery stores q, replacing an entry of the same name. It reports
// whether an existing entry was replaced.
func putSavedQuery(path string, q savedQuery) (bool, error) {
	q.Name = strings.TrimSpace(q.Name)
	q.Description = strings.TrimSpace(q.Description)
	if q.Name == "" {
		return false, fmt.Errorf("the name is empty")
	}
	if strings.TrimSpace(q.Query) == "" {
		return false, fmt.Errorf("the query is empty")
	}
	if q.SavedAt.IsZero() {
		q.SavedAt = time.Now()
	}

	queries, err := readSavedQueries(path)
	if err != nil {
		return false, err
	}
	replaced := false
	for i := range queries {
		if queries[i].Name == q.Name {
			queries[i] = q
			replaced = true
			break
		}
	}
	if !replaced {
		queries = append(queries, q)
	}
	sortSavedQueries(queries)
	return replaced, writeSavedQueries(path, queries)
}

// sortSavedQueries orders by name so the picker looks the same every time.
func sortSavedQueries(queries []savedQuery) {
	sort.SliceStable(queries, func(i, j int) bool { return queries[i].Name < queries[j].Name })
}
