package downloads

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// interrupted is why a download still active in the file did not end.
const interrupted = "light quit before it ended"

// list is the file's shape.
type list struct {
	Version int    `json:"version"`
	Items   []Item `json:"items"`
}

// Load reads the items saved at path, newest first, keeping the newest
// limit; one that was active is failed. A file that is not there is no
// item; one that cannot be read is no item, and the reason.
func Load(path string, limit int) ([]Item, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reading the downloads: %w", err)
	}
	var l list
	if err = json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("reading the downloads: %w", err)
	}
	items := l.Items[:min(limit, len(l.Items))]
	for i := range items {
		if items[i].State == Active {
			items[i].State, items[i].Err = Failed, interrupted
		}
	}
	return items, nil
}

// Save writes items at path, making its folder, the file whole or not at
// all.
func Save(path string, items []Item) error {
	data, err := json.MarshalIndent(list{Version: 1, Items: items}, "", "\t")
	if err != nil {
		return fmt.Errorf("saving the downloads: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("saving the downloads: %w", err)
	}
	draft := path + ".tmp"
	if err = os.WriteFile(draft, data, 0o600); err != nil {
		return fmt.Errorf("saving the downloads: %w", err)
	}
	if err = os.Rename(draft, path); err != nil {
		return fmt.Errorf("saving the downloads: %w", err)
	}
	return nil
}
