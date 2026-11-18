// Package storage is what sites keep in a browser between visits, as
// localStorage gives it to their scripts: texts by key, an area a site,
// capped, and the file they are kept in between runs.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// version is the version of the file's form.
const version = 1

// MaxBytes is how much one site may keep, keys and values together.
const MaxBytes = 5 << 20

// Store is every site's area, safe to use from any goroutine.
type Store struct {
	mu      sync.Mutex
	sites   map[string]map[string]string
	changed bool
}

// New is an empty store.
func New() *Store { return &Store{sites: map[string]map[string]string{}} }

// Area is the area of the site of that origin.
func (s *Store) Area(origin string) *Area { return &Area{store: s, origin: origin} }

// Changed reports whether the store holds what its file does not.
func (s *Store) Changed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changed
}

// Area is one site's texts by key.
type Area struct {
	store  *Store
	origin string
}

func (a *Area) Get(key string) (string, bool) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	value, ok := a.store.sites[a.origin][key]
	return value, ok
}

// Set keeps value under key, unless the area would then hold more than
// MaxBytes.
func (a *Area) Set(key, value string) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	site := a.store.sites[a.origin]
	held := len(key) + len(value)
	for k, v := range site {
		if k != key {
			held += len(k) + len(v)
		}
	}
	if held > MaxBytes {
		return
	}
	if site == nil {
		site = map[string]string{}
		a.store.sites[a.origin] = site
	}
	site[key] = value
	a.store.changed = true
}

func (a *Area) Remove(key string) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if _, ok := a.store.sites[a.origin][key]; ok {
		delete(a.store.sites[a.origin], key)
		a.store.changed = true
	}
}

func (a *Area) Clear() {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if len(a.store.sites[a.origin]) > 0 {
		delete(a.store.sites, a.origin)
		a.store.changed = true
	}
}

// Keys are the area's keys, in order.
func (a *Area) Keys() []string {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	keys := make([]string, 0, len(a.store.sites[a.origin]))
	for key := range a.store.sites[a.origin] {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// file is the store as its file holds it.
type file struct {
	Version int                          `json:"version"`
	Sites   map[string]map[string]string `json:"sites"`
}

// Save writes the store at path, making its folder, the file whole or
// not at all; the sites that keep nothing are left out.
func (s *Store) Save(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := file{Version: version, Sites: map[string]map[string]string{}}
	for origin, site := range s.sites {
		if len(site) > 0 {
			out.Sites[origin] = site
		}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the sites' storage: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("saving the sites' storage: %w", err)
	}
	draft := path + ".tmp"
	if err = os.WriteFile(draft, data, 0o600); err != nil {
		return fmt.Errorf("saving the sites' storage: %w", err)
	}
	if err = os.Rename(draft, path); err != nil {
		return fmt.Errorf("saving the sites' storage: %w", err)
	}
	s.changed = false
	return nil
}

// Load reads the store saved at path. A file that is not there is an
// empty store; one that cannot be read is an empty store too, and the
// reason.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return New(), nil
	case err != nil:
		return New(), fmt.Errorf("reading the sites' storage: %w", err)
	}
	var in file
	if err = json.Unmarshal(data, &in); err != nil || in.Sites == nil {
		return New(), errors.New("the sites' storage file is not one light wrote")
	}
	s := New()
	for origin, site := range in.Sites {
		if site != nil {
			s.sites[origin] = site
		}
	}
	return s, nil
}
