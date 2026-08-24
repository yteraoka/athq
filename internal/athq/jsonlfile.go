package athq

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// writeJSONLAtomic rewrites path with whatever encode writes, through a
// temporary file so a crash cannot leave a half written listing behind. The
// files hold query text, so they are created user readable only.
func writeJSONLAtomic(path string, encode func(*json.Encoder) error) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".athq-*")
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	w := bufio.NewWriter(tmp)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := encode(enc); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// readJSONLFile decodes one object per line. A damaged line is skipped rather
// than failing the whole read, and a missing file is simply empty.
func readJSONLFile[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var items []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			continue
		}
		items = append(items, item)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return items, nil
}
