package idb

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"
)

// A site's databases are kept in one file of the folder: the mark, the
// site's key, then what each transaction that committed changed, one
// after another as they committed. A change is its length, the
// checksum of its bytes, and those: a list of operations. Reading the
// file is doing them again, up to the first that is not whole. Once a
// file is over twice what its site holds, it is written again as the
// one change that makes what the site holds.

const (
	// Folder is the folder of the files, in the one the store is given.
	Folder = "indexeddb"
	mark   = "lightidb1\n"
	// slack is how much a file may hold beyond twice its site's bytes
	// before it is written again.
	slack = 1 << 20
)

// The operations of a change.
const (
	opDatabase = iota + 1
	opDrop
	opStore
	opDropStore
	opNameStore
	opIndex
	opDropIndex
	opNameIndex
	opPut
	opDelete
	opClear
	opList
)

// enc writes the operations of a change.
type enc struct{ b []byte }

func (e *enc) u(n uint64) { e.b = binary.AppendUvarint(e.b, n) }

func (e *enc) s(s string) {
	e.u(uint64(len(s)))
	e.b = append(e.b, s...)
}

func (e *enc) flag(on bool) {
	if on {
		e.b = append(e.b, 1)
	} else {
		e.b = append(e.b, 0)
	}
}

func (e *enc) op(op byte, db uint32) {
	e.b = append(e.b, op)
	e.u(uint64(db))
}

func (e *enc) keys(keys []Key) {
	e.u(uint64(len(keys)))
	for _, k := range keys {
		e.s(string(k))
	}
}

func (e *enc) record(key Key, value []byte, refs []Ref) {
	e.s(string(key))
	e.u(uint64(len(value)))
	e.b = append(e.b, value...)
	e.u(uint64(len(refs)))
	for _, ref := range refs {
		e.u(uint64(ref.Index))
		e.keys(ref.Keys)
	}
}

func (e *enc) span(r Range) {
	e.s(string(r.Lower))
	e.s(string(r.Upper))
	e.flag(r.LowerOpen)
	e.flag(r.UpperOpen)
}

func (e *enc) store(st *objectStore) {
	e.u(uint64(st.id))
	e.s(st.name)
	e.s(st.keyPath)
	e.flag(st.auto)
	e.u(st.generator)
}

func (e *enc) index(x *index) {
	e.u(uint64(x.id))
	e.s(x.name)
	e.s(x.keyPath)
	e.flag(x.unique)
	e.flag(x.multi)
}

// dec reads the operations of a change: bad once they were not whole.
type dec struct {
	b   []byte
	bad bool
}

func (d *dec) u() uint64 {
	n, size := binary.Uvarint(d.b)
	if size <= 0 {
		d.bad, d.b = true, nil
		return 0
	}
	d.b = d.b[size:]
	return n
}

func (d *dec) id() uint32 { return uint32(d.u()) } //nolint:gosec // a number this package wrote

func (d *dec) bytes() []byte {
	n := d.u()
	if n > uint64(len(d.b)) {
		d.bad, d.b = true, nil
		return nil
	}
	out := d.b[:n]
	d.b = d.b[n:]
	return out
}

func (d *dec) s() string { return string(d.bytes()) }

func (d *dec) flag() bool {
	if len(d.b) == 0 {
		d.bad = true
		return false
	}
	on := d.b[0] != 0
	d.b = d.b[1:]
	return on
}

func (d *dec) keys() []Key {
	n := d.u()
	if n > uint64(len(d.b)) {
		d.bad = true
		return nil
	}
	keys := make([]Key, n)
	for i := range keys {
		keys[i] = Key(d.s())
	}
	return keys
}

func (d *dec) record() *record {
	r := &record{key: Key(d.s()), value: bytes.Clone(d.bytes())}
	n := d.u()
	if n > uint64(len(d.b)) {
		d.bad = true
		return r
	}
	for range n {
		r.refs = append(r.refs, Ref{Index: d.id(), Keys: d.keys()})
	}
	return r
}

// apply does the operations of a change again, and reports whether
// they were whole.
func (s *Site) apply(change []byte) bool {
	d := &dec{b: change}
	for len(d.b) > 0 && !d.bad {
		op := d.b[0]
		d.b = d.b[1:]
		id := d.id()
		if op == opDatabase {
			s.restore(id, d.s(), d.u())
			continue
		}
		db := s.byID[id]
		if db == nil {
			db = &database{stores: map[uint32]*objectStore{}}
		}
		if op == opDrop {
			delete(s.dbs, db.name)
			delete(s.byID, id)
			continue
		}
		if !db.apply(op, d) {
			return false
		}
	}
	return !d.bad
}

// restore is a database of the file at that version.
func (s *Site) restore(id uint32, name string, version uint64) {
	db := s.byID[id]
	if db == nil {
		if old := s.dbs[name]; old != nil {
			delete(s.byID, old.id)
		}
		db = &database{id: id, name: name, stores: map[uint32]*objectStore{}}
		s.dbs[name], s.byID[id] = db, db
		s.lastDB = max(s.lastDB, id)
	}
	db.version = version
}

func (db *database) apply(op byte, d *dec) bool {
	switch op {
	case opStore:
		st := &objectStore{id: d.id(), name: d.s(), keyPath: d.s(), auto: d.flag(), generator: d.u(),
			indexes: map[uint32]*index{}}
		db.stores[st.id] = st
		db.lastID = max(db.lastID, st.id)
		return true
	case opDropStore:
		delete(db.stores, d.id())
		return true
	}
	st := db.stores[d.id()]
	if st == nil {
		st = &objectStore{indexes: map[uint32]*index{}}
	}
	switch op {
	case opNameStore:
		st.name = d.s()
	case opIndex:
		x := &index{id: d.id(), name: d.s(), keyPath: d.s(), unique: d.flag(), multi: d.flag()}
		st.indexes[x.id] = x
		db.lastID = max(db.lastID, x.id)
	case opDropIndex:
		if x := st.indexes[d.id()]; x != nil {
			st.unindex(x)
		}
	case opNameIndex:
		if x := st.indexes[d.id()]; x != nil {
			x.name = d.s()
		} else {
			d.s()
		}
	case opPut:
		r := d.record()
		if i, found := st.find(r.key); found {
			st.remove(i)
		}
		st.insert(r)
		st.count(r.key)
	case opDelete:
		st.cut(Range{Lower: Key(d.s()), Upper: Key(d.s()), LowerOpen: d.flag(), UpperOpen: d.flag()})
	case opClear:
		st.empty()
	case opList:
		ref := Ref{Index: d.id()}
		primary := Key(d.s())
		ref.Keys = d.keys()
		if i, found := st.find(primary); found {
			st.unlist(st.records[i])
			st.records[i].refs = append(st.records[i].refs, ref)
			st.bytes += st.records[i].size()
			for _, each := range st.records[i].refs {
				st.list(st.records[i], each)
			}
		}
	default:
		return false
	}
	return true
}

// whole is the one change that makes what the site holds.
func (s *Site) whole() []byte {
	var e enc
	for _, db := range s.dbs {
		if db.version == 0 {
			continue
		}
		e.op(opDatabase, db.id)
		e.s(db.name)
		e.u(db.version)
		for _, st := range db.stores {
			e.op(opStore, db.id)
			e.store(st)
			for _, x := range st.indexes {
				e.op(opIndex, db.id)
				e.u(uint64(st.id))
				e.index(x)
			}
			for _, r := range st.records {
				e.op(opPut, db.id)
				e.u(uint64(st.id))
				e.record(r.key, r.value, r.refs)
			}
		}
	}
	return e.b
}

func (s *Store) folder() string { return filepath.Join(s.dir, Folder) }

func (s *Site) path() string {
	sum := sha256.Sum256([]byte(s.key))
	return filepath.Join(s.store.folder(), hex.EncodeToString(sum[:16])+".idb")
}

func (s *Site) head() []byte {
	head := binary.AppendUvarint([]byte(mark), uint64(len(s.key)))
	return append(head, s.key...)
}

// framed is a change as the file holds it.
func framed(change []byte) []byte {
	out := make([]byte, 8, 8+len(change))
	binary.BigEndian.PutUint32(out, uint32(len(change))) //nolint:gosec // a change is under 4 GB: a site holds less
	binary.BigEndian.PutUint32(out[4:], crc32.ChecksumIEEE(change))
	return append(out, change...)
}

// load reads the site's file, if the store keeps any: one that is not
// whole is written again as what was read of it.
func (s *Site) load() {
	if s.store.out == nil {
		return
	}
	data, err := os.ReadFile(s.path())
	head := s.head()
	if err != nil || !bytes.HasPrefix(data, head) {
		return
	}
	at := len(head)
	for at+8 <= len(data) {
		size := int(binary.BigEndian.Uint32(data[at:]))
		if at+8+size > len(data) || crc32.ChecksumIEEE(data[at+8:at+8+size]) != binary.BigEndian.Uint32(data[at+4:]) ||
			!s.apply(data[at+8:at+8+size]) {
			break
		}
		at += 8 + size
	}
	if s.logged = int64(at); at != len(data) {
		s.rewrite()
	}
}

// append adds a change to the site's file, or writes the file again
// when it holds much more than the site does and no transaction but
// by has changed anything it may yet undo.
func (s *Site) append(change []byte, by *Tx) {
	out := s.store.out
	if out == nil || len(change) == 0 {
		return
	}
	switch {
	case s.logged == 0:
		data := append(s.head(), framed(change)...)
		s.logged = int64(len(data))
		out.add(job{path: s.path(), data: data, whole: true})
	case s.logged+int64(len(change)) > 2*s.usage()+slack && s.settled(by):
		s.rewrite()
	default:
		data := framed(change)
		s.logged += int64(len(data))
		out.add(job{path: s.path(), data: data})
	}
}

func (s *Site) rewrite() {
	data := append(s.head(), framed(s.whole())...)
	s.logged = int64(len(data))
	s.store.out.add(job{path: s.path(), data: data, whole: true})
}

// settled reports whether what the site holds is what was committed.
func (s *Site) settled(but *Tx) bool {
	for _, db := range s.dbs {
		for _, tx := range db.txs {
			if tx != but && (len(tx.redo.b) > 0 || len(tx.undo) > 0) {
				return false
			}
		}
	}
	return true
}

// job is something to write: bytes at the end of a file, a file whole,
// or a folder to remove.
type job struct {
	path          string
	data          []byte
	whole, remove bool
}

// writer writes the files off the goroutines that commit, in the order
// they committed.
type writer struct {
	mu      sync.Mutex
	changed *sync.Cond
	jobs    []job
	busy    bool
	closed  bool
	err     error
}

func newWriter() *writer {
	w := &writer{}
	w.changed = sync.NewCond(&w.mu)
	go w.run()
	return w
}

func (w *writer) add(j job) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.jobs = append(w.jobs, j)
	w.changed.Broadcast()
}

func (w *writer) run() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for {
		for len(w.jobs) == 0 && !w.closed {
			w.changed.Wait()
		}
		if len(w.jobs) == 0 {
			return
		}
		jobs := w.jobs
		w.jobs, w.busy = nil, true
		w.mu.Unlock()
		err := write(jobs)
		w.mu.Lock()
		w.busy = false
		w.err = errors.Join(w.err, err)
		w.changed.Broadcast()
	}
}

// wait is why what was to be written until now could not be, once it
// was or was not.
func (w *writer) wait() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for len(w.jobs) > 0 || w.busy {
		w.changed.Wait()
	}
	err := w.err
	w.err = nil
	return err
}

func (w *writer) close() error {
	err := w.wait()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	w.changed.Broadcast()
	return err
}

func write(jobs []job) error {
	var failed error
	for i := 0; i < len(jobs); i++ {
		j := jobs[i]
		var err error
		switch {
		case j.remove:
			err = os.RemoveAll(j.path)
		case j.whole:
			err = replace(j.path, j.data)
		default:
			for i+1 < len(jobs) && jobs[i+1].path == j.path && !jobs[i+1].whole && !jobs[i+1].remove {
				i++
				j.data = append(j.data, jobs[i].data...)
			}
			err = extend(j.path, j.data)
		}
		if err != nil {
			failed = errors.Join(failed, fmt.Errorf("saving the sites' databases: %w", err))
		}
	}
	return failed
}

// replace writes a file whole or not at all.
func replace(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	draft := path + ".tmp"
	if err := os.WriteFile(draft, data, 0o600); err != nil {
		return err
	}
	return os.Rename(draft, path)
}

func extend(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
