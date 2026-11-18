package cookies

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// version is the version of the file's form.
const version = 1

// file is the jar as its file holds it: the cookies that outlive the
// run.
type file struct {
	Version int     `json:"version"`
	Cookies []saved `json:"cookies"`
}

type saved struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	HostOnly bool      `json:"host_only,omitempty"`
	Path     string    `json:"path"`
	Secure   bool      `json:"secure,omitempty"`
	HTTPOnly bool      `json:"http_only,omitempty"`
	SameSite SameSite  `json:"same_site"`
	Expires  time.Time `json:"expires"`
	Created  time.Time `json:"created"`
	Used     time.Time `json:"used"`
}

// Save writes the jar at path, making its folder, the file whole or not
// at all and the player's alone to read. The cookies that end with the
// run and those that ended are left out.
func (j *Jar) Save(path string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := j.now()
	out := file{Version: version, Cookies: []saved{}}
	for _, site := range j.sites {
		for _, c := range site {
			if c.Persistent() && !c.expired(now) {
				out.Cookies = append(out.Cookies, saved(*c))
			}
		}
	}
	slices.SortFunc(out.Cookies, func(a, b saved) int {
		return cmp.Or(cmp.Compare(a.Domain, b.Domain), cmp.Compare(a.Path, b.Path), cmp.Compare(a.Name, b.Name))
	})
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return errors.New("encoding the cookies")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("saving the cookies: %w", err)
	}
	draft := path + ".tmp"
	if err = write(draft, data); err != nil {
		_ = os.Remove(draft)
		return fmt.Errorf("saving the cookies: %w", err)
	}
	if err = os.Rename(draft, path); err != nil {
		return fmt.Errorf("saving the cookies: %w", err)
	}
	j.changed = false
	return nil
}

// write writes a file no one but the player reads, whatever was there.
func write(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	return errors.Join(err, f.Close())
}

// Load reads the jar saved at path, without the cookies that ended
// since. A file that is not there is an empty jar; one that cannot be
// read is an empty jar too, and the reason.
func Load(path string, opts ...Option) (*Jar, error) {
	j := New(opts...)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return j, nil
	case err != nil:
		return j, fmt.Errorf("reading the cookies: %w", err)
	}
	var in file
	if err = json.Unmarshal(data, &in); err != nil || in.Version != version {
		return j, errors.New("the cookies' file is not one light wrote")
	}
	now := j.now()
	for _, s := range in.Cookies {
		c := Cookie(s)
		if c.Name == "" || c.Domain == "" || c.Path == "" || !c.Persistent() || c.expired(now) {
			continue
		}
		switch c.SameSite {
		case Lax, Strict, None:
		default:
			c.SameSite = Lax
		}
		key := registrable(c.Domain)
		j.sites[key] = append(j.sites[key], &c)
	}
	for key := range j.sites {
		j.trim(key, now)
	}
	j.changed = false
	return j, nil
}
