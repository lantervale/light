package history

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Load reads the log saved at path, keeping its last limit visits. A file
// that is not there is an empty log; one that cannot be read is an empty
// log too, and the reason.
func Load(path string, limit int) (*Log, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return New(limit), nil
	case err != nil:
		return New(limit), fmt.Errorf("reading the history: %w", err)
	}
	return Decode(data, limit)
}

// Save writes l at path, making its folder, the file whole or not at all.
func Save(path string, l *Log) error {
	data, err := l.Encode()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("saving the history: %w", err)
	}
	draft := path + ".tmp"
	if err = os.WriteFile(draft, data, 0o600); err != nil {
		return fmt.Errorf("saving the history: %w", err)
	}
	if err = os.Rename(draft, path); err != nil {
		return fmt.Errorf("saving the history: %w", err)
	}
	return nil
}
