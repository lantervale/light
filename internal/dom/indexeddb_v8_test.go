//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/idb"
)

// tools are what the pages of these tests share: a request and a
// transaction as promises, what a call throws by its name, and the
// lines the test reads once main ended.
const tools = `
	window.out = [];
	const say = (...parts) => { out.push(parts.map(String).join(" ")); };
	const show = (value) => JSON.stringify(value, (_, v) => v instanceof ArrayBuffer
		? "bytes " + Array.from(new Uint8Array(v)).join(",") : v === undefined ? "undefined" : v);
	const got = (request) => new Promise((resolve, reject) => {
		request.onsuccess = () => resolve(request.result);
		request.onerror = (event) => { event.preventDefault(); reject(request.error); };
	});
	const over = (tx) => new Promise((resolve, reject) => {
		tx.addEventListener("complete", () => resolve("complete"));
		tx.addEventListener("abort", () => reject(tx.error || new DOMException("aborted", "AbortError")));
	});
	const opened = (name, version, upgrade) => new Promise((resolve, reject) => {
		const request = indexedDB.open(name, version);
		request.onupgradeneeded = (event) => upgrade && upgrade(request.result, request.transaction, event);
		request.onsuccess = () => resolve(request.result);
		request.onerror = (event) => { event.preventDefault(); reject(request.error); };
		request.onblocked = () => say("blocked");
	});
	const thrown = (call) => { try { call(); return "nothing"; } catch (error) { return error.name; } };
	const failing = (promise) => promise.then(() => "nothing", (error) => error ? error.name : "null");
	const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
	const begin = () => main().then(() => { window.ended = true; }, (error) => {
		say("FAILED", error && error.name, error && error.message, error && error.stack);
		window.ended = true;
	});
`

// based runs a page whose script defines main, with the databases of
// site, until main ended: the lines it said.
func based(t *testing.T, site *idb.Site, script string) (*scripted, []string) {
	t.Helper()
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.start("<body><script>"+tools+script+"\nbegin();</script>", dom.Host{Bases: func() *idb.Site { return site }})
	return s, s.lines()
}

// lines are what the page said once its main ended.
func (s *scripted) lines() []string {
	s.t.Helper()
	for end := time.Now().Add(5 * time.Second); s.eval("String(window.ended)") != "true"; {
		if time.Now().After(end) {
			s.t.Fatalf("the page never ended; it said %s, and its console %s", s.eval("out.join('\\n')"), s.said())
		}
		s.rt.Frame()
	}
	return strings.Split(s.eval(`out.join("\n")`), "\n")
}

// says checks the lines a page said.
func says(t *testing.T, got []string, want ...string) {
	t.Helper()
	for i := range max(len(got), len(want)) {
		switch {
		case i >= len(got):
			t.Errorf("line %d is missing: %s", i+1, want[i])
		case i >= len(want):
			t.Errorf("line %d is one too many: %s", i+1, got[i])
		case got[i] != want[i]:
			t.Errorf("line %d is\n%s, not\n%s", i+1, got[i], want[i])
		}
	}
}

func TestADatabaseOpensAndUpgrades(t *testing.T) {
	_, got := based(t, nil, `async function main() {
		const first = indexedDB.open("db");
		say("at once", first.readyState, thrown(() => first.result), thrown(() => first.error), first.transaction,
			first.source, first instanceof IDBOpenDBRequest, first instanceof IDBRequest,
				first instanceof EventTarget);
		first.onupgradeneeded = (event) => {
			const db = first.result, tx = first.transaction;
			say("upgradeneeded", event.constructor.name, event.oldVersion, event.newVersion, first.readyState,
				db.version,
				db.name, tx.mode, tx.db === db, event.target === first);
			const store = db.createObjectStore("books", { keyPath: "isbn" });
			say("store", store.name, store.keyPath, store.autoIncrement, store.transaction === tx,
				show(Array.from(db.objectStoreNames)),
				show(Array.from(tx.objectStoreNames)), tx.objectStore("books") === store);
			say("again", thrown(() => db.createObjectStore("books")), thrown(() => db.createObjectStore("x",
				{ keyPath: "a b" })),
				thrown(() => db.createObjectStore("x", { keyPath: [] })),
				thrown(() => db.createObjectStore("x", { keyPath: "", autoIncrement: true })),
				thrown(() => db.createObjectStore("x", { keyPath: ["a"], autoIncrement: true })),
				thrown(() => db.deleteObjectStore("nope")), thrown(() => db.transaction("books")));
			db.createObjectStore("gone");
			db.deleteObjectStore("gone");
			db.createObjectStore("b", { keyPath: ["x", "y.z"] });
			tx.oncomplete = () => say("upgrade complete", thrown(() => db.createObjectStore("late")));
		};
		const db = await got(first);
		say("success", first.readyState, first.result === db, first.error, first.transaction, db.version,
			show(Array.from(db.objectStoreNames)), db.objectStoreNames.contains("books"), db.objectStoreNames.item(1),
			db.objectStoreNames.length, db.objectStoreNames instanceof DOMStringList);
		say("outside an upgrade", thrown(() => db.createObjectStore("x")),
			thrown(() => db.deleteObjectStore("books")));
		say("transactions", thrown(() => db.transaction("nope")), thrown(() => db.transaction([])),
			thrown(() => db.transaction("books", "versionchange")), thrown(() => db.transaction("books", "readonly",
				{ durability: "x" })),
			db.transaction(["books", "b", "books"], "readwrite", { durability: "strict" }).durability,
			show(Array.from(db.transaction(new Set(["books", "b"])).objectStoreNames)), db.transaction("b").mode);
		say("versions", thrown(() => indexedDB.open("db", 0)), thrown(() => indexedDB.open("db", -1)),
			thrown(() => indexedDB.open("db", NaN)), thrown(() => indexedDB.open()), await failing(opened("db",
				1.9).then((db) => db.close())));
		db.close();
		say("closed", thrown(() => db.transaction("books")));
		const second = await opened("db", 3, (db, tx, event) => say("upgrade", event.oldVersion, event.newVersion,
			show(Array.from(db.objectStoreNames))));
		say("reopened", second.version, await failing(opened("db", 2)), await failing(opened("db",
			3).then((db) => db.close())));
		say("listed", show(await indexedDB.databases()));
		second.close();
		const removal = indexedDB.deleteDatabase("db");
		removal.onsuccess = (event) => say("deleted", event.constructor.name, event.oldVersion, event.newVersion,
			removal.result);
		await new Promise((resolve) => removal.addEventListener("success", resolve));
		say("listed", show(await indexedDB.databases()), indexedDB.cmp(1, "a"), indexedDB.cmp([2], [1, 9]),
			indexedDB.cmp(new Date(0), new Date(0)),
			thrown(() => indexedDB.cmp(NaN, 1)), indexedDB instanceof IDBFactory, thrown(() => new IDBFactory()));
		const none = indexedDB.deleteDatabase("never");
		none.onsuccess = (event) => say("deleted none", event.oldVersion, event.newVersion);
		await new Promise((resolve) => none.addEventListener("success", resolve));
	}`)
	says(t, got,
		"at once pending InvalidStateError InvalidStateError null null true true true",
		"upgradeneeded IDBVersionChangeEvent 0 1 done 1 db versionchange true true",
		`store books isbn false true ["books"] ["books"] true`,
		"again ConstraintError SyntaxError SyntaxError InvalidAccessError InvalidAccessError NotFoundError InvalidStateError",
		"upgrade complete InvalidStateError",
		`success done true null null 1 ["b","books"] true books 2 true`,
		"outside an upgrade InvalidStateError InvalidStateError",
		`transactions NotFoundError InvalidAccessError TypeError TypeError strict ["b","books"] readonly`,
		"versions TypeError TypeError TypeError TypeError nothing",
		"closed InvalidStateError",
		`upgrade 1 3 ["b","books"]`,
		"reopened 3 VersionError nothing",
		`listed [{"name":"db","version":3}]`,
		"deleted IDBVersionChangeEvent 3 null undefined",
		"listed [] -1 1 0 DataError true TypeError",
		"deleted none 0 null",
	)
}

func TestRecordsAreStoredAndRead(t *testing.T) {
	_, got := based(t, nil, `async function main() {
		const db = await opened("db", 1, (db) => {
			db.createObjectStore("plain");
			db.createObjectStore("inline", { keyPath: "a.b" });
			db.createObjectStore("pair", { keyPath: ["x", "y"] });
			db.createObjectStore("made", { autoIncrement: true });
			db.createObjectStore("madeInline", { keyPath: "at.id", autoIncrement: true });
		});
		const tx = db.transaction(db.objectStoreNames, "readwrite");
		const plain = tx.objectStore("plain");
		const put = plain.put("one", 1);
		say("a request", put.readyState, put.source === plain, put.transaction === tx, put instanceof IDBRequest);
		say("put", await got(put), await got(plain.add({ deep: [1, { two: 2 }] }, "k")), await got(plain.put("uno",
			1)));
		say("add again", await failing(got(plain.add("x", 1))));
		for (const key of [new Date(5), [1, "a"], new Uint8Array([1, 0, 255]).buffer, -Infinity, "", [], 0.5,
			new Date(-1),
			new Uint16Array([513]), [[]], "b", "ab"]) await got(plain.add(null, key));
		const keys = (list) => show(list.map((key) => key instanceof Date ? "date " + key.getTime() : key));
		say("keys", keys(await got(plain.getAllKeys())));
		say("get", await got(plain.get(1)), show(await got(plain.get("k"))), await got(plain.get("none")),
			await got(plain.getKey("k")),
			await got(plain.getKey(9)), await got(plain.count()), await got(plain.count(IDBKeyRange.bound(0, 1))),
			show(await got(plain.getAll(IDBKeyRange.lowerBound("a"), 2))), keys(await got(plain.getAllKeys(null,
				3))));
		say("get a range", await got(plain.get(IDBKeyRange.lowerBound(0.6))),
			await got(plain.getKey(IDBKeyRange.upperBound(0.7, true))));
		for (const key of [NaN, {}, null, true, new Date(NaN), [1, , 2], [[NaN]], () => 1, /x/]) {
			if (thrown(() => plain.put(1, key)) !== "DataError") say("a key that is none was taken", String(key));
		}
		const ring = [];
		ring.push(ring);
		say("no key", thrown(() => plain.put(1)), thrown(() => plain.get()), thrown(() => plain.get(null)),
			thrown(() => plain.delete(undefined)),
			thrown(() => plain.put(1, ring)), thrown(() => plain.getAll(null, -1)), thrown(() => plain.put()));
		const twice = [1];
		say("an array twice", show(await got(plain.put("twice", [twice, twice]))));
		say("delete", await got(plain.delete(IDBKeyRange.bound(-Infinity, new Date(9)))),
			keys(await got(plain.getAllKeys())),
			await got(plain.delete("none")));
		say("clear", await got(plain.clear()), await got(plain.count()));

		const inline = tx.objectStore("inline");
		say("inline", await got(inline.put({ a: { b: 7 }, c: 1 })), show(await got(inline.get(7))),
			thrown(() => inline.put({ a: {} })),
			thrown(() => inline.put({ a: { b: 7 } }, 7)), thrown(() => inline.put({ a: { b: {} } })),
				thrown(() => inline.put(3)));
		const pair = tx.objectStore("pair");
		say("pair", show(await got(pair.put({ x: 1, y: "b" }))), show(await got(pair.put({ x: 1, y: "a" }))),
			show(pair.keyPath),
			show((await got(pair.getAll())).map((v) => v.y)), thrown(() => pair.put({ x: 1 })),
				show(await got(pair.get([1, "b"]))));
		say("lengths", await got(tx.objectStore("pair").count(IDBKeyRange.only([1, "a"]))));

		const made = tx.objectStore("made");
		say("made", await got(made.add("a")), await got(made.add("b")), await got(made.add("c", 10)),
			await got(made.add("d")),
			await got(made.put("e", 3.7)), await got(made.add("f")), await got(made.add("g", "text")),
				await got(made.add("h")),
			await got(made.put("i", 20.2)), await got(made.add("j")), await failing(got(made.add("again", 2))),
				await got(made.add("k")),
			made.autoIncrement, made.keyPath);
		const madeInline = tx.objectStore("madeInline");
		say("made inline", await got(madeInline.add({ n: "a" })), show(await got(madeInline.get(1))),
			await got(madeInline.add({ at: { id: 9 } })),
			await got(madeInline.add({ at: { more: 1 } })), show(await got(madeInline.get(10))),
				thrown(() => madeInline.add({ at: 5 })),
			thrown(() => madeInline.add("text")), await got(madeInline.add({ at: {} })));
		await over(tx);

		const reader = db.transaction("plain");
		say("read only", reader.mode, thrown(() => reader.objectStore("plain").put(1, 1)),
			thrown(() => reader.objectStore("plain").clear()),
			thrown(() => reader.objectStore("plain").delete(1)), thrown(() => reader.objectStore("made")),
				await got(reader.objectStore("plain").count()));
	}`)
	says(t, got,
		"a request pending true true true",
		"put 1 k 1",
		"add again ConstraintError",
		`keys [null,0.5,1,"date -1","date 5","","ab","b","k","bytes 1,0,255","bytes 1,2",[],[1,"a"],[[]]]`,
		`get uno {"deep":[1,{"two":2}]} undefined k undefined 14 2 [null,null] [null,0.5,1]`,
		"get a range uno -Infinity",
		"no key DataError TypeError DataError DataError DataError TypeError TypeError",
		"an array twice [[1],[1]]",
		`delete undefined ["","ab","b","k","bytes 1,0,255","bytes 1,2",[],[1,"a"],[[]],[[1],[1]]] undefined`,
		"clear undefined 0",
		`inline 7 {"a":{"b":7},"c":1} DataError DataError DataError DataError`,
		`pair [1,"b"] [1,"a"] ["x","y"] ["a","b"] DataError {"x":1,"y":"b"}`,
		"lengths 1",
		"made 1 2 10 11 3.7 12 text 13 20.2 21 ConstraintError 22 true null",
		`made inline 1 {"n":"a","at":{"id":1}} 9 10 {"at":{"more":1,"id":10}} DataError DataError 11`,
		"read only readonly ReadOnlyError ReadOnlyError ReadOnlyError NotFoundError 0",
	)
}

func TestValuesAreStoredAsTheirStructuredClones(t *testing.T) {
	_, got := based(t, nil, `async function main() {
		const db = await opened("db", 1, (db) => db.createObjectStore("s"));
		const tx = db.transaction("s", "readwrite"), s = tx.objectStore("s");
		const shared = { n: 1 }, ring = { shared, also: shared };
		ring.self = ring;
		const sparse = [1, , 3];
		sparse.extra = "x";
		const bytes = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]);
		const value = {
			ring, sparse, text: "héllo \u{1F600} \ud800", zero: -0, nan: NaN, big: 12345678901234567890n,
				none: undefined, nil: null,
			yes: true, date: new Date(86400000), expression: /a+/gi, map: new Map([[ring, new Set([1, "two"])]]),
				boxed: new Number(4),
			view: new Uint16Array(bytes.buffer, 2, 2), data: new DataView(bytes.buffer, 1, 3), buffer: bytes.buffer,
			floats: new Float64Array([1.5, -2]), error: new RangeError("too far", { cause: "a cause" }),
				exception: new DOMException("no", "NotFoundError"),
			blob: new Blob(["blob ", "bytes"], { type: "text/plain" }), file: new File([new Uint8Array([0, 255])],
				"a.bin", { type: "x/y", lastModified: 7 }),
			long: "x".repeat(70000),
		};
		await got(s.put(value, 1));
		value.text = "changed after the put";
		const back = await got(s.get(1));
		say("objects", back !== value, back.ring.self === back.ring, back.ring.shared === back.ring.also,
			back.map.keys().next().value === back.ring,
			show(Array.from(back.map.get(back.ring))), show(back.sparse), 1 in back.sparse, back.sparse.length,
				back.sparse.extra);
		say("values", back.text === "héllo \u{1F600} \ud800", Object.is(back.zero, -0), Number.isNaN(back.nan),
			back.big, "none" in back, back.none,
			back.nil, back.yes, back.date instanceof Date, back.date.getTime(), String(back.expression),
				back.boxed instanceof Number, back.boxed + 1);
		say("bytes", back.view instanceof Uint16Array, back.view.byteOffset, show(Array.from(back.view)),
			back.view.buffer === back.buffer,
			back.data.buffer === back.buffer, back.data.getUint8(0), back.data.byteLength,
				show(Array.from(new Uint8Array(back.buffer))),
			show(Array.from(back.floats)), back.long.length);
		say("errors", back.error instanceof RangeError, back.error.message, back.error.cause,
			back.exception instanceof DOMException,
			back.exception.name, back.exception.message);
		say("blobs", back.blob instanceof Blob, back.blob.type, await back.blob.text(), back.file instanceof File,
			back.file.name, back.file.type,
			back.file.lastModified, show(Array.from(new Uint8Array(await back.file.arrayBuffer()))));
		const again = await got(db.transaction("s").objectStore("s").get(1));
		say("anew each time", again !== back, again.ring !== back.ring);
		const late = db.transaction("s", "readwrite"), t = late.objectStore("s");
		say("not cloned", thrown(() => t.put(() => 1, 2)), thrown(() => t.put({ node: document.body }, 2)),
			thrown(() => t.put(Symbol("s"), 2)),
			thrown(() => t.put({ deep: [new WeakMap()] }, 2)), thrown(() => t.put(window, 2)),
			thrown(() => t.put(new Proxy({}, { ownKeys() { throw new RangeError("mine"); } }), 2)),
				await got(t.count()));
		let seen = "never read";
		await got(t.put({ get lazy() { seen = thrown(() => t.get(1)); return 5; } }, 3));
		say("a getter", seen, show(await got(t.get(3))));
		const all = await got(t.getAll());
		say("all", all.length, all[0].long.length, show(await got(t.getAllKeys())));
	}`)
	says(t, got,
		`objects true true true true [1,"two"] [1,"undefined",3] false 3 x`,
		"values true true true 12345678901234567890 true undefined null true true 86400000 /a+/gi true 5",
		"bytes true 2 [1027,1541] true true 2 3 [1,2,3,4,5,6,7,8] [1.5,-2] 70000",
		"errors true too far a cause true NotFoundError no",
		"blobs true text/plain blob bytes true a.bin x/y 7 [0,255]",
		"anew each time true true",
		"not cloned DataCloneError DataCloneError DataCloneError DataCloneError DataCloneError RangeError 1",
		`a getter TransactionInactiveError {"lazy":5}`,
		"all 2 70000 [1,3]",
	)
}

func TestIndexesListRecords(t *testing.T) {
	_, got := based(t, nil, `async function main() {
		let db = await opened("db", 1, (db, tx) => {
			const people = db.createObjectStore("people", { keyPath: "id", autoIncrement: true });
			const name = people.createIndex("name", "name", { unique: true });
			const tags = people.createIndex("tags", "tags", { multiEntry: true });
			people.createIndex("both", ["city", "age"]);
			people.createIndex("size", "name.length");
			say("made", name.name, name.keyPath, name.unique, name.multiEntry, tags.multiEntry,
				name.objectStore === people,
				show(Array.from(people.indexNames)), people.index("name") === name,
					show(people.index("both").keyPath));
			say("refused", thrown(() => people.createIndex("name", "x")), thrown(() => people.createIndex("y",
				"a b")),
				thrown(() => people.createIndex("y", ["a", "b"], { multiEntry: true })),
					thrown(() => people.index("nope")),
				thrown(() => people.deleteIndex("nope")), thrown(() => people.createIndex("y")));
		});
		const tx = db.transaction("people", "readwrite"), people = tx.objectStore("people");
		say("outside an upgrade", thrown(() => people.createIndex("z", "z")),
			thrown(() => people.deleteIndex("name")),
			thrown(() => { people.name = "folk"; }), thrown(() => { people.index("name").name = "called"; }));
		for (const person of [
			{ name: "ada", tags: ["x", "y", "x"], city: "lyon", age: 36 }, { name: "bob", tags: ["y", {}, 3],
				city: "lyon", age: 20 },
			{ name: "cyd", tags: "y", city: "nancy", age: 36 }, { name: "dee", city: "lyon" }, { name: 5, tags: [] },
		]) await got(people.add(person));
		const name = people.index("name"), tags = people.index("tags"), both = people.index("both"),
			size = people.index("size");
		say("unique", await failing(got(people.add({ name: "ada" }))), await got(people.put({ id: 1, name: "ada",
			tags: ["x", "y"], city: "lyon", age: 36 })),
			await failing(got(people.put({ id: 2, name: "ada" }))), await got(people.count()));
		say("by name", (await got(name.get("bob"))).id, await got(name.getKey("cyd")), await got(name.get("nobody")),
			await got(name.count()),
			show(await got(name.getAllKeys())), show((await got(name.getAll(IDBKeyRange.bound("b", "d", false,
				true)))).map((p) => p.name)));
		say("by tag", show(await got(tags.getAllKeys())), show(await got(tags.getAllKeys("y"))),
			await got(tags.count("x")), await got(tags.count()),
			(await got(tags.get(3))).name, show(await got(tags.getAllKeys(null, 2))));
		say("by city and age", show(await got(both.getAllKeys())),
			show(await got(both.getAllKeys(IDBKeyRange.bound(["lyon"], ["lyon", []])))),
			await got(both.getKey(["nancy", 36])), show(await got(size.getAllKeys(3))));
		await got(people.delete(2));
		say("after a delete", show(await got(tags.getAllKeys())), await got(name.count()));
		await over(tx);
		db.close();

		db = await opened("db", 2, (db, tx) => {
			const people = tx.objectStore("people"), name = people.index("name");
			people.createIndex("age", "age");
			people.deleteIndex("tags");
			name.name = "called";
			people.name = "folk";
			say("renamed", people.name, name.name, show(Array.from(people.indexNames)),
				show(Array.from(db.objectStoreNames)),
				thrown(() => tx.objectStore("people")), tx.objectStore("folk") === people,
					thrown(() => { name.name = "age"; }),
				thrown(() => people.index("tags")));
			people.index("age").count().onsuccess = (event) => say("filled", event.target.result);
			const gone = people.createIndex("gone", "city");
			people.deleteIndex("gone");
			say("deleted", thrown(() => gone.count()), thrown(() => gone.openCursor()));
		});
		const reading = db.transaction("folk"), folk = reading.objectStore("folk");
		say("upgraded", show(Array.from(folk.indexNames)), show(await got(folk.index("age").getAllKeys())),
			await got(folk.index("called").getKey("ada")));
		await over(reading);
		db.close();

		say("a rule the records break", await failing(opened("db", 3, (db, tx) => {
			tx.objectStore("folk").createIndex("city", "city", { unique: true });
			tx.onabort = () => say("aborted", tx.error.name);
		})));
		db = await opened("db");
		say("left as it was", db.version, show(Array.from(db.transaction("folk").objectStore("folk").indexNames)));
	}`)
	says(t, got,
		`made name name true false true true ["both","name","size","tags"] true ["city","age"]`,
		"refused ConstraintError SyntaxError InvalidAccessError NotFoundError NotFoundError TypeError",
		"outside an upgrade InvalidStateError InvalidStateError InvalidStateError InvalidStateError",
		"unique ConstraintError 1 ConstraintError 5",
		`by name 2 3 undefined 5 [5,1,2,3,4] ["bob","cyd"]`,
		"by tag [2,1,1,2,3] [1,2,3] 1 5 bob [2,1]",
		"by city and age [2,1,3] [2,1] 3 [1,2,3,4]",
		"after a delete [1,1,3] 4",
		`renamed folk called ["age","both","called","size"] ["folk"] NotFoundError true ConstraintError NotFoundError`,
		"deleted InvalidStateError InvalidStateError",
		"filled 2",
		`upgraded ["age","both","called","size"] [1,3] 1`,
		"aborted ConstraintError",
		"a rule the records break AbortError",
		`left as it was 2 ["age","both","called","size"]`,
	)
}

func TestCursorsGoThroughRecords(t *testing.T) {
	_, got := based(t, nil, `async function main() {
		const db = await opened("db", 1, (db) => {
			const s = db.createObjectStore("s", { keyPath: "id" });
			s.createIndex("letter", "letter");
			for (const [id, letter] of [[1, "a"], [2, "a"], [3, "b"], [4, "b"], [5, "b"], [6, "c"]]) s.add({ id,
				letter });
			db.createObjectStore("loose");
		});
		// walk goes through a cursor: what each record shows, and how the cursor moves on from it.
		const walk = (request, each = (cursor) => cursor.continue()) => new Promise((resolve, reject) => {
			const seen = [];
			request.onsuccess = () => {
				const cursor = request.result;
				if (!cursor) return resolve(seen.join(" "));
				seen.push(cursor.key + ":" + cursor.primaryKey);
				try { each(cursor, seen); } catch (error) { reject(error); }
			};
			request.onerror = () => reject(request.error);
		});
		const tx = db.transaction(["s", "loose"], "readwrite"), s = tx.objectStore("s"), letter = s.index("letter");
		say("a store", await walk(s.openCursor()), "/", await walk(s.openCursor(null, "prev")), "/",
			await walk(s.openCursor(IDBKeyRange.bound(2, 5, true, true), "nextunique")),
			"/", await walk(s.openKeyCursor(IDBKeyRange.upperBound(3), "prevunique")), "/",
				await walk(s.openCursor(9)) === "");
		say("an index", await walk(letter.openCursor()), "/", await walk(letter.openCursor(null, "nextunique")), "/",
			await walk(letter.openCursor(null, "prev")),
			"/", await walk(letter.openKeyCursor(null, "prevunique")), "/", await walk(letter.openCursor("b")), "/",
				await walk(letter.openCursor(IDBKeyRange.only("b"), "prevunique")));
		say("moves", await walk(s.openCursor(), (c) => c.advance(2)), "/", await walk(s.openCursor(null, "prev"),
			(c) => c.advance(4)), "/",
			await walk(s.openCursor(), (c) => c.key === 1 ? c.continue(4.5) : c.continue()), "/",
			await walk(letter.openCursor(null, "prev"), (c) => c.primaryKey === 6 ? c.continue("a") : c.continue()),
				"/",
			await walk(letter.openCursor(), (c) => c.primaryKey === 1 ? c.continuePrimaryKey("b", 4) : c.continue()),
				"/",
			await walk(letter.openCursor(null, "prev"), (c) => c.primaryKey === 6 ? c.continuePrimaryKey("b",
				4) : c.continue()), "/",
			await walk(letter.openCursor(null, "nextunique"), (c) => c.advance(2)));

		const first = s.openCursor(2);
		say("a cursor", first.readyState, first.source === s);
		const cursor = await got(first);
		say("on a record", cursor instanceof IDBCursorWithValue, cursor instanceof IDBCursor, cursor.source === s,
			cursor.direction, cursor.request === first,
			cursor.key, cursor.primaryKey, show(cursor.value), cursor.value === cursor.value,
				first.result === cursor);
		say("refused", thrown(() => cursor.advance(0)), thrown(() => cursor.advance(-1)),
			thrown(() => cursor.continue(2)), thrown(() => cursor.continue(1)),
			thrown(() => cursor.continue({})), thrown(() => cursor.continuePrimaryKey(3, 3)),
				thrown(() => cursor.update({ id: 3 })), thrown(() => cursor.update({})));
		cursor.continue();
		say("moving", first.readyState, thrown(() => cursor.continue()), thrown(() => cursor.advance(1)),
			thrown(() => cursor.update({ id: 2 })),
			thrown(() => cursor.delete()), cursor.key);
		say("moved", (await got(first)) === null, first.readyState);

		const keys = await got(letter.openKeyCursor("c"));
		say("keys alone", keys instanceof IDBCursorWithValue, "value" in keys, keys.key, keys.primaryKey,
			keys.source === letter, thrown(() => keys.update({})),
			thrown(() => keys.delete()), thrown(() => keys.continuePrimaryKey("c", 6)),
				thrown(() => keys.continuePrimaryKey("a", 1)));
		const unique = await got(letter.openCursor(null, "nextunique"));
		say("by keys", thrown(() => unique.continuePrimaryKey("b", 3)), thrown(() => cursor.continuePrimaryKey(4,
			4)));

		say("changes", await walk(letter.openCursor("b"), (c) => {
			if (c.primaryKey === 4) c.delete().onsuccess = (event) => say("deleted", event.target.result,
				event.target.source === c);
			else c.update({ ...c.value, seen: true }).onsuccess = (event) => say("updated", event.target.result);
			c.continue();
		}));
		say("changed", show(await got(s.getAll(IDBKeyRange.bound(3, 5)))));
		const loose = tx.objectStore("loose");
		await got(loose.put("a", 1));
		say("without a key path", await walk(loose.openCursor(), (c) => { c.update("b"); c.continue(); }),
			await got(loose.get(1)));
		say("deleted under it", await walk(s.openCursor(), (c) => { if (c.key === 1) s.delete(IDBKeyRange.bound(2,
			5)); c.continue(); }));
		await over(tx);

		const reader = db.transaction("s"), read = await got(reader.objectStore("s").openCursor());
		say("read only", thrown(() => read.update({ id: 1 })), thrown(() => read.delete()),
			thrown(() => reader.objectStore("s").openCursor(null, "sideways")));
		await over(reader);
		say("finished", thrown(() => read.continue()), thrown(() => read.advance(1)),
			thrown(() => read.update({ id: 1 })), read.key);
	}`)
	says(t, got,
		"a store 1:1 2:2 3:3 4:4 5:5 6:6 / 6:6 5:5 4:4 3:3 2:2 1:1 / 3:3 4:4 / 3:3 2:2 1:1 / true",
		"an index a:1 a:2 b:3 b:4 b:5 c:6 / a:1 b:3 c:6 / c:6 b:5 b:4 b:3 a:2 a:1 / c:6 b:3 a:1 / b:3 b:4 b:5 / b:3",
		"moves 1:1 3:3 5:5 / 6:6 2:2 / 1:1 5:5 6:6 / c:6 a:2 a:1 / a:1 b:4 b:5 c:6 / c:6 b:4 b:3 a:2 a:1 / a:1 c:6",
		"a cursor pending true",
		`on a record true true true next true 2 2 {"id":2,"letter":"a"} true true`,
		"refused TypeError TypeError DataError DataError DataError InvalidAccessError DataError DataError",
		"moving pending InvalidStateError InvalidStateError InvalidStateError InvalidStateError 2",
		"moved true done",
		"keys alone false false c 6 true InvalidStateError InvalidStateError DataError DataError",
		"by keys InvalidAccessError InvalidAccessError",
		"updated 3",
		"deleted undefined true",
		"updated 5",
		"changes b:3 b:4 b:5",
		`changed [{"id":3,"letter":"b","seen":true},{"id":5,"letter":"b","seen":true}]`,
		"without a key path 1:1 b",
		"deleted under it 1:1 6:6",
		"read only ReadOnlyError ReadOnlyError TypeError",
		"finished TransactionInactiveError TransactionInactiveError TransactionInactiveError 1",
	)
}

func TestATransactionIsActiveInItsCallbacksAlone(t *testing.T) {
	_, got := based(t, nil, `async function main() {
		const db = await opened("db", 1, (db) => { db.createObjectStore("s"); db.createObjectStore("t"); });
		const order = [];
		const tx = db.transaction("s", "readwrite"), s = tx.objectStore("s");
		tx.oncomplete = () => order.push("complete");
		const put = s.put("a", 1);
		put.onsuccess = () => order.push("success");
		order.push("asked " + put.readyState);
		await Promise.resolve();
		await null;
		say("after microtasks", thrown(() => s.put("b", 2)), order.join());
		await got(s.get(1));
		await Promise.resolve();
		await new Promise((resolve) => queueMicrotask(resolve));
		say("in a callback, after microtasks", thrown(() => s.put("c", 3)));
		await got(s.get(3));
		await tick();
		say("after a task", thrown(() => s.put("d", 4)), thrown(() => s.get(1)), thrown(() => s.openCursor()),
			thrown(() => tx.objectStore("s").count()),
			thrown(() => tx.commit()), order.join());
		say("finished", thrown(() => tx.objectStore("s")), thrown(() => tx.abort()), tx.error,
			show(await got(db.transaction("s").objectStore("s").getAllKeys())));

		const empty = db.transaction("s");
		say("no request", await over(empty), thrown(() => empty.objectStore("s")));

		const waited = db.transaction("s", "readwrite");
		const fetched = fetch("/missing").then((answer) => answer.status);
		say("after a fetch", await fetched, thrown(() => waited.objectStore("s").put("late", 9)));

		const committed = db.transaction("s", "readwrite"), store = committed.objectStore("s");
		const kept = store.put("kept", 5);
		committed.commit();
		say("committing", thrown(() => store.put("after", 6)), thrown(() => committed.commit()),
			thrown(() => committed.abort()), kept.readyState);
		kept.onsuccess = () => say("the request of a commit", thrown(() => store.get(5)));
		say("committed", await over(committed), await got(db.transaction("s").objectStore("s").get(5)));

		const inner = db.transaction("s", "readwrite");
		inner.objectStore("s").get(1).onsuccess = () => {
			const other = db.transaction("t", "readwrite");
			other.objectStore("t").put("made in a callback", 1);
			other.oncomplete = () => say("a transaction made in a callback completes");
			Promise.resolve().then(() => say("then", thrown(() => other.objectStore("t").put("x", 2)),
				thrown(() => inner.objectStore("s").get(1))));
			setTimeout(() => say("a task later", thrown(() => other.objectStore("t").put("x", 3)),
				thrown(() => inner.objectStore("s").get(1))), 0);
		};
		await over(inner);
		await tick();
		await tick();
		say("in order", order.join());
	}`)
	says(t, got,
		"after microtasks nothing asked pending",
		"in a callback, after microtasks nothing",
		"after a task TransactionInactiveError TransactionInactiveError TransactionInactiveError InvalidStateError "+
			"InvalidStateError asked pending,success,complete",
		"finished InvalidStateError InvalidStateError null [1,2,3]",
		"no request complete InvalidStateError",
		"after a fetch 404 InvalidStateError",
		"committing TransactionInactiveError InvalidStateError InvalidStateError pending",
		"the request of a commit TransactionInactiveError",
		"committed complete kept",
		"then nothing nothing",
		"a transaction made in a callback completes",
		"a task later InvalidStateError InvalidStateError",
		"in order asked pending,success,complete",
	)
}

func TestTransactionsOnTheSameStoresRunOneAfterAnother(t *testing.T) {
	_, got := based(t, nil, `async function main() {
		const db = await opened("db", 1, (db) => { db.createObjectStore("s"); db.createObjectStore("t"); });
		const first = db.transaction("s", "readwrite"), second = db.transaction(["s", "t"], "readwrite"),
			apart = db.transaction("t", "readonly");
		const third = db.transaction("s");
		const log = [];
		const note = (text) => () => log.push(text);
		for (const [tx, name] of [[first, "first"], [second, "second"], [apart, "apart"], [third,
			"third"]]) tx.oncomplete = note(name + " complete");
		second.objectStore("s").get(1).onsuccess = (event) => log.push("second reads " + event.target.result);
		second.objectStore("s").put("of second", 1);
		third.objectStore("s").get(1).onsuccess = (event) => log.push("third reads " + event.target.result);
		apart.objectStore("t").count().onsuccess = note("apart counts");
		const one = first.objectStore("s").put("of first", 1);
		one.onsuccess = () => {
			log.push("first puts");
			first.objectStore("s").get(1).onsuccess = () => {
				log.push("first reads");
				first.objectStore("s").put("of first, later", 1).onsuccess = note("first puts again");
			};
		};
		await over(third);
		say(log.join(", "));

		const a = db.transaction("s"), b = db.transaction("s");
		const both = [];
		a.objectStore("s").get(1).onsuccess = () => { both.push("a");
			a.objectStore("s").get(1).onsuccess = () => both.push("a again"); };
		b.objectStore("s").get(1).onsuccess = () => both.push("b");
		await Promise.all([over(a), over(b)]);
		say("two that read:", both.join(", "));
	}`)
	says(t, got,
		"first puts, first reads, first puts again, first complete, second reads of first, later, second complete, "+
			"apart counts, third reads of second, apart complete, third complete",
		"two that read: a, b, a again",
	)
}

func TestAnAbortUndoesATransaction(t *testing.T) {
	_, got := based(t, nil, `async function main() {
		let db = await opened("db", 1, (db) => {
			const s = db.createObjectStore("s", { autoIncrement: true });
			s.createIndex("v", "v", { unique: true });
			s.add({ v: "kept" });
		});
		const tx = db.transaction("s", "readwrite"), s = tx.objectStore("s");
		await got(s.add({ v: "undone" }));
		await got(s.put({ v: "changed" }, 1));
		await got(s.delete(1));
		await got(s.clear());
		const pending = s.add({ v: "never" });
		const events = [];
		pending.onsuccess = () => events.push("success");
		pending.onerror = (event) => events.push("request error " + pending.error.name + " " + (event.target ===
			pending) + " " + event.bubbles + " " + event.cancelable);
		tx.onerror = (event) => events.push("transaction error " + (event.target === pending));
		db.onerror = () => events.push("database error");
		db.onabort = (event) => events.push("database abort " + (event.target === tx));
		const ended = failing(over(tx));
		tx.abort();
		say("aborting", pending.readyState, thrown(() => s.get(1)), thrown(() => tx.abort()),
			thrown(() => tx.commit()), tx.error, events.length);
		say("aborted", await ended, tx.error, events.join(", "), pending.readyState, thrown(() => pending.result),
			thrown(() => tx.objectStore("s")));
		db.onerror = db.onabort = null;
		const after = db.transaction("s", "readwrite").objectStore("s");
		say("left", show(await got(after.getAll())), show(await got(after.index("v").getAllKeys())),
			await got(after.add({ v: "next" })));
		db.close();

		const request = indexedDB.open("db", 2);
		request.onupgradeneeded = () => {
			const db = request.result, tx = request.transaction, s = tx.objectStore("s");
			const made = db.createObjectStore("made"), index = s.createIndex("w", "w");
			s.deleteIndex("v");
			s.name = "renamed";
			s.clear();
			db.deleteObjectStore("renamed");
			tx.onabort = () => say("upgrade aborted", db.version, show(Array.from(db.objectStoreNames)), s.name,
				show(Array.from(s.indexNames)),
				thrown(() => made.get(1)), thrown(() => index.get(1)), request.transaction,
					thrown(() => request.result), request.readyState);
			tx.abort();
			say("in the upgrade", db.version, show(Array.from(db.objectStoreNames)),
				thrown(() => db.createObjectStore("x")), thrown(() => made.put(1, 1)));
		};
		say("the open", await failing(got(request)), request.error.name, thrown(() => request.result),
			request.readyState);
		db = await opened("db");
		const kept = db.transaction("s").objectStore("s");
		say("as before", db.version, show(Array.from(db.objectStoreNames)), show(Array.from(kept.indexNames)),
			show(await got(kept.getAll())),
			show(await got(kept.index("v").getAllKeys())));
		db.close();

		say("a new database", await failing(opened("new", 5, (db, tx) => { db.createObjectStore("s"); tx.abort(); })),
			show(await indexedDB.databases()));
		say("a listener that throws", await failing(opened("thrown", 1, (db, tx) => {
			db.createObjectStore("s");
			tx.onabort = () => say("aborted for", tx.error.name);
			throw new Error("in upgradeneeded");
		})), show(await indexedDB.databases()));

		db = await opened("db");
		const thrower = db.transaction("s", "readwrite");
		thrower.objectStore("s").add({ v: "lost" }).onsuccess = () => { throw new Error("in success"); };
		say("a success that throws", await failing(over(thrower)), thrower.error.name,
			await got(db.transaction("s").objectStore("s").count()));
	}`)
	says(t, got,
		"aborting pending TransactionInactiveError InvalidStateError InvalidStateError null 0",
		"aborted AbortError null request error AbortError true true true, transaction error true, database error, "+
			"database abort true done nothing InvalidStateError",
		`left [{"v":"kept"}] [1] 2`,
		`in the upgrade 1 ["s"] TransactionInactiveError InvalidStateError`,
		`upgrade aborted 1 ["s"] s ["v"] InvalidStateError InvalidStateError null InvalidStateError pending`,
		"the open AbortError AbortError nothing done",
		`as before 1 ["s"] ["v"] [{"v":"kept"},{"v":"next"}] [1,2]`,
		`a new database AbortError [{"name":"db","version":1}]`,
		"aborted for AbortError",
		`a listener that throws AbortError [{"name":"db","version":1}]`,
		"a success that throws AbortError AbortError 2",
	)
}

func TestAnErrorGoesUpAndAbortsUnlessPrevented(t *testing.T) {
	s, got := based(t, nil, `async function main() {
		const db = await opened("db", 1, (db) => db.createObjectStore("s").add("there", 1));
		const events = [];
		db.onerror = (event) => events.push("database " + event.eventPhase + " " + (event.currentTarget === db));
		db.addEventListener("error", (event) => events.push("database capture " + event.eventPhase), true);
		db.onabort = () => events.push("database abort");

		const tx = db.transaction("s", "readwrite");
		const request = tx.objectStore("s").add("again", 1);
		request.onerror = (event) => events.push("request " + request.error.name + " " + event.eventPhase + " " +
			(event.target === request));
		tx.onerror = (event) => events.push("transaction " + (event.target === request) + " " + (event.currentTarget
			=== tx));
		tx.onabort = (event) => events.push("abort " + tx.error.name + " " + (tx.error === request.error) + " " +
			event.bubbles);
		tx.objectStore("s").put("lost", 2);
		say("not prevented", await failing(over(tx)), events.splice(0).join(", "));
		say("undone", show(await got(db.transaction("s").objectStore("s").getAll())));

		const kept = db.transaction("s", "readwrite");
		const refused = kept.objectStore("s").add("again", 1);
		refused.onerror = (event) => { events.push("request"); event.preventDefault();
			kept.objectStore("s").put("after the error", 3); };
		kept.onerror = () => events.push("transaction");
		say("prevented", await over(kept), events.splice(0).join(", "),
			show(await got(db.transaction("s").objectStore("s").getAll())));

		const stopped = db.transaction("s", "readwrite");
		stopped.objectStore("s").add("again",
			1).onerror = (event) => { events.push("request"); event.stopPropagation(); };
		stopped.onerror = () => events.push("transaction");
		say("stopped", await failing(over(stopped)), events.splice(0).join(", "));

		const higher = db.transaction("s", "readwrite");
		higher.objectStore("s").add("again", 1);
		higher.onerror = (event) => { events.push("transaction"); event.preventDefault(); };
		say("prevented higher", await over(higher), events.splice(0).join(", "));

		const thrower = db.transaction("s", "readwrite");
		thrower.objectStore("s").add("again",
			1).onerror = (event) => { event.preventDefault(); throw new Error("in error"); };
		say("an error listener that throws", await failing(over(thrower)), thrower.error.name);
	}`)
	says(t, got,
		"not prevented ConstraintError database capture 1, request ConstraintError 2 true, transaction true true, "+
			"database 3 true, database capture 1, transaction false true, database 3 true, "+
			"abort ConstraintError true true, database abort",
		`undone ["there"]`,
		`prevented complete database capture 1, request, transaction, database 3 true ["there","after the error"]`,
		"stopped ConstraintError database capture 1, request, database abort",
		"prevented higher complete database capture 1, transaction, database 3 true",
		"an error listener that throws AbortError AbortError",
	)
	if said := s.said(); strings.Count(said, "in error") != 1 {
		t.Errorf("the console says %s", said)
	}
}

func TestTwoPagesShareTheirSiteDatabases(t *testing.T) {
	site := idb.New("").Site("https://site.test")
	one, got := based(t, site, `async function main() {
		window.db = await opened("db", 1, (db) => db.createObjectStore("s").put("from one", 1));
		db.onversionchange = (event) => say("versionchange", event.oldVersion, event.newVersion,
			event.constructor.name);
		window.read = async () => say("one reads", await got(db.transaction("s").objectStore("s").get(1)));
		window.spinning = true;
		window.spin = () => { held.objectStore("s").get(2).onsuccess = () => window.spinning && spin(); };
		window.hold = () => {
			window.held = db.transaction("s", "readwrite");
			held.objectStore("s").put("held", 2).onsuccess = () => { say("one holds"); spin(); };
		};
	}`)
	says(t, got, "")
	two, got := based(t, site, `async function main() {
		const db = await opened("db");
		say("two reads", db.version, await got(db.transaction("s").objectStore("s").get(1)));
		await got(db.transaction("s", "readwrite").objectStore("s").put("from two", 1));
		db.close();
		window.upgrade = (version) => opened("db", version, (db, tx, event) => say("upgrade", event.oldVersion,
			event.newVersion))
			.then((db) => { say("two opened", db.version); window.db = db; }, (error) => say("two failed",
				error.name));
		window.remove = () => {
			const request = indexedDB.deleteDatabase("db");
			request.onblocked = (event) => say("blocked", event.oldVersion, event.newVersion);
			request.onsuccess = () => say("two deleted");
		};
		window.write = () => { window.db.transaction("s", "readwrite").objectStore("s").put("two",
			3).onsuccess = () => say("two writes"); };
	}`)
	says(t, got, "two reads 1 from one")
	both := func(what string, held func() bool) {
		t.Helper()
		for end := time.Now().Add(2 * time.Second); !held(); {
			if time.Now().After(end) {
				t.Fatalf("%s never came; one said %s, two said %s", what, one.eval("out.join('|')"), two.eval("out.join('|')"))
			}
			one.rt.Frame()
			two.rt.Frame()
		}
	}
	said := func(s *scripted, line string) func() bool {
		return func() bool { return strings.Contains(s.eval(`out.join("\n")`), line) }
	}
	one.eval("read()")
	both("what two wrote", said(one, "one reads from two"))

	two.eval("upgrade(2)")
	both("a blocked upgrade", said(two, "blocked"))
	got = strings.Split(one.eval(`out.join("\n")`), "\n")
	if got[len(got)-1] != "versionchange 1 2 IDBVersionChangeEvent" {
		t.Errorf("one said %v", got)
	}
	if said(two, "upgrade 1 2")() {
		t.Error("the database was upgraded under an open connection")
	}
	one.eval("db.close()")
	both("the upgrade", said(two, "two opened 2"))

	one.eval(`opened("db").then((db) => {
		window.db = db;
		db.onversionchange = () => { say("versionchange again"); db.close(); };
		say("one reopened", db.version);
	})`)
	both("one's connection", said(one, "one reopened 2"))
	two.eval("out.length = 0; db.close(); upgrade(3)")
	both("an upgrade that is let through", said(two, "two opened 3"))
	if said(two, "blocked")() {
		t.Error("an upgrade was told it was blocked by a connection that closed when asked")
	}

	one.eval(`opened("db").then((db) => { window.db = db; say("one reopened", db.version); hold(); })`)
	both("one's transaction", said(one, "one holds"))
	two.eval("write()")
	for range 20 {
		one.rt.Frame()
		two.rt.Frame()
	}
	if said(two, "two writes")() {
		t.Error("a page wrote a store another page's transaction held")
	}
	one.eval("window.spinning = false")
	both("the write that waited", said(two, "two writes"))

	two.eval("db.close(); remove()")
	both("the delete", said(two, "blocked 3 null"))
	one.eval("db.close()")
	both("the delete", said(two, "two deleted"))
	if site.Connections() != 0 || len(site.Databases()) != 0 {
		t.Errorf("%d connections and %d databases are left", site.Connections(), len(site.Databases()))
	}
	one.quiet()
	two.quiet()
}

func TestAPageThatIsLeftLetsGoOfItsDatabases(t *testing.T) {
	site := idb.New("").Site("https://site.test")
	s, got := based(t, site, `async function main() {
		const db = await opened("db", 1, (db) => db.createObjectStore("s"));
		await opened("other", 1);
		await got(db.transaction("s", "readwrite").objectStore("s").put("kept", 1));
		window.held = db.transaction("s", "readwrite");
		await got(held.objectStore("s").put("never committed", 2));
		const spin = () => { held.objectStore("s").get(2).onsuccess = spin; };
		spin();
		say("holding");
	}`)
	says(t, got, "holding")
	if site.Connections() != 2 {
		t.Fatalf("%d connections are open", site.Connections())
	}
	other, got := based(t, site, `async function main() {
		window.db = await opened("db");
		const tx = db.transaction("s");
		tx.objectStore("s").getAllKeys().onsuccess = (event) => say("read", show(event.target.result));
	}`)
	says(t, got, "")
	s.rt.Close()
	other.frames("the store a page that left held", func() bool { return other.eval("out.join()") == "read [1]" })
	other.rt.Close()
	if site.Connections() != 0 {
		t.Errorf("%d connections are left by pages that were left", site.Connections())
	}
	if got := site.Databases(); len(got) != 2 || got[0].Version != 1 || got[0].Stores[0].Count != 1 {
		t.Errorf("the databases are %+v", got)
	}
}

func TestADocumentWithoutAnOriginHasNoDatabase(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.start(`<body><script>`+tools+`
		say(typeof indexedDB, thrown(() => indexedDB.open("db")), thrown(() => indexedDB.deleteDatabase("db")),
			indexedDB.cmp(1, 2));
		Promise.all([failing(indexedDB.databases()), failing(navigator.storage.estimate()),
			failing(navigator.storage.persisted())])
			.then((names) => { say(names.join(" ")); window.ended = true; });
	</script>`, dom.Host{Sandboxed: dom.Sandboxed{Opaque: true}, Bases: func() *idb.Site { return idb.New("").Site("x") }})
	says(t, s.lines(), "object SecurityError SecurityError -1", "SecurityError SecurityError SecurityError")
}

func TestASiteIsToldWhatItsDatabasesHold(t *testing.T) {
	store := idb.New("")
	store.SetQuota(4000)
	_, got := based(t, store.Site("a"), `async function main() {
		const before = await navigator.storage.estimate();
		const db = await opened("db", 1, (db) => db.createObjectStore("s"));
		const tx = db.transaction("s", "readwrite"), s = tx.objectStore("s");
		await got(s.put("x".repeat(1000), 1));
		const full = s.put("x".repeat(4000), 2);
		full.onerror = () => say("full", full.error.name);
		say("aborted", await failing(over(tx)), tx.error.name);
		await got(db.transaction("s", "readwrite").objectStore("s").put("x".repeat(1000), 1));
		const after = await navigator.storage.estimate();
		say("holds", before.usage, before.quota, after.usage > 1000, after.usage < 1100,
			after.usageDetails.indexedDB === after.usage,
			await navigator.storage.persisted(), await navigator.storage.persist(),
				navigator.storage instanceof StorageManager);
	}`)
	says(t, got, "full QuotaExceededError", "aborted QuotaExceededError QuotaExceededError",
		"holds 0 4000 true true true false false true")
}

// TestAWrapperOfPromisesWorks uses the databases the way the wrappers
// pages ship do: every request a promise, transactions awaited through
// their done, cursors walked with await.
func TestAWrapperOfPromisesWorks(t *testing.T) {
	_, got := based(t, nil, `
	const wrap = (request) => new Promise((resolve, reject) => {
		request.addEventListener("success", () => resolve(request.result));
		request.addEventListener("error", () => reject(request.error));
	});
	const openDB = (name, version, { upgrade } = {}) => {
		const request = indexedDB.open(name, version);
		request.addEventListener("upgradeneeded", (event) => upgrade(request.result, event.oldVersion,
			event.newVersion, request.transaction));
		return wrap(request).then((db) => ({
			raw: db,
			transaction(names, mode) {
				const tx = db.transaction(names, mode);
				const done = new Promise((resolve, reject) => {
					tx.addEventListener("complete", () => resolve());
					const failed = () => reject(tx.error || new DOMException("aborted", "AbortError"));
					tx.addEventListener("error", failed);
					tx.addEventListener("abort", failed);
				});
				const storeOf = (name) => {
					const store = tx.objectStore(name);
					const source = (from) => ({
						get: (key) => wrap(from.get(key)), getAll: (query, count) => wrap(from.getAll(query, count)),
						getAllKeys: (query, count) => wrap(from.getAllKeys(query, count)),
							count: (query) => wrap(from.count(query)),
						async *iterate(query, direction) {
							let cursor = await wrap(from.openCursor(query, direction));
							while (cursor) {
								yield cursor;
								const request = cursor.request;
								cursor.continue();
								cursor = await wrap(request);
							}
						},
					});
					return {
						...source(store), put: (value, key) => wrap(store.put(value, key)), add: (value,
							key) => wrap(store.add(value, key)),
						delete: (key) => wrap(store.delete(key)), clear: () => wrap(store.clear()),
							index: (name) => source(store.index(name)),
					};
				};
				return { done, store: typeof names === "string" ? storeOf(names) : null, objectStore: storeOf,
					abort: () => tx.abort() };
			},
			async get(store, key) { return this.transaction(store).store.get(key); },
			async getAll(store) { return this.transaction(store).store.getAll(); },
			async put(store, value, key) {
				const tx = this.transaction(store, "readwrite");
				const [made] = await Promise.all([tx.store.put(value, key), tx.done]);
				return made;
			},
		}));
	};
	async function main() {
		const db = await openDB("notes", 2, {
			upgrade(db, oldVersion) {
				say("upgrade from", oldVersion);
				const notes = db.createObjectStore("notes", { keyPath: "id", autoIncrement: true });
				notes.createIndex("by-date", "date");
				notes.createIndex("by-tag", "tags", { multiEntry: true });
				db.createObjectStore("settings");
			},
		});
		await db.put("settings", { theme: "dark" }, "ui");
		say("a setting", show(await db.get("settings", "ui")));
		const tx = db.transaction(["notes", "settings"], "readwrite");
		const notes = tx.objectStore("notes");
		const ids = await Promise.all([
			notes.add({ title: "milk", date: new Date(3), tags: ["home"] }),
			notes.add({ title: "tests", date: new Date(1), tags: ["work", "light"] }),
			notes.add({ title: "bike", date: new Date(2), tags: ["home", "light"] }),
		]);
		const count = await notes.count();
		await tx.objectStore("settings").put(count, "notes");
		await tx.done;
		say("added", show(ids), count, await db.get("settings", "notes"));

		const reading = db.transaction("notes");
		const titles = [];
		for await (const cursor of reading.store.index("by-date").iterate(null,
			"prev")) titles.push(cursor.value.title);
		say("by date", titles.join(),
			show((await reading.store.index("by-tag").getAll("light")).map((note) => note.title)),
			show(await reading.store.index("by-tag").getAllKeys(IDBKeyRange.bound("h", "i"))));
		await reading.done;

		const editing = db.transaction("notes", "readwrite");
		for await (const cursor of editing.store.iterate()) {
			if (cursor.value.tags.includes("home")) await wrap(cursor.update({ ...cursor.value, done: true }));
			else await wrap(cursor.delete());
		}
		await editing.done;
		say("edited", show((await db.getAll("notes")).map((note) => [note.id, note.title, note.done])));

		const refusing = db.transaction("notes", "readwrite");
		refusing.store.put({ id: 1, title: "never" });
		const refused = refusing.store.add({ id: 1, title: "taken" }).catch((error) => error.name);
		const ended = refusing.done.catch((error) => error.name);
		say("refused", await refused, await ended, (await db.get("notes", 1)).title);

		const late = db.transaction("notes", "readwrite");
		await late.store.get(1);
		await new Promise((resolve) => setTimeout(resolve, 0));
		say("too late", thrown(() => late.store.put({ title: "late" })), await late.done.then(() => "done"));
	}`)
	says(t, got,
		"upgrade from 0",
		`a setting {"theme":"dark"}`,
		"added [1,2,3] 3 3",
		`by date milk,bike,tests ["tests","bike"] [1,3]`,
		`edited [[1,"milk",true],[3,"bike",true]]`,
		"refused ConstraintError AbortError milk",
		"too late TransactionInactiveError done",
	)
}

// TestManyRecords measures a transaction of ten thousand puts and what
// reads them back, which its log tells.
func TestManyRecords(t *testing.T) {
	began := time.Now()
	s, got := based(t, nil, `async function main() {
		const db = await opened("db", 1, (db) => db.createObjectStore("s", { keyPath: "id" }).createIndex("group",
			"group"));
		let at = performance.now();
		const tx = db.transaction("s", "readwrite"), s = tx.objectStore("s");
		for (let id = 0; id < 10000; id++) s.put({ id, group: id % 10, name: "record " + id, tags: ["a", "b"] });
		window.asked = performance.now() - at;
		await over(tx);
		window.put = performance.now() - at;
		at = performance.now();
		const all = await got(db.transaction("s").objectStore("s").getAll());
		window.all = performance.now() - at;
		at = performance.now();
		let walked = 0;
		await new Promise((resolve) => {
			db.transaction("s").objectStore("s").index("group").openCursor(3).onsuccess = (event) => {
				const cursor = event.target.result;
				if (!cursor) return resolve();
				walked++;
				cursor.continue();
			};
		});
		window.walk = performance.now() - at;
		at = performance.now();
		const one = db.transaction("s", "readwrite");
		for (let i = 0; i < 1000; i++) {
			await got(one.objectStore("s").put({ id: 20000 + i, group: 1 }));
			await got(one.objectStore("s").get(20000 + i));
		}
		window.each = (performance.now() - at) / 1000;
		say(all.length, all[9999].name, walked, await got(db.transaction("s").objectStore("s").count()));
	}`)
	says(t, got, "10000 record 9999 1000 11000")
	t.Logf("10,000 puts in one transaction: asked in %s ms, complete after %s ms; getAll of them %s ms; a cursor "+
		"over 1,000 of them %s ms; a put then a get, each awaited, %s ms the pair; the page in all %v",
		s.eval("asked.toFixed(1)"), s.eval("put.toFixed(1)"), s.eval("all.toFixed(1)"), s.eval("walk.toFixed(1)"),
		s.eval("each.toFixed(3)"), time.Since(began).Round(time.Millisecond))
}
