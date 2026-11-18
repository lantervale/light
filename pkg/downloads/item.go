// Package downloads saves answers of the web to a folder: each download
// streamed to a temporary file and renamed at its end under a free name,
// counted as it comes, cancelled, retried, listed, and the list kept in a
// file. It is plain Go: whoever shows it reads Items.
package downloads

import (
	"time"
)

// State is where a download stands.
type State string

const (
	Active    State = "active"
	Done      State = "done"
	Cancelled State = "cancelled"
	Failed    State = "failed"
)

// Item is one download.
type Item struct {
	ID int `json:"id"`
	// Address is the address asked for.
	Address string `json:"address"`
	// Name is the file's name, Path where it is once Done.
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
	// Media is the answer's content type.
	Media string `json:"media,omitempty"`
	// Received is how many bytes came, Total how many the server said
	// would, -1 when it did not say.
	Received int64 `json:"received"`
	Total    int64 `json:"total"`
	State    State `json:"state"`
	// Err is why a Failed download failed.
	Err     string    `json:"err,omitempty"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitzero"`
	// Speed is the bytes a second an Active download last came at.
	Speed float64 `json:"-"`
	// suggested is the name whoever started it asked for.
	suggested string
}

// Fraction is the part of an item received, from 0 to 1, false when its
// size is not known.
func (i Item) Fraction() (float64, bool) {
	if i.Total <= 0 {
		return 0, false
	}
	return min(1, float64(i.Received)/float64(i.Total)), true
}

// Remaining is how long an Active item still takes at its speed, false
// when its size or its speed is not known.
func (i Item) Remaining() (time.Duration, bool) {
	if i.State != Active || i.Total <= 0 || i.Speed <= 0 {
		return 0, false
	}
	left := float64(i.Total-i.Received) / i.Speed
	return time.Duration(left * float64(time.Second)), true
}

// Missing reports whether a Done item's file is no longer where it was
// saved.
func (i Item) Missing(exists func(path string) bool) bool {
	return i.State == Done && !exists(i.Path)
}
