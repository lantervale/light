// Package assist keeps which of light's features the model on this
// device helps with: each does until its user turns it off.
package assist

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// Features are the features turned off, by their ids.
type Features struct {
	off map[string]bool
}

type file struct {
	Off []string `json:"off"`
}

func New() *Features { return &Features{off: map[string]bool{}} }

// On reports whether the model helps with the feature id.
func (f *Features) On(id string) bool { return !f.off[id] }

// Set turns the feature id on or off, and reports whether that changed
// it.
func (f *Features) Set(id string, on bool) bool {
	if f.On(id) == on {
		return false
	}
	if on {
		delete(f.off, id)
	} else {
		f.off[id] = true
	}
	return true
}

func (f *Features) Encode() ([]byte, error) {
	kept := file{Off: []string{}}
	for id := range f.off {
		kept.Off = append(kept.Off, id)
	}
	slices.Sort(kept.Off)
	data, err := json.MarshalIndent(kept, "", "  ")
	return append(data, '\n'), err
}

func Decode(data []byte) (*Features, error) {
	var kept file
	f := New()
	if err := json.Unmarshal(data, &kept); err != nil {
		return f, err
	}
	for _, id := range kept.Off {
		f.off[id] = true
	}
	return f, nil
}

// Load reads the features of the file at path: every one on when there
// is none, or with the error of one that does not read.
func Load(path string) (*Features, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return New(), err
	}
	return Decode(data)
}

// Save writes the features to path, whole or not at all.
func Save(path string, f *Features) error {
	data, err := f.Encode()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
