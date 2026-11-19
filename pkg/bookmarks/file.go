package bookmarks

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Load reads the list saved at path. A file that is not there is an
// empty list; one that cannot be read is an empty list too, and the
// reason.
func Load(path string) (*List, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return New(), nil
	case err != nil:
		return New(), fmt.Errorf("reading the bookmarks: %w", err)
	}
	return Decode(data)
}

// Save writes l at path, making its folder, the file whole or not at all.
func Save(path string, l *List) error {
	data, err := l.Encode()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("saving the bookmarks: %w", err)
	}
	draft := path + ".tmp"
	if err = os.WriteFile(draft, data, 0o600); err != nil {
		return fmt.Errorf("saving the bookmarks: %w", err)
	}
	if err = os.Rename(draft, path); err != nil {
		return fmt.Errorf("saving the bookmarks: %w", err)
	}
	return nil
}
