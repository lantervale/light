package categories

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type file struct {
	Categories *[]Category               `json:"categories"`
	Sites      []Choice                  `json:"sites"`
	Pages      []Choice                  `json:"pages"`
	Read       map[string]map[string]int `json:"read"`
}

// Encode is the list as JSON.
func (l *List) Encode() ([]byte, error) {
	own := l.own
	if own == nil {
		own = []Category{}
	}
	kept := file{Categories: &own, Sites: l.sites, Pages: l.pages, Read: l.read}
	if kept.Sites == nil {
		kept.Sites = []Choice{}
	}
	if kept.Pages == nil {
		kept.Pages = []Choice{}
	}
	data, err := json.MarshalIndent(kept, "", "  ")
	return append(data, '\n'), err
}

// Decode reads a list from JSON: the shipped one where it names no
// categories. What does not hold is dropped: a category without a name
// or named twice or as Default, a mark or an action light does not
// know, a choice for a category that is not there.
func Decode(data []byte) (*List, error) {
	var kept file
	l := New()
	if err := json.Unmarshal(data, &kept); err != nil {
		return l, fmt.Errorf("reading the categories: %w", err)
	}
	if kept.Categories != nil {
		l.own = nil
		for _, c := range *kept.Categories {
			c.Name, c.Description = line(c.Name, maxName), line(c.Description, maxDescription)
			if _, taken := l.Named(c.Name); c.Name == "" || taken {
				continue
			}
			if !c.valid() {
				c.Mark = l.spare()
			}
			if !c.Action.valid() {
				c.Action = Marked
			}
			l.own = append(l.own, c)
		}
	}
	for _, c := range kept.Sites {
		if named, ok := l.Named(c.Category); ok && c.Site != "" {
			l.sites = append(l.sites, Choice{Site: c.Site, Category: named.Name})
		}
	}
	for _, c := range kept.Pages {
		if named, ok := l.Named(c.Category); ok && c.Address != "" {
			l.pages = append(l.pages, Choice{Address: c.Address, Category: named.Name})
		}
	}
	for on, pages := range kept.Read {
		for name, n := range pages {
			if n <= 0 {
				continue
			}
			if l.read[on] == nil {
				l.read[on] = map[string]int{}
			}
			l.read[on][name] = n
		}
	}
	return l, nil
}

// Load reads the list saved at path. A file that is not there is the
// shipped list; one that cannot be read is the shipped list too, and
// the reason.
func Load(path string) (*List, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return New(), nil
	case err != nil:
		return New(), fmt.Errorf("reading the categories: %w", err)
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
		return fmt.Errorf("saving the categories: %w", err)
	}
	draft := path + ".tmp"
	if err = os.WriteFile(draft, data, 0o600); err != nil {
		return fmt.Errorf("saving the categories: %w", err)
	}
	if err = os.Rename(draft, path); err != nil {
		return fmt.Errorf("saving the categories: %w", err)
	}
	return nil
}
