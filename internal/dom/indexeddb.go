package dom

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/lantervale/light/pkg/idb"
)

// IndexedDB is the page's side of its site's databases (pkg/idb): the
// script asks for an operation by its name, with the numbers light
// knows a connection, a transaction, a store and an index by, and
// what holds bytes (keys, values) in the buffer it shares, where the
// answer is left. What a request comes to later (a database opened, a
// connection asked to close) lands for the next frame, whose tasks
// light then runs one at a time.

// bases is a page's use of its site's databases.
type bases struct {
	client *idb.Client
	mu     sync.Mutex
	// told is what the databases told the page and it has not heard,
	// awake whether their tasks are left to a frame, and answer an
	// answer the script's buffer was too short for.
	told   []idb.Event
	awake  bool
	answer []byte
}

// nameKey is the key of a name in what the script is told.
const nameKey = "name"

var errWire = errors.New("DataError: what was asked of the databases is not whole")

// based is the page as a document of its site's databases: those the
// browser keeps, or some kept as long as the page.
func (r *Runtime) based() *idb.Client {
	if r.bases.client == nil {
		var site *idb.Site
		if r.host.Bases != nil {
			site = r.host.Bases()
		}
		if site == nil {
			site = idb.New("").Site("")
		}
		r.bases.client = site.Client(r.informed)
	}
	return r.bases.client
}

// informed takes note of what the databases tell the page, from any
// goroutine.
func (r *Runtime) informed(e idb.Event) {
	r.bases.mu.Lock()
	defer r.bases.mu.Unlock()
	r.bases.told = append(r.bases.told, e)
	r.wake()
}

// wake leaves the tasks of the databases to the next frame.
func (r *Runtime) wake() {
	if !r.bases.awake {
		r.bases.awake = true
		r.land(r.pump)
	}
}

// pump tells the page what the databases told it and runs their tasks,
// each entered apart so the microtasks of one run before the next: as
// long as a frame's timers may, the rest left to the next frame.
func (r *Runtime) pump() {
	for end := time.Now().Add(frameBudget); !r.closed; {
		for _, e := range r.news() {
			r.enter("idb", "told", eventOf(e))
		}
		if more, _ := r.enter("idb", "run").(bool); !more && r.idle() {
			return
		}
		if time.Now().After(end) {
			r.land(r.pump)
			return
		}
	}
}

func (r *Runtime) news() []idb.Event {
	r.bases.mu.Lock()
	defer r.bases.mu.Unlock()
	told := r.bases.told
	r.bases.told = nil
	return told
}

// idle reports whether nothing is left to tell the page, which is then
// woken by what comes next.
func (r *Runtime) idle() bool {
	r.bases.mu.Lock()
	defer r.bases.mu.Unlock()
	if len(r.bases.told) > 0 {
		return false
	}
	r.bases.awake = false
	return true
}

// unbased ends the page's use of the databases: what it had open is
// closed, and what it had not committed undone.
func (r *Runtime) unbased() {
	if r.bases.client != nil {
		r.bases.client.Close()
	}
}

func eventOf(e idb.Event) map[string]any {
	kinds := map[idb.Kind]string{idb.Opened: "opened", idb.Upgrade: "upgrade", idb.Failed: "failed",
		idb.Deleted: "deleted", idb.Blocked: "blocked", idb.VersionChange: "versionchange", idb.Started: "started",
		idb.Closed: "closed"}
	out := map[string]any{"kind": kinds[e.Kind], "request": float64(e.Request), "conn": float64(e.Conn),
		"tx": float64(e.Tx), "old": float64(e.Old), "new": float64(e.New)}
	if e.Err != nil {
		out[nameKey], out["message"] = e.Err.Name, e.Err.Message
	}
	if e.Kind == idb.Opened || e.Kind == idb.Upgrade {
		stores := []any{}
		for _, st := range e.Database.Stores {
			indexes := []any{}
			for _, x := range st.Indexes {
				indexes = append(indexes, map[string]any{"id": x.ID, nameKey: x.Name, "keyPath": x.KeyPath,
					"unique": x.Unique, "multi": x.Multi})
			}
			stores = append(stores, map[string]any{"id": st.ID, nameKey: st.Name, "keyPath": st.KeyPath, "auto": st.Auto,
				"indexes": indexes})
		}
		out["database"] = map[string]any{nameKey: e.Database.Name, "version": float64(e.Database.Version),
			"stores": stores}
	}
	return out
}

// idb answers what a script asks of the databases.
func (r *Runtime) idb(a arguments) (any, error) {
	if r.host.Sandboxed.Opaque {
		//nolint:revive,staticcheck // a DOMException's message, as Chrome words it
		return nil, errors.New("SecurityError: The document is sandboxed and lacks the 'allow-same-origin' flag.")
	}
	c := r.based()
	switch a.text(0) {
	case "wake":
		r.bases.mu.Lock()
		defer r.bases.mu.Unlock()
		r.wake()
	case "open":
		return float64(c.Open(a.text(1), uint64(a.number(2)))), nil
	case "delete":
		return float64(c.Delete(a.text(1))), nil
	case "databases":
		all := []any{}
		for _, db := range c.Databases() {
			all = append(all, map[string]any{nameKey: db.Name, "version": float64(db.Version)})
		}
		return all, nil
	case "heard":
		c.Heard(uint64(a.number(1)))
	case "shut":
		c.Shut(uint64(a.number(1)))
	case "usage":
		return []any{float64(c.Site().Usage()), float64(c.Site().Quota()), c.Site().Kept()}, nil
	case "begin":
		names, _ := a[2].([]any)
		stores := make([]string, len(names))
		for i, name := range names {
			stores[i], _ = name.(string)
		}
		tx, started, err := c.Begin(uint64(a.number(1)), stores, idb.Mode(int(a.number(3))&1))
		return []any{float64(tx), started}, err
	case "answer":
		into, _ := a[5].([]byte)
		copy(into, r.bases.answer)
		r.bases.answer = nil
	default:
		return r.transact(c, a)
	}
	return nothing()
}

// transact answers what a script asks of a transaction.
func (r *Runtime) transact(c *idb.Client, a arguments) (any, error) {
	tx := c.Tx(uint64(a.number(1)))
	op := a.text(0)
	switch {
	case tx == nil && op == "abort":
		return nothing()
	case tx == nil:
		return nil, &idb.Error{Name: "InvalidStateError", Message: "The transaction has finished."}
	}
	switch op {
	case "commit":
		tx.Commit()
	case "abort":
		tx.Abort()
	case "createStore":
		id, err := tx.CreateStore(a.text(2), a.text(3), a.flag(4))
		return id, err
	case "deleteStore":
		return nil, tx.DeleteStore(id(a, 2))
	case "renameStore":
		return nil, tx.RenameStore(id(a, 2), a.text(3))
	case "createIndex":
		made, err := tx.CreateIndex(id(a, 2), a.text(3), a.text(4), a.flag(5), a.flag(6))
		return made, err
	case "deleteIndex":
		return nil, tx.DeleteIndex(id(a, 2), id(a, 3))
	case "renameIndex":
		return nil, tx.RenameIndex(id(a, 2), id(a, 3), a.text(4))
	case "nextKey":
		return tx.NextKey(id(a, 2))
	case "empty":
		return nil, tx.Clear(id(a, 2))
	default:
		return r.asks(tx, op, a)
	}
	return nothing()
}

// id is a number of a store or an index a script gave.
func id(a arguments, i int) uint32 { return uint32(a.number(i)) }

// asks answers a request whose keys and values are in the buffer
// the script shares: the length of the answer left there, or minus the
// length of one it is too short for, which the script then asks for.
func (r *Runtime) asks(tx *idb.Tx, op string, a arguments) (any, error) {
	buffer, _ := a[5].([]byte)
	in := &wire{b: buffer[:min(max(int(a.number(4)), 0), len(buffer))]}
	store, index := id(a, 2), id(a, 3)
	var out []byte
	var err error
	switch op {
	case "get":
		var found idb.Found
		found, err = tx.Get(store, index, in.span())
		out = record(out, found, int(a.number(6))&4 != 0)
	case "all":
		var all []idb.Found
		all, err = tx.All(store, index, in.span(), int(a.number(6)))
		out = records(all, int(a.number(7)))
	case "count":
		count, failed := tx.Count(store, index, in.span())
		return count, either(in, failed)
	case "step":
		c := idb.Cursor{Range: in.span(), Position: in.key(), Primary: in.key(), Key: in.key(), ToPrimary: in.key(),
			Direction: idb.Direction(int(a.number(6)) & 3), Count: int(a.number(7))}
		var found idb.Found
		found, err = tx.Step(store, index, c)
		out = record(out, found, a.flag(8))
	case "put":
		key, value, refs := in.key(), bytes.Clone(in.bytes()), in.refs()
		if in.bad {
			return nil, errWire
		}
		key, err = tx.Put(store, key, value, refs, a.flag(6))
		out = appendBytes(out, []byte(key))
	case "remove":
		err = tx.Delete(store, in.span())
	case "list":
		err = tx.List(store, index, in.key(), in.keys())
	default:
		return nil, errors.New("light: no operation of the databases " + op)
	}
	if err = either(in, err); err != nil {
		return nil, err
	}
	if len(out) > len(buffer) {
		r.bases.answer = out
		return -len(out), nil
	}
	return copy(buffer, out), nil
}

func either(in *wire, err error) error {
	if in.bad {
		return errWire
	}
	return err
}

// record writes whether a record was found, then its keys, and its
// value when it is wanted.
func record(out []byte, found idb.Found, value bool) []byte {
	if !found.OK {
		return append(out, 0)
	}
	out = appendBytes(appendBytes(append(out, 1), []byte(found.Key)), []byte(found.Primary))
	if value {
		out = appendBytes(out, found.Value)
	}
	return out
}

// records writes how many records were found, then of each what is
// wanted: its key where it was read (1), its own (2), its value (4).
func records(all []idb.Found, wanted int) []byte {
	size := 4
	for _, f := range all {
		size += 12 + len(f.Key) + len(f.Primary) + len(f.Value)
	}
	out := binary.BigEndian.AppendUint32(make([]byte, 0, size), uint32(len(all))) //nolint:gosec // a count of records
	for _, f := range all {
		if wanted&1 != 0 {
			out = appendBytes(out, []byte(f.Key))
		}
		if wanted&2 != 0 {
			out = appendBytes(out, []byte(f.Primary))
		}
		if wanted&4 != 0 {
			out = appendBytes(out, f.Value)
		}
	}
	return out
}

func appendBytes(out, b []byte) []byte {
	out = binary.BigEndian.AppendUint32(out, uint32(len(b))) //nolint:gosec // a length under what a site holds
	return append(out, b...)
}

// wire reads what a script wrote in the buffer it shares: bad once it
// was not whole.
type wire struct {
	b   []byte
	bad bool
}

func (w *wire) count() int {
	if len(w.b) < 4 {
		w.bad, w.b = true, nil
		return 0
	}
	n := binary.BigEndian.Uint32(w.b)
	w.b = w.b[4:]
	return int(n)
}

func (w *wire) bytes() []byte {
	n := w.count()
	if n > len(w.b) {
		w.bad, w.b = true, nil
		return nil
	}
	out := w.b[:n]
	w.b = w.b[n:]
	return out
}

func (w *wire) key() idb.Key { return idb.Key(w.bytes()) }

func (w *wire) span() idb.Range {
	r := idb.Range{Lower: w.key(), Upper: w.key()}
	if len(w.b) == 0 {
		w.bad = true
		return r
	}
	r.LowerOpen, r.UpperOpen = w.b[0]&1 != 0, w.b[0]&2 != 0
	w.b = w.b[1:]
	return r
}

func (w *wire) keys() []idb.Key {
	n := w.count()
	if n > len(w.b) {
		w.bad = true
		return nil
	}
	keys := make([]idb.Key, n)
	for i := range keys {
		keys[i] = w.key()
	}
	return keys
}

func (w *wire) refs() []idb.Ref {
	n := w.count()
	if n > len(w.b) {
		w.bad = true
		return nil
	}
	refs := make([]idb.Ref, n)
	for i := range refs {
		refs[i] = idb.Ref{Index: uint32(w.count()), Keys: w.keys()} //nolint:gosec // the number of an index
	}
	return refs
}
