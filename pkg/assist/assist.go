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

// Features are the features turned off, by their ids, and of those
// that are asked for the ones turned on.
type Features struct {
	off, asks, on map[string]bool
}

type file struct {
	Off []string `json:"off"`
	On  []string `json:"on,omitempty"`
}

func New() *Features {
	return &Features{off: map[string]bool{}, asks: map[string]bool{}, on: map[string]bool{}}
}

// Ask has the feature id off until its user turns it on.
func (f *Features) Ask(id string) { f.asks[id] = true }

// On reports whether the model helps with the feature id.
func (f *Features) On(id string) bool {
	if f.asks[id] {
		return f.on[id]
	}
	return !f.off[id]
}

// Set turns the feature id on or off, and reports whether that changed
// it.
func (f *Features) Set(id string, on bool) bool {
	if f.On(id) == on {
		return false
	}
	switch {
	case f.asks[id] && on:
		f.on[id] = true
	case f.asks[id]:
		delete(f.on, id)
	case on:
		delete(f.off, id)
	default:
		f.off[id] = true
	}
	return true
}

func (f *Features) Encode() ([]byte, error) {
	kept := file{Off: []string{}}
	for id := range f.off {
		kept.Off = append(kept.Off, id)
	}
	for id := range f.on {
		kept.On = append(kept.On, id)
	}
	slices.Sort(kept.Off)
	slices.Sort(kept.On)
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
	for _, id := range kept.On {
		f.on[id] = true
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
