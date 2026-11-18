package app

import (
	"path/filepath"
	"time"
)

// saveEvery is the least time between two writes of a file.
const saveEvery = 2 * time.Second

// store is one file of the folder light is given: written once what it
// holds changed, not more often than saveEvery, and as light quits.
type store struct {
	name    string
	write   func(path string) error
	changed bool
	written time.Time
}

// touch says what the file holds changed.
func (st *store) touch() { st.changed = true }

// keep writes the file in dir if it changed and was not written lately.
func (st *store) keep(dir string) error {
	if time.Since(st.written) < saveEvery {
		return nil
	}
	return st.flush(dir)
}

// path is where the file is in dir.
func (st *store) path(dir string) string { return filepath.Join(dir, st.name) }

// flush writes the file in dir if it changed; with no folder, nothing
// is kept.
func (st *store) flush(dir string) error {
	if !st.changed || dir == "" {
		return nil
	}
	st.written = time.Now()
	if err := st.write(st.path(dir)); err != nil {
		return err
	}
	st.changed = false
	return nil
}
