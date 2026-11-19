package recall

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"
)

// Magic is what the file begins with, and version the version of its
// form: after them, in little-endian, the length of the vectors and the
// number of pages, then each page as its address, its title, when it
// was visited in nanoseconds, the number of its passages, and each
// passage as its text and its vector. A text is its length in bytes,
// then its bytes.
const (
	Magic   = "light recall\n"
	version = 1
)

var errBroken = errors.New("the file of what pages said is not one light wrote, or not whole")

func (s *Store) Encode() []byte {
	data := make([]byte, 0, len(Magic)+12+s.size)
	data = append(data, Magic...)
	data = binary.LittleEndian.AppendUint32(data, version)
	data = binary.LittleEndian.AppendUint32(data, uint32(s.dimensions)) //nolint:gosec // at most maxDimensions
	data = binary.LittleEndian.AppendUint32(data, uint32(len(s.pages))) //nolint:gosec // bounded by the cap
	text := func(value string) {
		data = binary.LittleEndian.AppendUint32(data, uint32(len(value))) //nolint:gosec // bounded by the cap
		data = append(data, value...)
	}
	for _, p := range s.pages {
		text(p.address)
		text(p.title)
		data = binary.LittleEndian.AppendUint64(data, uint64(p.at.UnixNano()))
		data = binary.LittleEndian.AppendUint32(data, uint32(len(p.passages))) //nolint:gosec // at most PassagesPerPage
		for _, said := range p.passages {
			text(said.text)
			for _, v := range said.vector {
				data = binary.LittleEndian.AppendUint32(data, math.Float32bits(v))
			}
		}
	}
	return data
}

// reader reads a file from its start, and notes the first thing that
// was not there.
type reader struct {
	data   []byte
	broken bool
}

func (r *reader) take(n int) []byte {
	if r.broken || n < 0 || n > len(r.data) {
		r.broken = true
		return nil
	}
	head := r.data[:n]
	r.data = r.data[n:]
	return head
}

func (r *reader) number() int {
	if b := r.take(4); b != nil {
		return int(binary.LittleEndian.Uint32(b))
	}
	return 0
}

func (r *reader) text() string {
	b := r.take(r.number())
	if !utf8.Valid(b) {
		r.broken = true
	}
	return string(b)
}

// Decode reads what Encode wrote, keeping what fits limit bytes, the
// oldest pages gone first. Data that is not such a file, whole, gives
// an empty store, ready to use, and an error that says nothing of what
// it held.
func Decode(data []byte, limit int) (*Store, error) {
	r := &reader{data: data}
	if string(r.take(len(Magic))) != Magic {
		return New(limit), errBroken
	}
	if v := r.number(); v != version {
		if r.broken {
			return New(limit), errBroken
		}
		return New(limit), fmt.Errorf("the file of what pages said is of a version %d this light does not read", v)
	}
	s := New(limit)
	dimensions, pages := r.number(), r.number()
	if dimensions > maxDimensions || pages > 0 && dimensions == 0 {
		return New(limit), errBroken
	}
	for range pages {
		p := page{address: r.text(), title: r.text()}
		if at := r.take(8); at != nil {
			p.at = time.Unix(0, int64(binary.LittleEndian.Uint64(at))) //nolint:gosec // written from an int64
		}
		passages := r.number()
		if p.address == "" || passages == 0 || passages > PassagesPerPage {
			r.broken = true
		}
		for range passages {
			said := passage{text: r.text()}
			raw := r.take(4 * dimensions)
			if raw == nil || len(said.text) > PassageSize {
				r.broken = true
				break
			}
			said.vector = make([]float32, dimensions)
			for i := range said.vector {
				v := math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					r.broken = true
				}
				said.vector[i] = v
			}
			p.passages = append(p.passages, said)
		}
		if r.broken {
			return New(limit), errBroken
		}
		s.pages = append(s.pages, p)
		s.size += p.size()
	}
	if r.broken || len(r.data) != 0 {
		return New(limit), errBroken
	}
	if s.dimensions = dimensions; len(s.pages) == 0 {
		s.dimensions = 0
	}
	s.trim()
	return s, nil
}

// Load reads the store saved at path, keeping what fits limit bytes. A
// file that is not there is an empty store; one that cannot be read is
// an empty store too, and the reason.
func Load(path string, limit int) (*Store, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return New(limit), nil
	case err != nil:
		return New(limit), fmt.Errorf("reading what pages said: %w", err)
	}
	return Decode(data, limit)
}

// Save writes s at path for its user alone, making its folder, the file
// whole or not at all. A store that holds nothing has no file: the one
// there is removed.
func Save(path string, s *Store) error {
	if s.Len() == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("forgetting what pages said: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("saving what pages said: %w", err)
	}
	draft := path + ".tmp"
	if err := os.WriteFile(draft, s.Encode(), 0o600); err != nil {
		return fmt.Errorf("saving what pages said: %w", err)
	}
	if err := os.Rename(draft, path); err != nil {
		return fmt.Errorf("saving what pages said: %w", err)
	}
	return nil
}
