// Package bookmarks is light's bookmarks in plain Go: the pages kept,
// each under a name and at most once, the folders they are filed in,
// and both written to and read from JSON.
package bookmarks

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Bookmark is one page kept: under what name, where, in which folder
// ("" for none), and since when.
type Bookmark struct {
	Name    string    `json:"name"`
	Address string    `json:"address"`
	Folder  string    `json:"folder,omitempty"`
	Added   time.Time `json:"added"`
}

// Group is the bookmarks of one folder, in the order they were added.
type Group struct {
	Folder    string
	Bookmarks []Bookmark
}

// List is the bookmarks in the order they were added, and the folders in
// the order they were made.
type List struct {
	marks   []Bookmark
	folders []string
	usual   string
}

func New() *List { return &List{} }

func (l *List) Len() int { return len(l.marks) }

// All is every bookmark, in the order they were added.
func (l *List) All() []Bookmark { return slices.Clone(l.marks) }

// Folders are the folders, in the order they were made.
func (l *List) Folders() []string { return slices.Clone(l.folders) }

// Usual is the folder the last bookmark filed went to, "" for none.
func (l *List) Usual() string { return l.usual }

func (l *List) find(address string) int {
	return slices.IndexFunc(l.marks, func(b Bookmark) bool { return b.Address == address })
}

// Lookup is the bookmark of address.
func (l *List) Lookup(address string) (Bookmark, bool) {
	if i := l.find(address); i >= 0 {
		return l.marks[i], true
	}
	return Bookmark{}, false
}

// Add keeps address under name in folder, which it makes if need be, and
// gives its bookmark: the one it has already, as it is, for an address
// kept before. A bookmark given no name is named by its address.
func (l *List) Add(address, name, folder string, at time.Time) Bookmark {
	if i := l.find(address); i >= 0 {
		return l.marks[i]
	}
	if name = strings.TrimSpace(name); name == "" {
		name = address
	}
	b := Bookmark{Name: name, Address: address, Folder: l.file(folder), Added: at}
	l.marks = append(l.marks, b)
	return b
}

// file is folder as bookmarks name it, made if need be and from now on
// the usual one; "" files nothing.
func (l *List) file(folder string) string {
	if folder = strings.TrimSpace(folder); folder == "" {
		return ""
	}
	l.AddFolder(folder)
	l.usual = folder
	return folder
}

// Rename names the bookmark of address, and reports whether it did: not
// for an address that has none, nor for an empty name.
func (l *List) Rename(address, name string) bool {
	i := l.find(address)
	if name = strings.TrimSpace(name); i < 0 || name == "" {
		return false
	}
	l.marks[i].Name = name
	return true
}

// Move files the bookmark of address in folder, which it makes if need
// be; "" takes it out of its folder.
func (l *List) Move(address, folder string) bool {
	i := l.find(address)
	if i < 0 {
		return false
	}
	l.marks[i].Folder = l.file(folder)
	return true
}

// Remove forgets the bookmark of address.
func (l *List) Remove(address string) bool {
	i := l.find(address)
	if i < 0 {
		return false
	}
	l.marks = slices.Delete(l.marks, i, i+1)
	return true
}

// AddFolder makes a folder, and reports whether it did: not one that is
// there, nor one with no name.
func (l *List) AddFolder(name string) bool {
	if name = strings.TrimSpace(name); name == "" || slices.Contains(l.folders, name) {
		return false
	}
	l.folders = append(l.folders, name)
	return true
}

// RenameFolder names the folder from to, its bookmarks with it; not to
// the name of another folder.
func (l *List) RenameFolder(from, to string) bool {
	i := slices.Index(l.folders, from)
	if to = strings.TrimSpace(to); i < 0 || to == "" || slices.Contains(l.folders, to) {
		return false
	}
	l.folders[i] = to
	l.refile(from, to)
	return true
}

// RemoveFolder forgets a folder; its bookmarks stay, in none.
func (l *List) RemoveFolder(name string) bool {
	i := slices.Index(l.folders, name)
	if i < 0 {
		return false
	}
	l.folders = slices.Delete(l.folders, i, i+1)
	l.refile(name, "")
	return true
}

func (l *List) refile(from, to string) {
	for i := range l.marks {
		if l.marks[i].Folder == from {
			l.marks[i].Folder = to
		}
	}
	if l.usual == from {
		l.usual = to
	}
}

// In is the bookmarks of folder, "" for those in none, in the order they
// were added.
func (l *List) In(folder string) []Bookmark {
	var in []Bookmark
	for _, b := range l.marks {
		if b.Folder == folder {
			in = append(in, b)
		}
	}
	return in
}

// Groups is the bookmarks by folder: those in none first, when there are
// some, then every folder in the order they were made, an empty one too.
func (l *List) Groups() []Group {
	var groups []Group
	if loose := l.In(""); len(loose) > 0 {
		groups = append(groups, Group{Bookmarks: loose})
	}
	for _, folder := range l.folders {
		groups = append(groups, Group{Folder: folder, Bookmarks: l.In(folder)})
	}
	return groups
}

// file is a list as written.
type file struct {
	Bookmarks []Bookmark `json:"bookmarks"`
	Folders   []string   `json:"folders,omitempty"`
	Usual     string     `json:"usual,omitempty"`
}

func (l *List) Encode() ([]byte, error) {
	data, err := json.Marshal(file{l.marks, l.folders, l.usual})
	if err != nil {
		return nil, fmt.Errorf("writing the bookmarks: %w", err)
	}
	return data, nil
}

// Decode reads what Encode wrote: each address once, the folders it
// names and those of its bookmarks. Data it cannot read gives an empty
// list, ready to use, and the reason.
func Decode(data []byte) (*List, error) {
	l := New()
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return l, fmt.Errorf("reading the bookmarks: %w", err)
	}
	for _, folder := range f.Folders {
		l.AddFolder(folder)
	}
	for _, b := range f.Bookmarks {
		if b.Address != "" {
			l.Add(b.Address, b.Name, b.Folder, b.Added)
		}
	}
	l.usual = ""
	if slices.Contains(l.folders, f.Usual) {
		l.usual = f.Usual
	}
	return l, nil
}
