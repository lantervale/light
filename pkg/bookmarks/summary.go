package bookmarks

import "strings"

// Summarise notes what the page of the bookmark of address says, "" for
// nothing, and reports whether it has one.
func (l *List) Summarise(address, summary string) bool {
	i := l.find(address)
	if i < 0 {
		return false
	}
	l.marks[i].Summary = strings.TrimSpace(summary)
	return true
}

// Place files the bookmark of address in folder as Move does, the usual
// folder staying the one its user last chose: for a folder that was
// suggested.
func (l *List) Place(address, folder string) bool {
	i := l.find(address)
	if i < 0 {
		return false
	}
	folder = strings.TrimSpace(folder)
	l.AddFolder(folder)
	l.marks[i].Folder = folder
	return true
}
