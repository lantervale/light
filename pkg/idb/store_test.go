package idb_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/idb"
)

// doc is a document of a site, and what it was told.
type doc struct {
	t    *testing.T
	c    *idb.Client
	told []idb.Event
}

func document(t *testing.T, site *idb.Site) *doc {
	t.Helper()
	d := &doc{t: t}
	d.c = site.Client(func(e idb.Event) { d.told = append(d.told, e) })
	return d
}

// next is the oldest event the document has not read, of that kind.
func (d *doc) next(kind idb.Kind) idb.Event {
	d.t.Helper()
	if len(d.told) == 0 {
		d.t.Fatalf("nothing was told, where %d was to be", kind)
	}
	e := d.told[0]
	d.told = d.told[1:]
	if e.Kind != kind {
		d.t.Fatalf("told %+v, not %d", e, kind)
	}
	return e
}

func (d *doc) quiet() {
	d.t.Helper()
	if len(d.told) > 0 {
		d.t.Fatalf("told %+v, where nothing was to be", d.told)
	}
}

// open opens a database at a version, with upgrade run in the
// transaction of its upgrade when it has one: the connection.
func (d *doc) open(name string, version uint64, upgrade func(*idb.Tx)) uint64 {
	d.t.Helper()
	d.c.Open(name, version)
	if upgrade != nil {
		e := d.next(idb.Upgrade)
		tx := d.c.Tx(e.Tx)
		upgrade(tx)
		tx.Commit()
	}
	return d.next(idb.Opened).Conn
}

// begin is a transaction that has its stores.
func (d *doc) begin(conn uint64, mode idb.Mode, stores ...string) *idb.Tx {
	d.t.Helper()
	id, started, err := d.c.Begin(conn, stores, mode)
	if err != nil || !started {
		d.t.Fatalf("a transaction on %v: started %v, %v", stores, started, err)
	}
	return d.c.Tx(id)
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func named(err error) string {
	var e *idb.Error
	if errors.As(err, &e) {
		return e.Name
	}
	return fmt.Sprint(err)
}

func n(v float64) idb.Key { return idb.Number(v) }

// keys are the keys of the records of a store or an index.
func keys(tx *idb.Tx, store, index uint32) string {
	var out []string
	for _, f := range must(tx.All(store, index, idb.Range{}, 0)) {
		if index == 0 {
			out = append(out, f.Key.String())
		} else {
			out = append(out, f.Key.String()+"→"+f.Primary.String())
		}
	}
	return strings.Join(out, " ")
}

// people is a database of one store with two indexes: the connection,
// and the numbers of the store, of its index by name, unique, and of
// its index by tags, one entry a tag.
func people(d *doc) (uint64, uint32, uint32, uint32) {
	var store, byName, byTag uint32
	conn := d.open("people", 1, func(tx *idb.Tx) {
		store = must(tx.CreateStore("people", `"id"`, true))
		byName = must(tx.CreateIndex(store, "name", `"name"`, true, false))
		byTag = must(tx.CreateIndex(store, "tags", `"tags"`, false, true))
	})
	return conn, store, byName, byTag
}

func refs(byName, byTag uint32, name string, tags ...string) []idb.Ref {
	list := make([]idb.Key, len(tags))
	for i, tag := range tags {
		list[i] = idb.Text(tag)
	}
	return []idb.Ref{{Index: byName, Keys: []idb.Key{idb.Text(name)}}, {Index: byTag, Keys: list}}
}

func TestRecordsAreKeptInTheOrderOfTheirKeys(t *testing.T) {
	t.Parallel()
	d := document(t, idb.New("").Site("a"))
	var store uint32
	conn := d.open("db", 1, func(tx *idb.Tx) { store = must(tx.CreateStore("s", "null", false)) })
	tx := d.begin(conn, idb.ReadWrite, "s")
	for _, k := range []idb.Key{n(3), idb.Text("b"), n(1), idb.Array(n(1)), idb.Text("a"), idb.Date(5), n(2)} {
		if _, err := tx.Put(store, k, []byte(k.String()), nil, true); err != nil {
			t.Fatal(err)
		}
	}
	if got := keys(tx, store, 0); got != `1 2 3 @5 "a" "b" [1]` {
		t.Errorf("the store holds %s", got)
	}
	if _, err := tx.Put(store, n(2), nil, nil, true); named(err) != "ConstraintError" {
		t.Errorf("adding a key twice: %v", err)
	}
	if _, err := tx.Put(store, "", nil, nil, false); named(err) != "DataError" {
		t.Errorf("no key in a store that makes none: %v", err)
	}
	if _, err := tx.Put(store, "\x10", nil, nil, false); named(err) != "DataError" {
		t.Errorf("a key that is none: %v", err)
	}
	must(tx.Put(store, n(2), []byte("two"), nil, false))
	if f := must(tx.Get(store, 0, idb.Only(n(2)))); string(f.Value) != "two" {
		t.Errorf("2 holds %q", f.Value)
	}
	if f := must(tx.Get(store, 0, idb.Only(n(9)))); f.OK {
		t.Errorf("9 holds %q", f.Value)
	}
	if got := must(tx.Count(store, 0, idb.Range{Lower: n(2), Upper: idb.Text("a"), UpperOpen: true})); got != 3 {
		t.Errorf("%d records from 2 to before a", got)
	}
	if got := must(tx.All(store, 0, idb.Range{Lower: n(2)}, 2)); len(got) != 2 || got[1].Key != n(3) {
		t.Errorf("the first two from 2 are %v", got)
	}
	if err := tx.Delete(store, idb.Range{Lower: n(2), Upper: idb.Date(5)}); err != nil {
		t.Fatal(err)
	}
	if got := keys(tx, store, 0); got != `1 "a" "b" [1]` {
		t.Errorf("after a delete the store holds %s", got)
	}
	if err := tx.Clear(store); err != nil || keys(tx, store, 0) != "" {
		t.Errorf("a cleared store holds %s (%v)", keys(tx, store, 0), err)
	}
	tx.Commit()
	if _, err := tx.Get(store, 0, idb.Range{}); named(err) != "InvalidStateError" {
		t.Errorf("a request of a transaction that committed: %v", err)
	}
}

func TestAStoreMakesItsKeys(t *testing.T) {
	t.Parallel()
	d := document(t, idb.New("").Site("a"))
	var store uint32
	conn := d.open("db", 1, func(tx *idb.Tx) { store = must(tx.CreateStore("s", "null", true)) })
	tx := d.begin(conn, idb.ReadWrite, "s")
	for _, c := range []struct {
		given, want idb.Key
	}{
		{"", n(1)}, {"", n(2)}, {n(10), n(10)}, {"", n(11)}, {n(5), n(5)}, {"", n(12)}, {idb.Text("x"), idb.Text("x")},
		{"", n(13)}, {n(20.7), n(20.7)}, {"", n(21)}, {n(-3), n(-3)}, {"", n(22)},
	} {
		if got := must(tx.Put(store, c.given, nil, nil, false)); got != c.want {
			t.Errorf("a record given %s is kept under %s, not %s", c.given, got, c.want)
		}
	}
	if next := must(tx.NextKey(store)); next != 23 {
		t.Errorf("the next key is %v", next)
	}
	must(tx.Put(store, n(22), nil, nil, false))
	if _, err := tx.Put(store, n(22), nil, nil, true); named(err) != "ConstraintError" {
		t.Fatal(err)
	}
	if err := tx.Clear(store); err != nil {
		t.Fatal(err)
	}
	if got := must(tx.Put(store, "", nil, nil, false)); got != n(23) {
		t.Errorf("after a clear the store makes %s", got)
	}
	must(tx.Put(store, n(1<<53), nil, nil, false))
	if _, err := tx.Put(store, "", nil, nil, false); named(err) != "ConstraintError" {
		t.Errorf("a key past the last: %v", err)
	}
	if _, err := tx.NextKey(store); named(err) != "ConstraintError" {
		t.Errorf("the next key past the last: %v", err)
	}
	tx.Abort()
	tx = d.begin(conn, idb.ReadWrite, "s")
	if got := must(tx.Put(store, "", nil, nil, false)); got != n(1) {
		t.Errorf("after an abort the store makes %s", got)
	}
}

func TestIndexesListRecordsUnderOtherKeys(t *testing.T) {
	t.Parallel()
	d := document(t, idb.New("").Site("a"))
	conn, store, byName, byTag := people(d)
	tx := d.begin(conn, idb.ReadWrite, "people")
	must(tx.Put(store, "", []byte("ada"), refs(byName, byTag, "ada", "x", "y"), false))
	must(tx.Put(store, "", []byte("bob"), refs(byName, byTag, "bob", "y"), false))
	must(tx.Put(store, "", []byte("cyd"), refs(byName, byTag, "cyd"), false))
	if got := keys(tx, store, byName); got != `"ada"→1 "bob"→2 "cyd"→3` {
		t.Errorf("by name: %s", got)
	}
	if got := keys(tx, store, byTag); got != `"x"→1 "y"→1 "y"→2` {
		t.Errorf("by tag: %s", got)
	}
	if got := must(tx.Count(store, byTag, idb.Only(idb.Text("y")))); got != 2 {
		t.Errorf("%d records tagged y", got)
	}
	if f := must(tx.Get(store, byName, idb.Only(idb.Text("bob")))); string(f.Value) != "bob" || f.Primary != n(2) {
		t.Errorf("bob is %+v", f)
	}
	if _, err := tx.Put(store, "", nil, refs(byName, byTag, "ada"), false); named(err) != "ConstraintError" {
		t.Errorf("a name taken: %v", err)
	}
	if got := must(tx.Put(store, "", nil, refs(byName, byTag, "dee"), false)); got != n(4) {
		t.Errorf("the key after one that was refused is %s", got)
	}
	if _, err := tx.Put(store, n(2), nil, refs(byName, byTag, "ada"), false); named(err) != "ConstraintError" {
		t.Errorf("a name taken by another record: %v", err)
	}
	if f := must(tx.Get(store, 0, idb.Only(n(2)))); string(f.Value) != "bob" {
		t.Errorf("a put that was refused left %q", f.Value)
	}
	must(tx.Put(store, n(1), []byte("ada!"), refs(byName, byTag, "ada", "z"), false))
	if got := keys(tx, store, byTag); got != `"y"→2 "z"→1` {
		t.Errorf("by tag, after a record changed its tags: %s", got)
	}
	if err := tx.Delete(store, idb.Only(n(2))); err != nil {
		t.Fatal(err)
	}
	if got := keys(tx, store, byName); got != `"ada"→1 "cyd"→3 "dee"→4` {
		t.Errorf("by name, after a delete: %s", got)
	}
	tx.Commit()
	if got := d.c.Site().Databases(); len(got) != 1 || got[0].Stores[0].Count != 3 || len(got[0].Stores[0].Indexes) != 2 {
		t.Errorf("the site's databases are %+v", got)
	}
}

func TestCursorsStep(t *testing.T) {
	t.Parallel()
	d := document(t, idb.New("").Site("a"))
	var store, index uint32
	conn := d.open("db", 1, func(tx *idb.Tx) {
		store = must(tx.CreateStore("s", "null", false))
		index = must(tx.CreateIndex(store, "i", `"i"`, false, false))
	})
	tx := d.begin(conn, idb.ReadWrite, "s")
	for i, letter := range []string{"a", "a", "b", "b", "b", "c"} {
		must(tx.Put(store, n(float64(i+1)), nil, []idb.Ref{{Index: index, Keys: []idb.Key{idb.Text(letter)}}}, true))
	}
	a, b, c := idb.Text("a"), idb.Text("b"), idb.Text("c")
	walk := func(source uint32, c idb.Cursor) string {
		var out []string
		for range 10 {
			f := must(tx.Step(store, source, c))
			if !f.OK {
				break
			}
			out = append(out, f.Key.String()+"→"+f.Primary.String())
			c.Position, c.Primary, c.Key, c.ToPrimary, c.Count = f.Key, f.Primary, "", "", 1
		}
		return strings.Join(out, " ")
	}
	for _, each := range []struct {
		name   string
		source uint32
		cursor idb.Cursor
		want   string
	}{
		{"a store, next", 0, idb.Cursor{}, "1→1 2→2 3→3 4→4 5→5 6→6"},
		{"a store, prev", 0, idb.Cursor{Direction: idb.Prev}, "6→6 5→5 4→4 3→3 2→2 1→1"},
		{"a store, a range", 0, idb.Cursor{Range: idb.Range{Lower: n(2), Upper: n(5), UpperOpen: true}}, "2→2 3→3 4→4"},
		{"a store, a range, prev", 0, idb.Cursor{Range: idb.Range{Lower: n(2), LowerOpen: true, Upper: n(5)},
			Direction: idb.Prev}, "5→5 4→4 3→3"},
		{"a store, advancing", 0, idb.Cursor{Count: 3}, "3→3 4→4 5→5 6→6"},
		{"a store, advancing back", 0, idb.Cursor{Count: 3, Direction: idb.Prev}, "4→4 3→3 2→2 1→1"},
		{"a store, to a key", 0, idb.Cursor{Position: n(1), Key: n(4.5)}, "5→5 6→6"},
		{"a store, back to a key", 0, idb.Cursor{Direction: idb.Prev, Position: n(6), Key: n(2.5)}, "2→2 1→1"},
		{"a store, past its end", 0, idb.Cursor{Position: n(6)}, ""},
		{"an index, next", index, idb.Cursor{}, `"a"→1 "a"→2 "b"→3 "b"→4 "b"→5 "c"→6`},
		{"an index, nextunique", index, idb.Cursor{Direction: idb.NextUnique}, `"a"→1 "b"→3 "c"→6`},
		{"an index, prev", index, idb.Cursor{Direction: idb.Prev}, `"c"→6 "b"→5 "b"→4 "b"→3 "a"→2 "a"→1`},
		{"an index, prevunique", index, idb.Cursor{Direction: idb.PrevUnique}, `"c"→6 "b"→3 "a"→1`},
		{"an index, a key", index, idb.Cursor{Range: idb.Only(b)}, `"b"→3 "b"→4 "b"→5`},
		{"an index, a key, prevunique", index, idb.Cursor{Range: idb.Only(b), Direction: idb.PrevUnique}, `"b"→3`},
		{"an index, from within a key", index, idb.Cursor{Position: b, Primary: n(3)}, `"b"→4 "b"→5 "c"→6`},
		{"an index, back from within a key", index, idb.Cursor{Direction: idb.Prev, Position: b, Primary: n(4)},
			`"b"→3 "a"→2 "a"→1`},
		{"an index, to a key and a record", index, idb.Cursor{Position: a, Primary: n(1), Key: b, ToPrimary: n(4)},
			`"b"→4 "b"→5 "c"→6`},
		{"an index, back to a key and a record", index, idb.Cursor{Direction: idb.Prev, Position: c, Primary: n(6), Key: b,
			ToPrimary: n(4)}, `"b"→4 "b"→3 "a"→2 "a"→1`},
		{"an index, advancing by keys", index, idb.Cursor{Direction: idb.NextUnique, Count: 2}, `"b"→3 "c"→6`},
		{"an index, advancing back by keys", index, idb.Cursor{Direction: idb.PrevUnique, Count: 2}, `"b"→3 "a"→1`},
		{"an index, advancing back past its start", index, idb.Cursor{Direction: idb.PrevUnique, Count: 4}, ""},
		{"an index, an open range", index, idb.Cursor{Range: idb.Range{Lower: a, LowerOpen: true, Upper: c,
			UpperOpen: true}}, `"b"→3 "b"→4 "b"→5`},
	} {
		if got := walk(each.source, each.cursor); got != each.want {
			t.Errorf("%s: %s, not %s", each.name, got, each.want)
		}
	}
}

func TestTransactionsWaitForTheStoresTheyNeed(t *testing.T) {
	t.Parallel()
	d := document(t, idb.New("").Site("a"))
	conn := d.open("db", 1, func(tx *idb.Tx) {
		must(tx.CreateStore("a", "null", true))
		must(tx.CreateStore("b", "null", true))
		must(tx.CreateStore("c", "null", true))
	})
	begin := func(mode idb.Mode, stores ...string) (uint64, bool) {
		id, started, err := d.c.Begin(conn, stores, mode)
		if err != nil {
			t.Fatal(err)
		}
		return id, started
	}
	read1, started := begin(idb.ReadOnly, "a")
	if !started {
		t.Fatal("the first transaction waits")
	}
	if _, started = begin(idb.ReadOnly, "a", "b"); !started {
		t.Error("two transactions that read wait for one another")
	}
	write, started := begin(idb.ReadWrite, "a")
	if started {
		t.Error("a transaction writes a store another reads")
	}
	apart, started := begin(idb.ReadWrite, "c")
	if !started {
		t.Error("a transaction waits for stores it does not use")
	}
	late, started := begin(idb.ReadOnly, "a")
	if started {
		t.Error("a transaction reads a store one before it is to write")
	}
	both, started := begin(idb.ReadWrite, "a", "b")
	if started {
		t.Error("a transaction writes stores others hold")
	}
	if _, err := d.c.Tx(write).Put(1, "", nil, nil, false); named(err) != "InvalidStateError" {
		t.Errorf("a request of a transaction that waits: %v", err)
	}
	d.c.Tx(read1).Commit()
	d.quiet()
	d.c.Tx(read1 + 1).Commit()
	if e := d.next(idb.Started); e.Tx != write {
		t.Errorf("%d was started, not %d", e.Tx, write)
	}
	d.quiet()
	d.c.Tx(write).Abort()
	if e := d.next(idb.Started); e.Tx != late {
		t.Errorf("%d was started, not %d", e.Tx, late)
	}
	d.quiet()
	d.c.Tx(late).Commit()
	if e := d.next(idb.Started); e.Tx != both {
		t.Errorf("%d was started, not %d", e.Tx, both)
	}
	if _, _, err := d.c.Begin(conn, []string{"nope"}, idb.ReadOnly); named(err) != "NotFoundError" {
		t.Errorf("a transaction on a store that is none: %v", err)
	}
	if _, err := d.c.Tx(both).Put(3, "", nil, nil, false); named(err) != "NotFoundError" {
		t.Errorf("a request for a store the transaction has not: %v", err)
	}
	d.c.Tx(apart).Commit()
	reader := d.begin(conn, idb.ReadOnly, "c")
	if _, err := reader.Put(3, "", nil, nil, false); named(err) != "ReadOnlyError" {
		t.Errorf("a put in a transaction that reads: %v", err)
	}
	if err := reader.Clear(3); named(err) != "ReadOnlyError" {
		t.Errorf("a clear in a transaction that reads: %v", err)
	}
	if _, err := reader.CreateStore("z", "null", false); named(err) != "InvalidStateError" {
		t.Errorf("a store made outside an upgrade: %v", err)
	}
}

func TestAnAbortUndoesWhatATransactionChanged(t *testing.T) {
	t.Parallel()
	site := idb.New("").Site("a")
	d := document(t, site)
	conn, store, byName, byTag := people(d)
	tx := d.begin(conn, idb.ReadWrite, "people")
	must(tx.Put(store, "", []byte("ada"), refs(byName, byTag, "ada", "x"), false))
	must(tx.Put(store, "", []byte("bob"), refs(byName, byTag, "bob", "x"), false))
	tx.Commit()
	before := site.Usage()

	tx = d.begin(conn, idb.ReadWrite, "people")
	must(tx.Put(store, n(1), []byte("ADA"), refs(byName, byTag, "eve", "q"), false))
	must(tx.Put(store, "", []byte("cyd"), refs(byName, byTag, "cyd"), false))
	if err := tx.Delete(store, idb.Only(n(2))); err != nil {
		t.Fatal(err)
	}
	must(tx.Put(store, n(2), []byte("BOB"), refs(byName, byTag, "bob2"), false))
	if err := tx.Clear(store); err != nil {
		t.Fatal(err)
	}
	must(tx.Put(store, "", []byte("dee"), refs(byName, byTag, "ada"), false))
	tx.Abort()

	tx = d.begin(conn, idb.ReadWrite, "people")
	if got := keys(tx, store, byName); got != `"ada"→1 "bob"→2` {
		t.Errorf("by name, after an abort: %s", got)
	}
	if got := keys(tx, store, byTag); got != `"x"→1 "x"→2` {
		t.Errorf("by tag, after an abort: %s", got)
	}
	if f := must(tx.Get(store, 0, idb.Only(n(1)))); string(f.Value) != "ada" {
		t.Errorf("1 holds %q after an abort", f.Value)
	}
	if got := must(tx.Put(store, "", nil, nil, false)); got != n(3) {
		t.Errorf("after an abort the store makes %s", got)
	}
	tx.Abort()
	if site.Usage() != before {
		t.Errorf("the site holds %d bytes after an abort, %d before", site.Usage(), before)
	}
	d.c.Shut(conn)

	d.c.Open("people", 2)
	e := d.next(idb.Upgrade)
	if e.Old != 1 || e.New != 2 || len(e.Database.Stores) != 1 {
		t.Fatalf("the upgrade is %+v", e)
	}
	up := d.c.Tx(e.Tx)
	if err := up.RenameStore(store, "folk"); err != nil {
		t.Fatal(err)
	}
	if err := up.RenameIndex(store, byName, "called"); err != nil {
		t.Fatal(err)
	}
	if err := up.DeleteIndex(store, byTag); err != nil {
		t.Fatal(err)
	}
	other := must(up.CreateStore("other", "null", false))
	must(up.Put(other, n(1), []byte("x"), nil, false))
	byLength := must(up.CreateIndex(store, "length", `"length"`, false, false))
	if err := up.List(store, byLength, n(1), []idb.Key{n(3)}); err != nil {
		t.Fatal(err)
	}
	if err := up.DeleteStore(store); err != nil {
		t.Fatal(err)
	}
	up.Abort()
	if e = d.next(idb.Failed); e.Err.Name != "AbortError" {
		t.Errorf("the open of an upgrade that aborted: %+v", e)
	}
	conn = d.open("people", 0, nil)
	got := site.Databases()
	if len(got) != 1 || got[0].Version != 1 || len(got[0].Stores) != 1 || got[0].Stores[0].Name != "people" ||
		len(got[0].Stores[0].Indexes) != 2 || got[0].Stores[0].Indexes[0].Name != "name" {
		t.Fatalf("after an upgrade that aborted the databases are %+v", got)
	}
	tx = d.begin(conn, idb.ReadOnly, "people")
	if got := keys(tx, store, byTag); got != `"x"→1 "x"→2` {
		t.Errorf("by tag, after an upgrade that aborted: %s", got)
	}
	if site.Usage() != before {
		t.Errorf("the site holds %d bytes after an upgrade that aborted, %d before", site.Usage(), before)
	}

	d.c.Open("new", 3)
	d.c.Tx(d.next(idb.Upgrade).Tx).Abort()
	d.next(idb.Failed)
	if listed := d.c.Databases(); len(listed) != 1 {
		t.Errorf("a database whose first upgrade aborted is listed: %+v", listed)
	}
}

func TestAnIndexFilledLaterKeepsItsRule(t *testing.T) {
	t.Parallel()
	d := document(t, idb.New("").Site("a"))
	var store uint32
	conn := d.open("db", 1, func(tx *idb.Tx) {
		store = must(tx.CreateStore("s", "null", false))
		must(tx.Put(store, n(1), []byte("a"), nil, false))
		must(tx.Put(store, n(2), []byte("a"), nil, false))
	})
	d.c.Shut(conn)
	d.c.Open("db", 2)
	up := d.c.Tx(d.next(idb.Upgrade).Tx)
	index := must(up.CreateIndex(store, "i", `""`, true, false))
	if err := up.List(store, index, n(1), []idb.Key{idb.Text("a")}); err != nil {
		t.Fatal(err)
	}
	if err := up.List(store, index, n(2), []idb.Key{idb.Text("a")}); named(err) != "ConstraintError" {
		t.Errorf("a second record under a key of a unique index: %v", err)
	}
	if _, err := up.CreateIndex(store, "i", `""`, false, false); named(err) != "ConstraintError" {
		t.Errorf("an index named as another: %v", err)
	}
	if _, err := up.CreateStore("s", "null", false); named(err) != "ConstraintError" {
		t.Errorf("a store named as another: %v", err)
	}
	up.Commit()
	d.next(idb.Opened)
}

func TestConnectionsAreAskedToCloseForAnUpgrade(t *testing.T) {
	t.Parallel()
	site := idb.New("").Site("a")
	one, two := document(t, site), document(t, site)
	first := one.open("db", 1, func(*idb.Tx) {})
	second := two.open("db", 0, nil)
	if site.Connections() != 2 {
		t.Fatalf("%d connections are open", site.Connections())
	}

	request := two.c.Open("db", 2)
	if e := one.next(idb.VersionChange); e.Conn != first || e.Old != 1 || e.New != 2 {
		t.Errorf("the first connection was told %+v", e)
	}
	if e := two.next(idb.VersionChange); e.Conn != second {
		t.Errorf("the second connection was told %+v", e)
	}
	two.c.Shut(second)
	two.c.Heard(second)
	two.quiet()
	one.c.Heard(first)
	if e := two.next(idb.Blocked); e.Request != request || e.Old != 1 || e.New != 2 {
		t.Errorf("the request was told %+v", e)
	}
	later := one.c.Open("db", 0)
	one.c.Shut(first)
	up := two.next(idb.Upgrade)
	if up.Request != request || up.Old != 1 || up.New != 2 {
		t.Errorf("the upgrade is %+v", up)
	}
	one.quiet()
	two.c.Tx(up.Tx).Commit()
	two.next(idb.Opened)
	reopened := one.next(idb.Opened)
	if reopened.Request != later || reopened.Database.Version != 2 {
		t.Errorf("the request that waited was told %+v", reopened)
	}

	if e := failing(one, "db", 1); e.Err.Name != "VersionError" {
		t.Errorf("an open at a version before the database's: %+v", e)
	}

	removal := one.c.Delete("db")
	if e := two.next(idb.VersionChange); e.Old != 2 || e.New != 0 {
		t.Errorf("a connection was told of a delete %+v", e)
	}
	one.next(idb.VersionChange)
	two.c.Close()
	one.quiet()
	one.c.Shut(reopened.Conn)
	if e := one.next(idb.Deleted); e.Request != removal || e.Old != 2 {
		t.Errorf("the delete was answered %+v", e)
	}
	if e := one.c.Delete("db"); one.next(idb.Deleted).Request != e {
		t.Error("the delete of no database was not answered")
	}
	if site.Connections() != 0 || len(site.Databases()) != 0 {
		t.Errorf("%d connections and %d databases are left", site.Connections(), len(site.Databases()))
	}
}

func failing(d *doc, name string, version uint64) idb.Event {
	d.c.Open(name, version)
	return d.next(idb.Failed)
}

func TestADocumentThatEndsLetsGoOfWhatItHeld(t *testing.T) {
	t.Parallel()
	site := idb.New("").Site("a")
	one, two := document(t, site), document(t, site)
	var store uint32
	conn := one.open("db", 1, func(tx *idb.Tx) { store = must(tx.CreateStore("s", "null", true)) })
	tx := one.begin(conn, idb.ReadWrite, "s")
	must(tx.Put(store, "", []byte("kept for now"), nil, false))
	other := two.open("db", 0, nil)
	waiting, started, err := two.c.Begin(other, []string{"s"}, idb.ReadOnly)
	if err != nil || started {
		t.Fatalf("a transaction on a store another writes: started %v, %v", started, err)
	}
	two.c.Open("db", 2)
	two.next(idb.VersionChange)
	one.c.Open("db", 3)
	one.c.Close()
	if e := two.next(idb.Started); e.Tx != waiting {
		t.Errorf("told %+v", e)
	}
	if got := must(two.c.Tx(waiting).Count(store, 0, idb.Range{})); got != 0 {
		t.Errorf("%d records are left of a transaction whose document ended", got)
	}
	two.c.Tx(waiting).Commit()
	two.c.Shut(other)
	two.c.Tx(two.next(idb.Upgrade).Tx).Commit()
	two.next(idb.Opened)
	two.quiet()
	two.c.Close()
	if site.Connections() != 0 {
		t.Errorf("%d connections are left", site.Connections())
	}
}

func TestSitesKeepTheirDatabasesApart(t *testing.T) {
	t.Parallel()
	store := idb.New("")
	a, b := document(t, store.Site("https://a.test")), document(t, store.Site("https://top.test\x00https://a.test"))
	var id uint32
	conn := a.open("db", 3, func(tx *idb.Tx) {
		id = must(tx.CreateStore("s", "null", false))
		must(tx.Put(id, n(1), []byte("a's"), nil, false))
	})
	if got := b.c.Databases(); len(got) != 0 {
		t.Errorf("a site lists another's databases: %+v", got)
	}
	other := b.open("db", 1, func(tx *idb.Tx) { must(tx.CreateStore("s", "null", false)) })
	tx := b.begin(other, idb.ReadOnly, "s")
	if got := must(tx.Count(1, 0, idb.Range{})); got != 0 {
		t.Errorf("a site reads %d records of another's", got)
	}
	a.quiet()
	if got := a.c.Databases(); len(got) != 1 || got[0].Version != 3 {
		t.Errorf("a site's databases are %+v", got)
	}
	if store.Site("https://a.test") != a.c.Site() {
		t.Error("a site asked for twice is two")
	}
	store.Clear()
	if e := a.next(idb.Closed); e.Conn != conn {
		t.Errorf("a connection to a database that was cleared was told %+v", e)
	}
	b.next(idb.Closed)
	if _, err := tx.Count(1, 0, idb.Range{}); named(err) != "InvalidStateError" {
		t.Errorf("a transaction on a database that was cleared: %v", err)
	}
	if len(a.c.Databases())+len(b.c.Databases()) != 0 {
		t.Error("databases are left after a clear")
	}
}

func TestASiteHoldsSoMuch(t *testing.T) {
	t.Parallel()
	store := idb.New("")
	store.SetQuota(1000)
	d, other := document(t, store.Site("a")), document(t, store.Site("b"))
	var id uint32
	conn := d.open("db", 1, func(tx *idb.Tx) { id = must(tx.CreateStore("s", "null", false)) })
	tx := d.begin(conn, idb.ReadWrite, "s")
	must(tx.Put(id, n(1), make([]byte, 600), nil, false))
	if _, err := tx.Put(id, n(2), make([]byte, 600), nil, false); named(err) != "QuotaExceededError" {
		t.Errorf("a put past what a site may hold: %v", err)
	}
	must(tx.Put(id, n(1), make([]byte, 900), nil, false))
	if _, err := tx.Put(id, n(1), make([]byte, 1000), nil, false); named(err) != "QuotaExceededError" {
		t.Errorf("a record grown past what a site may hold: %v", err)
	}
	must(tx.Put(id, n(1), make([]byte, 10), nil, false))
	tx.Commit()
	if got := d.c.Site().Usage(); got != 19 || d.c.Site().Quota() != 1000 {
		t.Errorf("the site holds %d bytes of %d", got, d.c.Site().Quota())
	}
	conn = other.open("db", 1, func(tx *idb.Tx) { id = must(tx.CreateStore("s", "null", false)) })
	tx = other.begin(conn, idb.ReadWrite, "s")
	if _, err := tx.Put(id, n(1), make([]byte, 900), nil, false); err != nil {
		t.Errorf("a site is held to what another holds: %v", err)
	}
}

func TestDatabasesOutlastTheRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := idb.New(dir)
	d := document(t, store.Site("https://a.test"))
	if !d.c.Site().Kept() {
		t.Error("a store given a folder says it keeps nothing")
	}
	conn, people, byName, byTag := people(d)
	tx := d.begin(conn, idb.ReadWrite, "people")
	must(tx.Put(people, "", []byte("ada"), refs(byName, byTag, "ada", "x", "y"), false))
	must(tx.Put(people, "", []byte("bob"), refs(byName, byTag, "bob", "y"), false))
	must(tx.Put(people, "", []byte("gone"), refs(byName, byTag, "gone"), false))
	if err := tx.Delete(people, idb.Only(n(3))); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	tx = d.begin(conn, idb.ReadWrite, "people")
	must(tx.Put(people, "", []byte("never"), refs(byName, byTag, "never"), false))
	tx.Abort()
	d.c.Shut(conn)
	d.open("people", 2, func(tx *idb.Tx) {
		if err := tx.RenameStore(people, "folk"); err != nil {
			t.Fatal(err)
		}
		if err := tx.DeleteIndex(people, byTag); err != nil {
			t.Fatal(err)
		}
		byLength := must(tx.CreateIndex(people, "length", `"length"`, false, false))
		for _, k := range []float64{1, 2} {
			if err := tx.List(people, byLength, n(k), []idb.Key{n(3)}); err != nil {
				t.Fatal(err)
			}
		}
		other := must(tx.CreateStore("other", `["a","b"]`, false))
		must(tx.Put(other, idb.Array(n(1), n(2)), []byte{0, 1, 2}, nil, false))
		must(tx.Put(other, n(7), nil, nil, false))
		if err := tx.Clear(other); err != nil {
			t.Fatal(err)
		}
		dropped := must(tx.CreateStore("dropped", "null", false))
		if err := tx.DeleteStore(dropped); err != nil {
			t.Fatal(err)
		}
	})
	left := d.open("left", 1, func(*idb.Tx) {})
	d.c.Delete("left")
	d.next(idb.VersionChange)
	d.c.Shut(left)
	d.next(idb.Deleted)
	d.c.Close()
	want := fmt.Sprintf("%+v", d.c.Site().Databases())
	usage := d.c.Site().Usage()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		store = idb.New(dir)
		site := store.Site("https://a.test")
		if got := fmt.Sprintf("%+v", site.Databases()); got != want {
			t.Fatalf("read again, the databases are\n%s, not\n%s", got, want)
		}
		if site.Usage() != usage {
			t.Errorf("read again, the site holds %d bytes, not %d", site.Usage(), usage)
		}
		if got := store.Site("https://b.test").Databases(); len(got) != 0 {
			t.Errorf("another site reads %+v", got)
		}
		d = document(t, site)
		conn = d.open("people", 0, nil)
		tx = d.begin(conn, idb.ReadWrite, "folk")
		if got := keys(tx, people, byName); got != `"ada"→1 "bob"→2` {
			t.Errorf("read again, by name: %s", got)
		}
		if got := must(tx.Put(people, "", []byte("temp"), nil, false)); got != n(4) {
			t.Errorf("read again, the store makes %s", got)
		}
		tx.Abort()
		d.c.Close()
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAFileCutShortIsReadUpToTheCut(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := idb.New(dir)
	d := document(t, store.Site("a"))
	var id uint32
	conn := d.open("db", 1, func(tx *idb.Tx) { id = must(tx.CreateStore("s", "null", false)) })
	for i := range 3 {
		tx := d.begin(conn, idb.ReadWrite, "s")
		must(tx.Put(id, n(float64(i)), []byte("value"), nil, false))
		tx.Commit()
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, idb.Folder, "*.idb"))
	if err != nil || len(files) != 1 {
		t.Fatalf("the folder holds %v (%v)", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(files[0], append(data[:len(data)-3], "garbage"...), 0o600); err != nil {
		t.Fatal(err)
	}

	store = idb.New(dir)
	d = document(t, store.Site("a"))
	conn = d.open("db", 0, nil)
	tx := d.begin(conn, idb.ReadWrite, "s")
	if got := keys(tx, id, 0); got != "0 1" {
		t.Errorf("a file cut short reads %s", got)
	}
	must(tx.Put(id, n(9), []byte("after"), nil, false))
	tx.Commit()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = idb.New(dir)
	d = document(t, store.Site("a"))
	tx = d.begin(d.open("db", 0, nil), idb.ReadOnly, "s")
	if got := keys(tx, id, 0); got != "0 1 9" {
		t.Errorf("written after a cut, the file reads %s", got)
	}
	if err = os.WriteFile(files[0], []byte("not a file of light's"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := idb.New(dir).Site("a").Databases(); len(got) != 0 {
		t.Errorf("a file that is not light's reads %+v", got)
	}
}

func TestAFileIsWrittenAgainOnceItHoldsMuchMoreThanItsSite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := idb.New(dir)
	d := document(t, store.Site("a"))
	var id uint32
	conn := d.open("db", 1, func(tx *idb.Tx) {
		id = must(tx.CreateStore("s", "null", false))
		must(tx.CreateStore("t", "null", false))
	})
	held := d.begin(conn, idb.ReadWrite, "t")
	must(held.Put(id+1, n(1), []byte("not committed"), nil, false))
	value := make([]byte, 200<<10)
	for i := range 40 {
		tx := d.begin(conn, idb.ReadWrite, "s")
		must(tx.Put(id, n(1), value, nil, false))
		tx.Commit()
		if i == 20 {
			held.Abort()
		}
	}
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, idb.Folder, "*"))
	if len(files) != 1 {
		t.Fatalf("the folder holds %v", files)
	}
	info, err := os.Stat(files[0])
	if err != nil || info.Size() > 3<<20 {
		t.Errorf("the file of a site that holds 200 kB is of %d bytes (%v)", info.Size(), err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = idb.New(dir)
	got := store.Site("a").Databases()
	if len(got) != 1 || !slices.Equal([]int{got[0].Stores[0].Count, got[0].Stores[1].Count}, []int{1, 0}) {
		t.Errorf("read again, the databases are %+v", got)
	}
	store.Clear()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, idb.Folder, "*")); len(left) != 0 {
		t.Errorf("a clear left %v", left)
	}
}

func TestNothingIsWrittenWithoutAFolder(t *testing.T) {
	t.Parallel()
	store := idb.New("")
	d := document(t, store.Site("a"))
	d.open("db", 1, func(tx *idb.Tx) { must(tx.CreateStore("s", "null", false)) })
	if d.c.Site().Kept() {
		t.Error("a store given no folder says it keeps its databases")
	}
	if err := errors.Join(store.Flush(), store.Close()); err != nil {
		t.Fatal(err)
	}
}
