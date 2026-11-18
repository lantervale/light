package idb

import (
	"math"
	"slices"
)

// Mode is what a transaction may do.
type Mode int

const (
	ReadOnly Mode = iota
	ReadWrite
	// ChangeVersion is the transaction of an upgrade, which alone
	// changes what stores and indexes a database has.
	ChangeVersion
)

// Tx is a transaction: what it changed is kept when it commits and
// undone when it aborts, and no other transaction reads its stores
// while it writes them.
type Tx struct {
	id     uint64
	client *Client
	conn   *conn
	db     *database
	mode   Mode
	// scope are the stores it may use, all of them when nil.
	scope         map[uint32]bool
	started, done bool
	// undo takes back what it changed, the last change first, and redo
	// is what it changed as the site's file says it.
	undo []func()
	redo enc
	// request is the request an upgrade answers.
	request *request
}

// Begin starts a transaction on those stores of a connection: its
// number, and whether it has them already. One that waits for other
// transactions to finish is told once it has (Started).
func (c *Client) Begin(connection uint64, stores []string, mode Mode) (uint64, bool, error) {
	s := c.site
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	cn := c.conns[connection]
	if cn == nil || cn.closing {
		return 0, false, failed("InvalidStateError", "The database connection is closing.")
	}
	tx := &Tx{id: s.store.next(), client: c, conn: cn, db: cn.db, mode: mode, scope: map[uint32]bool{}}
	for _, name := range stores {
		st := cn.db.named(name)
		if st == nil {
			return 0, false, failed("NotFoundError", "One of the specified object stores was not found.")
		}
		tx.scope[st.id] = true
	}
	tx.started = cn.db.free(tx)
	cn.db.txs = append(cn.db.txs, tx)
	c.txs[tx.id] = tx
	return tx.id, tx.started, nil
}

// Tx is the transaction of that number, nil once it has finished.
func (c *Client) Tx(id uint64) *Tx {
	c.site.store.mu.Lock()
	defer c.site.store.mu.Unlock()
	return c.txs[id]
}

// free reports whether no transaction before tx holds a store it
// needs: two that only read share theirs.
func (db *database) free(tx *Tx) bool {
	for _, other := range db.txs {
		if other == tx {
			break
		}
		if other.mode == ReadOnly && tx.mode == ReadOnly {
			continue
		}
		if other.scope == nil || tx.scope == nil {
			return false
		}
		for id := range tx.scope {
			if other.scope[id] {
				return false
			}
		}
	}
	return true
}

func (t *Tx) lock() func() {
	t.client.site.store.mu.Lock()
	return t.client.site.store.mu.Unlock
}

// Commit keeps what the transaction changed.
func (t *Tx) Commit() {
	defer t.lock()()
	if t.done {
		return
	}
	t.client.site.append(t.redo.b, t)
	t.finish(true)
}

// Abort undoes what the transaction changed.
func (t *Tx) Abort() {
	defer t.lock()()
	t.abort()
}

func (t *Tx) abort() {
	if t.done {
		return
	}
	for i := len(t.undo) - 1; i >= 0; i-- {
		t.undo[i]()
	}
	t.finish(false)
}

// finish lets go of the transaction's stores, which those that waited
// for them are given.
func (t *Tx) finish(kept bool) {
	s := t.client.site
	t.done, t.undo, t.redo = true, nil, enc{}
	t.db.txs = slices.DeleteFunc(t.db.txs, func(o *Tx) bool { return o == t })
	delete(t.client.txs, t.id)
	for _, other := range t.db.txs {
		if !other.started && t.db.free(other) {
			other.started = true
			other.client.notify(Event{Kind: Started, Tx: other.id})
		}
	}
	if t.request != nil {
		s.upgraded(t, kept)
	}
	if t.conn.closing && !t.conn.busy() {
		t.conn.close()
	}
	s.advance(t.db.name)
}

// in is the store a request of the transaction is for.
func (t *Tx) in(store uint32, write bool) (*objectStore, error) {
	st := t.db.stores[store]
	switch {
	case t.done || !t.started:
		return nil, failed("InvalidStateError", "The transaction has finished.")
	case st == nil || (t.scope != nil && !t.scope[store]):
		return nil, failed("NotFoundError", "The object store is not one of the transaction's.")
	case write && t.mode == ReadOnly:
		return nil, failed("ReadOnlyError", "The transaction is read-only.")
	}
	return st, nil
}

// from is what a request reads: the store, or the index of it of that
// number when not 0.
func (t *Tx) from(store, index uint32) (source, error) {
	st, err := t.in(store, false)
	if err != nil {
		return source{}, err
	}
	if index == 0 {
		return source{store: st}, nil
	}
	x := st.indexes[index]
	if x == nil {
		return source{}, failed("NotFoundError", "The index was deleted.")
	}
	return source{store: st, index: x}, nil
}

// schema is the transaction of an upgrade, which changes stores and
// indexes.
func (t *Tx) schema() error {
	switch {
	case t.done:
		return failed("InvalidStateError", "The transaction has finished.")
	case t.mode != ChangeVersion:
		return failed("InvalidStateError", "The database is not running a version change transaction.")
	}
	return nil
}

var errRange = failed("DataError", "The parameter is not a valid key.")

// Get is the first record of the range.
func (t *Tx) Get(store, index uint32, r Range) (Found, error) {
	defer t.lock()()
	src, err := t.from(store, index)
	if err != nil || !r.valid() {
		return Found{}, either(err, errRange)
	}
	if from, to := src.bounds(r); from < to {
		return src.at(from), nil
	}
	return Found{}, nil
}

// All are the records of the range, limit of them at most when it is
// not 0.
func (t *Tx) All(store, index uint32, r Range, limit int) ([]Found, error) {
	defer t.lock()()
	src, err := t.from(store, index)
	if err != nil || !r.valid() {
		return nil, either(err, errRange)
	}
	from, to := src.bounds(r)
	if limit > 0 {
		to = min(to, from+limit)
	}
	all := make([]Found, 0, to-from)
	for i := from; i < to; i++ {
		all = append(all, src.at(i))
	}
	return all, nil
}

// Count is how many records the range has.
func (t *Tx) Count(store, index uint32, r Range) (int, error) {
	defer t.lock()()
	src, err := t.from(store, index)
	if err != nil || !r.valid() {
		return 0, either(err, errRange)
	}
	from, to := src.bounds(r)
	return to - from, nil
}

// Step is the record a move of a cursor ends on.
func (t *Tx) Step(store, index uint32, c Cursor) (Found, error) {
	defer t.lock()()
	src, err := t.from(store, index)
	if err != nil || !c.Range.valid() {
		return Found{}, either(err, errRange)
	}
	return src.step(c), nil
}

func either(err error, otherwise *Error) error {
	if err != nil {
		return err
	}
	return otherwise
}

// NextKey is the key the store would make for its next record.
func (t *Tx) NextKey(store uint32) (float64, error) {
	defer t.lock()()
	st, err := t.in(store, true)
	if err != nil {
		return 0, err
	}
	if st.generator > maxGenerated {
		return 0, failed("ConstraintError", "The key generator of the object store has no key left.")
	}
	return float64(st.generator), nil
}

// Put keeps value under key in a store, listed in its indexes under
// refs: in place of the record of that key, unless add, which fails
// when there is one. A store that makes its keys makes one for no key.
// It gives the key of the record.
func (t *Tx) Put(store uint32, key Key, value []byte, refs []Ref, add bool) (Key, error) {
	defer t.lock()()
	st, err := t.in(store, true)
	if err != nil {
		return "", err
	}
	generator := st.generator
	if key, err = st.keyed(key); err != nil {
		return "", err
	}
	if err = t.put(st, &record{key: key, value: value, refs: refs}, add); err != nil {
		st.generator = generator
		return "", err
	}
	t.undo = append(t.undo, func() { st.generator = generator })
	t.redo.op(opPut, t.db.id)
	t.redo.u(uint64(st.id))
	t.redo.record(key, value, refs)
	return key, nil
}

// keyed is the key a record is kept under: the one given, which a
// store that makes keys counts from, or the next it makes.
func (s *objectStore) keyed(key Key) (Key, error) {
	switch {
	case key == "" && !s.auto:
		return "", failed("DataError", "The object store makes no key and none was given.")
	case key == "" && s.generator > maxGenerated:
		return "", failed("ConstraintError", "The key generator of the object store has no key left.")
	case key == "":
		s.generator++
		return Number(float64(s.generator - 1)), nil
	case !key.Valid():
		return "", errRange
	}
	s.count(key)
	return key, nil
}

// count has the store make its next keys after key, when it is a
// number.
func (s *objectStore) count(key Key) {
	if n, is := key.number(); is && s.auto && n >= float64(s.generator) {
		s.generator = uint64(math.Min(n, maxGenerated)) + 1
	}
}

func (t *Tx) put(st *objectStore, r *record, add bool) error {
	for _, ref := range r.refs {
		for _, k := range ref.Keys {
			if !k.Valid() {
				return errRange
			}
		}
	}
	var old *record
	i, found := st.find(r.key)
	switch {
	case found && add:
		return failed("ConstraintError", "Key already exists in the object store.")
	case found:
		old = st.records[i]
	}
	site := t.client.site
	grown := r.size()
	if old != nil {
		grown -= old.size()
	}
	if grown > 0 && site.usage()+grown > site.store.quota {
		return failed("QuotaExceededError", "The site's databases are full.")
	}
	if x := st.clash(r.key, r.refs); x != nil {
		return failed("ConstraintError",
			"Unable to add key to index '"+x.name+"': at least one key does not satisfy the uniqueness requirements.")
	}
	if old != nil {
		st.remove(i)
	}
	st.insert(r)
	t.undo = append(t.undo, func() {
		if at, is := st.find(r.key); is && st.records[at] == r {
			st.remove(at)
		}
		if old != nil {
			st.insert(old)
		}
	})
	return nil
}

// Delete removes the records of a range from a store.
func (t *Tx) Delete(store uint32, r Range) error {
	defer t.lock()()
	st, err := t.in(store, true)
	if err != nil || !r.valid() {
		return either(err, errRange)
	}
	gone := st.cut(r)
	t.undo = append(t.undo, func() {
		for _, r := range gone {
			st.insert(r)
		}
	})
	t.redo.op(opDelete, t.db.id)
	t.redo.u(uint64(st.id))
	t.redo.span(r)
	return nil
}

// cut takes the records of a range out of the store.
func (s *objectStore) cut(r Range) []*record {
	from, to := source{store: s}.bounds(r)
	gone := slices.Clone(s.records[from:to])
	s.records = slices.Delete(s.records, from, to)
	for _, r := range gone {
		s.unlist(r)
	}
	return gone
}

// Clear removes every record of a store.
func (t *Tx) Clear(store uint32) error {
	defer t.lock()()
	st, err := t.in(store, true)
	if err != nil {
		return err
	}
	undo := st.empty()
	t.undo = append(t.undo, undo)
	t.redo.op(opClear, t.db.id)
	t.redo.u(uint64(st.id))
	return nil
}

// empty removes every record of the store, and gives what puts them
// back.
func (s *objectStore) empty() func() {
	records, bytes := s.records, s.bytes
	entries := map[*index][]entry{}
	for _, x := range s.indexes {
		entries[x], x.entries = x.entries, nil
	}
	s.records, s.bytes = nil, 0
	return func() {
		s.records, s.bytes = records, bytes
		for x, list := range entries {
			x.entries = list
		}
	}
}

// CreateStore adds an object store to the database: its number.
func (t *Tx) CreateStore(name, keyPath string, auto bool) (uint32, error) {
	defer t.lock()()
	if err := t.schema(); err != nil {
		return 0, err
	}
	if t.db.named(name) != nil {
		return 0, failed("ConstraintError", "An object store with the specified name already exists.")
	}
	t.db.lastID++
	st := &objectStore{id: t.db.lastID, name: name, keyPath: keyPath, auto: auto, generator: 1,
		indexes: map[uint32]*index{}}
	t.db.stores[st.id] = st
	t.undo = append(t.undo, func() { delete(t.db.stores, st.id) })
	t.redo.op(opStore, t.db.id)
	t.redo.store(st)
	return st.id, nil
}

// DeleteStore removes an object store, its records and its indexes.
func (t *Tx) DeleteStore(store uint32) error {
	defer t.lock()()
	st := t.db.stores[store]
	if err := t.schema(); err != nil || st == nil {
		return either(err, failed("NotFoundError", "The specified object store was not found."))
	}
	delete(t.db.stores, store)
	t.undo = append(t.undo, func() { t.db.stores[store] = st })
	t.redo.op(opDropStore, t.db.id)
	t.redo.u(uint64(store))
	return nil
}

// RenameStore gives an object store another name.
func (t *Tx) RenameStore(store uint32, name string) error {
	defer t.lock()()
	st := t.db.stores[store]
	switch err := t.schema(); {
	case err != nil || st == nil:
		return either(err, failed("NotFoundError", "The specified object store was not found."))
	case t.db.named(name) != nil:
		return failed("ConstraintError", "An object store with the specified name already exists.")
	}
	old := st.name
	st.name = name
	t.undo = append(t.undo, func() { st.name = old })
	t.redo.op(opNameStore, t.db.id)
	t.redo.u(uint64(store))
	t.redo.s(name)
	return nil
}

func (s *objectStore) indexed(name string) *index {
	for _, x := range s.indexes {
		if x.name == name {
			return x
		}
	}
	return nil
}

// CreateIndex adds an index to an object store, which lists no record
// yet (List): its number.
func (t *Tx) CreateIndex(store uint32, name, keyPath string, unique, multi bool) (uint32, error) {
	defer t.lock()()
	st := t.db.stores[store]
	switch err := t.schema(); {
	case err != nil || st == nil:
		return 0, either(err, failed("NotFoundError", "The specified object store was not found."))
	case st.indexed(name) != nil:
		return 0, failed("ConstraintError", "An index with the specified name already exists.")
	}
	t.db.lastID++
	x := &index{id: t.db.lastID, name: name, keyPath: keyPath, unique: unique, multi: multi}
	st.indexes[x.id] = x
	t.undo = append(t.undo, func() { delete(st.indexes, x.id) })
	t.redo.op(opIndex, t.db.id)
	t.redo.u(uint64(store))
	t.redo.index(x)
	return x.id, nil
}

// List lists the record of a store of the key primary in an index it
// was not listed in, under keys.
func (t *Tx) List(store, index uint32, primary Key, keys []Key) error {
	defer t.lock()()
	st := t.db.stores[store]
	x := st.at(index)
	if err := t.schema(); err != nil || x == nil {
		return either(err, failed("NotFoundError", "The specified index was not found."))
	}
	ref := Ref{Index: index, Keys: keys}
	i, found := st.find(primary)
	switch {
	case !found:
		return nil
	case slices.ContainsFunc(keys, func(k Key) bool { return !k.Valid() }):
		return errRange
	case st.clash(primary, []Ref{ref}) != nil:
		return failed("ConstraintError",
			"Unable to add key to index '"+x.name+"': at least one key does not satisfy the uniqueness requirements.")
	}
	r := st.records[i]
	before := r.size()
	r.refs = append(r.refs, ref)
	st.bytes += r.size() - before
	st.list(r, ref)
	t.undo = append(t.undo, func() {
		st.unlist(r)
		r.refs = slices.DeleteFunc(slices.Clone(r.refs), func(ref Ref) bool { return ref.Index == index })
		st.bytes += r.size()
		for _, ref := range r.refs {
			st.list(r, ref)
		}
	})
	t.redo.op(opList, t.db.id)
	t.redo.u(uint64(store))
	t.redo.u(uint64(index))
	t.redo.s(string(primary))
	t.redo.keys(keys)
	return nil
}

// at is the index of that number, nil for none.
func (s *objectStore) at(id uint32) *index {
	if s == nil {
		return nil
	}
	return s.indexes[id]
}

// DeleteIndex removes an index from an object store.
func (t *Tx) DeleteIndex(store, index uint32) error {
	defer t.lock()()
	st := t.db.stores[store]
	x := st.at(index)
	if err := t.schema(); err != nil || x == nil {
		return either(err, failed("NotFoundError", "The specified index was not found."))
	}
	undo := st.unindex(x)
	t.undo = append(t.undo, undo)
	t.redo.op(opDropIndex, t.db.id)
	t.redo.u(uint64(store))
	t.redo.u(uint64(index))
	return nil
}

// unindex removes an index and what its records are listed under in
// it, and gives what puts them back.
func (s *objectStore) unindex(x *index) func() {
	delete(s.indexes, x.id)
	type was struct {
		r    *record
		refs []Ref
	}
	var all []was
	for _, r := range s.records {
		if !slices.ContainsFunc(r.refs, func(ref Ref) bool { return ref.Index == x.id }) {
			continue
		}
		all = append(all, was{r, r.refs})
		before := r.size()
		r.refs = slices.DeleteFunc(slices.Clone(r.refs), func(ref Ref) bool { return ref.Index == x.id })
		s.bytes += r.size() - before
	}
	return func() {
		s.indexes[x.id] = x
		for _, w := range all {
			before := w.r.size()
			w.r.refs = w.refs
			s.bytes += w.r.size() - before
		}
	}
}

// RenameIndex gives an index another name.
func (t *Tx) RenameIndex(store, index uint32, name string) error {
	defer t.lock()()
	st := t.db.stores[store]
	x := st.at(index)
	switch err := t.schema(); {
	case err != nil || x == nil:
		return either(err, failed("NotFoundError", "The specified index was not found."))
	case st.indexed(name) != nil:
		return failed("ConstraintError", "An index with the specified name already exists.")
	}
	old := x.name
	x.name = name
	t.undo = append(t.undo, func() { x.name = old })
	t.redo.op(opNameIndex, t.db.id)
	t.redo.u(uint64(store))
	t.redo.u(uint64(index))
	t.redo.s(name)
	return nil
}
