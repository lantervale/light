package idb

import (
	"slices"
	"sort"
)

// record is one value of an object store, under its key, and the keys
// it is listed under in the store's indexes.
type record struct {
	key   Key
	value []byte
	refs  []Ref
}

// Ref is the keys one record is listed under in one index.
type Ref struct {
	Index uint32
	Keys  []Key
}

func (r *record) size() int64 {
	n := len(r.key) + len(r.value)
	for _, ref := range r.refs {
		for _, k := range ref.Keys {
			n += len(k)
		}
	}
	return int64(n)
}

// objectStore is the records of one store, in the order of their keys.
type objectStore struct {
	id            uint32
	name, keyPath string
	auto          bool
	// generator is the next key the store makes.
	generator uint64
	records   []*record
	indexes   map[uint32]*index
	bytes     int64
}

// index lists the records of a store under other keys: its entries in
// the order of those keys, then of the records' own.
type index struct {
	id            uint32
	name, keyPath string
	unique, multi bool
	entries       []entry
}

type entry struct {
	key Key
	rec *record
}

// maxGenerated is the last key a store makes.
const maxGenerated = 1 << 53

func (s *objectStore) find(key Key) (int, bool) {
	i := sort.Search(len(s.records), func(i int) bool { return s.records[i].key >= key })
	return i, i < len(s.records) && s.records[i].key == key
}

func (x *index) find(key, primary Key) int {
	return sort.Search(len(x.entries), func(i int) bool {
		e := x.entries[i]
		return e.key > key || (e.key == key && e.rec.key >= primary)
	})
}

// taken reports whether a record other than one of that key is listed
// under key.
func (x *index) taken(key, primary Key) bool {
	i := x.find(key, "")
	return i < len(x.entries) && x.entries[i].key == key &&
		(x.entries[i].rec.key != primary || (i+1 < len(x.entries) && x.entries[i+1].key == key))
}

// clash is the index a record of those refs cannot be listed in, its
// key there being another record's.
func (s *objectStore) clash(primary Key, refs []Ref) *index {
	for _, ref := range refs {
		x := s.indexes[ref.Index]
		if x == nil || !x.unique {
			continue
		}
		for _, k := range ref.Keys {
			if x.taken(k, primary) {
				return x
			}
		}
	}
	return nil
}

// insert adds a record, which the store has none of that key for.
func (s *objectStore) insert(r *record) {
	i, _ := s.find(r.key)
	s.records = slices.Insert(s.records, i, r)
	s.bytes += r.size()
	for _, ref := range r.refs {
		s.list(r, ref)
	}
}

// list lists a record in an index under the keys of ref.
func (s *objectStore) list(r *record, ref Ref) {
	x := s.indexes[ref.Index]
	if x == nil {
		return
	}
	for _, k := range ref.Keys {
		x.entries = slices.Insert(x.entries, x.find(k, r.key), entry{k, r})
	}
}

// remove takes the record at i out of the store and of its indexes.
func (s *objectStore) remove(i int) {
	r := s.records[i]
	s.records = slices.Delete(s.records, i, i+1)
	s.unlist(r)
}

func (s *objectStore) unlist(r *record) {
	s.bytes -= r.size()
	for _, ref := range r.refs {
		x := s.indexes[ref.Index]
		if x == nil {
			continue
		}
		for _, k := range ref.Keys {
			if i := x.find(k, r.key); i < len(x.entries) && x.entries[i].rec == r {
				x.entries = slices.Delete(x.entries, i, i+1)
			}
		}
	}
}

// source is what a request reads: a store's records, or an index's
// entries.
type source struct {
	store *objectStore
	index *index
}

func (s source) len() int {
	if s.index != nil {
		return len(s.index.entries)
	}
	return len(s.store.records)
}

func (s source) at(i int) Found {
	if s.index != nil {
		e := s.index.entries[i]
		return Found{Key: e.key, Primary: e.rec.key, Value: e.rec.value, OK: true}
	}
	r := s.store.records[i]
	return Found{Key: r.key, Primary: r.key, Value: r.value, OK: true}
}

// seek is the first place whose key is not before key, or is after it
// when past: the key alone, or with primary the key and then the
// record's own.
func (s source) seek(key, primary Key, past bool) int {
	return sort.Search(s.len(), func(i int) bool {
		var k, p Key
		if s.index != nil {
			k, p = s.index.entries[i].key, s.index.entries[i].rec.key
		} else {
			k = s.store.records[i].key
			p = k
		}
		switch {
		case k != key:
			return k > key
		case primary == "":
			return !past
		case past:
			return p > primary
		}
		return p >= primary
	})
}

// bounds are the places of a range: from the first, up to the second.
func (s source) bounds(r Range) (int, int) {
	from, to := 0, s.len()
	if r.Lower != "" {
		from = s.seek(r.Lower, "", r.LowerOpen)
	}
	if r.Upper != "" {
		to = s.seek(r.Upper, "", !r.UpperOpen)
	}
	return from, max(from, to)
}

// Direction is the way a cursor goes.
type Direction int

const (
	Next Direction = iota
	NextUnique
	Prev
	PrevUnique
)

// Cursor is one move of a cursor: from Position (and, in an index,
// Primary, the key of the record it is on), "" before its first, by
// Count records the way of Direction, or to Key (and ToPrimary) when
// they are given.
type Cursor struct {
	Range             Range
	Direction         Direction
	Position, Primary Key
	Key, ToPrimary    Key
	Count             int
}

// Found is a record a request found: its key in what was read, the
// record's own, and its value.
type Found struct {
	Key, Primary Key
	Value        []byte
	OK           bool
}

// step is the record a cursor's move ends on.
func (s source) step(c Cursor) Found {
	from, to := s.bounds(c.Range)
	count := max(c.Count, 1)
	var at int
	switch c.Direction {
	case Next:
		if c.Key != "" {
			from = max(from, s.seek(c.Key, c.ToPrimary, false))
		}
		if c.Position != "" {
			from = max(from, s.seek(c.Position, s.own(c.Primary), true))
		}
		at = from + count - 1
	case NextUnique:
		if c.Key != "" {
			from = max(from, s.seek(c.Key, "", false))
		}
		if c.Position != "" {
			from = max(from, s.seek(c.Position, "", true))
		}
		for ; count > 1 && from < to; count-- {
			from = s.seek(s.at(from).Key, "", true)
		}
		at = from
	case Prev:
		if c.Key != "" {
			to = min(to, s.seek(c.Key, c.ToPrimary, true))
		}
		if c.Position != "" {
			to = min(to, s.seek(c.Position, s.own(c.Primary), false))
		}
		at = to - count
	case PrevUnique:
		if c.Key != "" {
			to = min(to, s.seek(c.Key, "", true))
		}
		if c.Position != "" {
			to = min(to, s.seek(c.Position, "", false))
		}
		for ; count > 0 && to > from; count-- {
			to = s.seek(s.at(to-1).Key, "", false)
		}
		if count > 0 {
			return Found{}
		}
		return s.at(to)
	}
	if at < from || at >= to {
		return Found{}
	}
	return s.at(at)
}

// own is the key of the record a cursor of an index is on: a store's
// cursor is placed by its key alone.
func (s source) own(primary Key) Key {
	if s.index == nil {
		return ""
	}
	return primary
}
