// Package idb is the databases sites keep in a browser, as IndexedDB
// gives them to their scripts: object stores of records in the order
// of their keys, their indexes, the transactions that read and change
// them one after another, and the files they are kept in between runs.
// A value is bytes the package never reads, and a key is one already
// written to compare byte by byte (Key): what makes them of a script's
// values is the script's side.
package idb

import (
	"slices"
	"sort"
	"sync"
)

// MaxBytes is how much the databases of one site may hold: keys,
// values and the keys of their indexes together.
const MaxBytes = 128 << 20

// Error is why a request failed, by the name the standard gives it.
type Error struct {
	Name, Message string
}

func (e *Error) Error() string { return e.Name + ": " + e.Message }

func failed(name, message string) *Error { return &Error{Name: name, Message: message} }

// Store is every site's databases, safe to use from any goroutine.
type Store struct {
	mu    sync.Mutex
	dir   string
	quota int64
	sites map[string]*Site
	out   *writer
	last  uint64
}

// New is the databases kept in the folder dir, read as each site asks
// for its own, and kept nowhere when dir is "".
func New(dir string) *Store {
	s := &Store{dir: dir, quota: MaxBytes, sites: map[string]*Site{}}
	if dir != "" {
		s.out = newWriter()
	}
	return s
}

// SetQuota has each site hold at most that many bytes.
func (s *Store) SetQuota(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quota = bytes
}

// Site is the databases of one site, which key names: no other key
// reads them.
func (s *Store) Site(key string) *Site {
	s.mu.Lock()
	defer s.mu.Unlock()
	site := s.sites[key]
	if site == nil {
		site = &Site{store: s, key: key, dbs: map[string]*database{}, byID: map[uint32]*database{},
			queue: map[string][]*request{}}
		s.sites[key] = site
		site.load()
	}
	return site
}

// Clear forgets every site's databases and removes their files: the
// connections open to them are closed, and told so.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, site := range s.sites {
		for _, db := range site.dbs {
			site.shut(db)
			site.drop(db)
		}
		for name := range site.queue {
			site.advance(name)
		}
		site.logged = 0
	}
	if s.out != nil {
		s.out.add(job{path: s.folder(), remove: true})
	}
}

// Flush waits for what was committed to be in its files.
func (s *Store) Flush() error {
	if s.out == nil {
		return nil
	}
	return s.out.wait()
}

// Close writes what is not yet written; what is committed later is
// kept for the run alone.
func (s *Store) Close() error {
	s.mu.Lock()
	out := s.out
	s.out = nil
	s.mu.Unlock()
	if out == nil {
		return nil
	}
	return out.close()
}

func (s *Store) next() uint64 {
	s.last++
	return s.last
}

// Site is the databases of one site.
type Site struct {
	store *Store
	key   string
	dbs   map[string]*database
	byID  map[uint32]*database
	// queue are the requests to open or delete each database, which
	// are answered one after another.
	queue  map[string][]*request
	lastDB uint32
	// logged is how many bytes the site's file holds.
	logged int64
}

// Info is a database as a developer reads it.
type Info struct {
	Name    string
	Version uint64
	Stores  []StoreInfo
}

// StoreInfo is an object store of a database: Count is how many
// records it holds.
type StoreInfo struct {
	ID            uint32
	Name, KeyPath string
	Auto          bool
	Count         int
	Indexes       []IndexInfo
}

// IndexInfo is an index of an object store.
type IndexInfo struct {
	ID            uint32
	Name, KeyPath string
	Unique, Multi bool
}

// Databases are the site's databases, in the order of their names.
func (s *Site) Databases() []Info {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	var all []Info
	for _, db := range s.dbs {
		if db.version > 0 {
			all = append(all, db.info())
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}

// Usage is how many bytes the site's databases hold, and Quota how
// many they may.
func (s *Site) Usage() int64 {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return s.usage()
}

func (s *Site) Quota() int64 {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return s.store.quota
}

// Kept reports whether the site's databases outlast the run.
func (s *Site) Kept() bool {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return s.store.out != nil
}

// Connections is how many connections are open to the site's
// databases.
func (s *Site) Connections() int {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	n := 0
	for _, db := range s.dbs {
		n += len(db.conns)
	}
	return n
}

func (s *Site) usage() int64 {
	var n int64
	for _, db := range s.dbs {
		for _, st := range db.stores {
			n += st.bytes
		}
	}
	return n
}

// database is one database of a site.
type database struct {
	id      uint32
	name    string
	version uint64
	stores  map[uint32]*objectStore
	lastID  uint32
	conns   []*conn
	// txs are the transactions that have not finished, the oldest
	// first.
	txs []*Tx
}

func (db *database) info() Info {
	out := Info{Name: db.name, Version: db.version}
	for _, st := range db.stores {
		si := StoreInfo{ID: st.id, Name: st.name, KeyPath: st.keyPath, Auto: st.auto, Count: len(st.records)}
		for _, x := range st.indexes {
			si.Indexes = append(si.Indexes, IndexInfo{ID: x.id, Name: x.name, KeyPath: x.keyPath, Unique: x.unique,
				Multi: x.multi})
		}
		sort.Slice(si.Indexes, func(i, j int) bool { return si.Indexes[i].Name < si.Indexes[j].Name })
		out.Stores = append(out.Stores, si)
	}
	sort.Slice(out.Stores, func(i, j int) bool { return out.Stores[i].Name < out.Stores[j].Name })
	return out
}

func (db *database) named(name string) *objectStore {
	for _, st := range db.stores {
		if st.name == name {
			return st
		}
	}
	return nil
}

func (s *Site) create(name string) *database {
	s.lastDB++
	db := &database{id: s.lastDB, name: name, stores: map[uint32]*objectStore{}}
	s.dbs[name], s.byID[db.id] = db, db
	return db
}

// drop forgets a database, in the site's file too when it was ever
// written there.
func (s *Site) drop(db *database) {
	delete(s.dbs, db.name)
	delete(s.byID, db.id)
	if db.version > 0 {
		var e enc
		e.op(opDrop, db.id)
		s.append(e.b, nil)
	}
}

// shut closes every connection to a database, whose transactions are
// undone, and tells those who held them.
func (s *Site) shut(db *database) {
	for _, tx := range slices.Clone(db.txs) {
		tx.abort()
	}
	for _, c := range slices.Clone(db.conns) {
		c.close()
		c.client.notify(Event{Kind: Closed, Conn: c.id})
	}
}

// conn is one connection to a database.
type conn struct {
	id      uint64
	client  *Client
	db      *database
	closing bool
	// owes is the request the connection was told of and has not said
	// it heard.
	owes *request
}

func (db *database) connect(client *Client) *conn {
	c := &conn{id: client.site.store.next(), client: client, db: db}
	db.conns = append(db.conns, c)
	client.conns[c.id] = c
	return c
}

// close ends a connection whose transactions have finished.
func (c *conn) close() {
	c.closing = true
	c.db.conns = slices.DeleteFunc(c.db.conns, func(o *conn) bool { return o == c })
	delete(c.client.conns, c.id)
}

func (c *conn) busy() bool {
	return slices.ContainsFunc(c.db.txs, func(tx *Tx) bool { return tx.conn == c })
}

// Kind is what an Event tells of.
type Kind int

const (
	// Opened is a request to open answered with Conn, a connection to
	// Database, and Upgrade one answered first with Tx, the transaction
	// that takes the database from the version Old to New.
	Opened Kind = iota + 1
	Upgrade
	// Failed is a request to open that ended with Err.
	Failed
	// Deleted is a request to delete answered: the database was at the
	// version Old.
	Deleted
	// Blocked is a request that waits for other connections to close.
	Blocked
	// VersionChange asks Conn to close, for the database to go from
	// the version Old to New, 0 when it is to be deleted.
	VersionChange
	// Started is Tx given the stores it waited for.
	Started
	// Closed is Conn closed by the browser, its data cleared.
	Closed
)

// Event is what a Client is told of, later than what it asked.
type Event struct {
	Kind              Kind
	Request, Conn, Tx uint64
	Old, New          uint64
	Err               *Error
	Database          Info
}

// request is a request to open or to delete a database.
type request struct {
	id      uint64
	client  *Client
	name    string
	version uint64
	remove  bool
	state   int
	// created is whether the database is there for this request alone.
	created bool
}

// The states of a request, in their order.
const (
	asked = iota
	acking
	closing
	upgrading
)

// Client is one document's use of its site's databases.
type Client struct {
	site   *Site
	notify func(Event)
	conns  map[uint64]*conn
	txs    map[uint64]*Tx
}

// Client is a new document of the site. notify is told what its
// requests come to, from whichever goroutine made it so and while the
// store is held: it must only take note.
func (s *Site) Client(notify func(Event)) *Client {
	return &Client{site: s, notify: notify, conns: map[uint64]*conn{}, txs: map[uint64]*Tx{}}
}

// Site is the site the client is a document of.
func (c *Client) Site() *Site { return c.site }

// Open asks for a connection to the database of that name at that
// version, 0 for the one it is at: the number of the request, which an
// Event answers.
func (c *Client) Open(name string, version uint64) uint64 { return c.ask(name, version, false) }

// Delete asks for the database of that name to be deleted.
func (c *Client) Delete(name string) uint64 { return c.ask(name, 0, true) }

func (c *Client) ask(name string, version uint64, remove bool) uint64 {
	s := c.site
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	q := &request{id: s.store.next(), client: c, name: name, version: version, remove: remove}
	s.queue[name] = append(s.queue[name], q)
	s.advance(name)
	return q.id
}

// Databases are the names and the versions of the site's databases.
func (c *Client) Databases() []Info {
	all := c.site.Databases()
	for i := range all {
		all[i].Stores = nil
	}
	return all
}

// Heard says the connection was told of a VersionChange.
func (c *Client) Heard(id uint64) {
	s := c.site
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if cn := c.conns[id]; cn != nil {
		cn.owes = nil
		s.advance(cn.db.name)
	}
}

// Shut closes a connection, once its transactions have finished.
func (c *Client) Shut(id uint64) {
	s := c.site
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	cn := c.conns[id]
	if cn == nil {
		return
	}
	cn.closing = true
	if !cn.busy() {
		cn.close()
		s.advance(cn.db.name)
	}
}

// Close ends the document: its transactions are undone, its
// connections closed and its requests forgotten.
func (c *Client) Close() {
	s := c.site
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	c.notify = func(Event) {}
	for _, tx := range c.txs {
		tx.abort()
	}
	names := map[string]bool{}
	for _, cn := range c.conns {
		cn.close()
		names[cn.db.name] = true
	}
	for name, queue := range s.queue {
		s.queue[name] = slices.DeleteFunc(queue, func(q *request) bool { return q.client == c })
		names[name] = true
	}
	for name := range names {
		s.advance(name)
	}
}

// advance answers the requests for a database as far as they can be.
func (s *Site) advance(name string) {
	for len(s.queue[name]) > 0 && s.proceed(s.queue[name][0]) {
		s.queue[name] = s.queue[name][1:]
	}
	if len(s.queue[name]) > 0 {
		return
	}
	delete(s.queue, name)
	if db := s.dbs[name]; db != nil && db.version == 0 && len(db.conns) == 0 {
		delete(s.dbs, name)
		delete(s.byID, db.id)
	}
}

// proceed takes a request as far as it can go, and reports whether it
// was answered.
func (s *Site) proceed(q *request) bool {
	db := s.dbs[q.name]
	if q.state == asked {
		if s.answer(q, db) {
			return true
		}
		db = s.dbs[q.name]
		for _, c := range db.conns {
			if !c.closing {
				c.owes = q
				c.client.notify(Event{Kind: VersionChange, Conn: c.id, Old: db.version, New: q.version})
			}
		}
		q.state = acking
	}
	if q.state == acking {
		if slices.ContainsFunc(db.conns, func(c *conn) bool { return c.owes == q }) {
			return false
		}
		if len(db.conns) > 0 {
			q.client.notify(Event{Kind: Blocked, Request: q.id, Old: db.version, New: q.version})
		}
		q.state = closing
	}
	if q.state != closing || len(db.conns) > 0 {
		return false
	}
	if q.remove {
		s.drop(db)
		q.client.notify(Event{Kind: Deleted, Request: q.id, Old: db.version})
		return true
	}
	s.upgrade(q, db)
	return false
}

// answer answers a request that needs no other connection to close,
// and reports whether it did.
func (s *Site) answer(q *request, db *database) bool {
	switch {
	case db == nil && q.remove:
		q.client.notify(Event{Kind: Deleted, Request: q.id})
		return true
	case q.remove:
		return false
	case db == nil:
		db = s.create(q.name)
		q.created = true
	}
	if q.version == 0 {
		q.version = max(db.version, 1)
	}
	switch {
	case q.version < db.version:
		q.client.notify(Event{Kind: Failed, Request: q.id, Err: failed("VersionError",
			"The requested version is less than the existing version.")})
		return true
	case q.version == db.version:
		c := db.connect(q.client)
		q.client.notify(Event{Kind: Opened, Request: q.id, Conn: c.id, Database: db.info()})
		return true
	}
	return false
}

// upgrade begins the transaction that takes a database to the version
// a request asked for.
func (s *Site) upgrade(q *request, db *database) {
	c := db.connect(q.client)
	tx := &Tx{id: s.store.next(), client: q.client, conn: c, db: db, mode: ChangeVersion, started: true, request: q}
	db.txs = append(db.txs, tx)
	q.client.txs[tx.id] = tx
	old := db.version
	db.version = q.version
	tx.undo = append(tx.undo, func() { db.version = old })
	tx.redo.op(opDatabase, db.id)
	tx.redo.s(db.name)
	tx.redo.u(db.version)
	q.state = upgrading
	q.client.notify(Event{Kind: Upgrade, Request: q.id, Conn: c.id, Tx: tx.id, Old: old, New: q.version,
		Database: db.info()})
}

// upgraded answers the request whose upgrade ended.
func (s *Site) upgraded(tx *Tx, kept bool) {
	q := tx.request
	if kept {
		q.client.notify(Event{Kind: Opened, Request: q.id, Conn: tx.conn.id, Database: tx.db.info()})
	} else {
		if !tx.conn.closing {
			tx.conn.close()
		}
		q.client.notify(Event{Kind: Failed, Request: q.id, Err: failed("AbortError",
			"The transaction that upgraded the database was aborted.")})
	}
	if queue := s.queue[q.name]; len(queue) > 0 && queue[0] == q {
		s.queue[q.name] = queue[1:]
	}
}
