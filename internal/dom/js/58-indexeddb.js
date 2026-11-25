// ---- IndexedDB: the site's databases, which light keeps (pkg/idb)
//
// The records, their order, the indexes, the transactions' turns and
// the connections of every document of the site are light's, in Go:
// what is here is the standard's objects over it. A request is done as
// it is made, when its transaction has its stores, and its event comes
// later, a task each. A key crosses as the bytes light orders records
// by, and a value as the bytes of its structured clone.

const idbNamed = /^(ConstraintError|DataError|TransactionInactiveError|ReadOnlyError|InvalidStateError|NotFoundError|VersionError|AbortError|InvalidAccessError|QuotaExceededError|DataCloneError|SecurityError|UnknownError): (.*)$/s;
const based = (...args) => {
  try {
    return light("idb", ...args);
  } catch (error) {
    const named = idbNamed.exec(error.message);
    if (named) throw new DOMException(named[2], named[1]);
    throw error;
  }
};
const idbError = (name, message) => new DOMException(message, name);
const inactive = () => idbError("TransactionInactiveError", "The transaction is not active.");
const notKey = () => idbError("DataError", "The parameter is not a valid key.");

// ---- the bytes that cross: one buffer both sides read and write

let wire = new SharedBuffer(1 << 16), wired = new Uint8Array(wire), wireAt = 0;
const room = (size) => {
  if (wireAt + size <= wired.length) return;
  const grown = new SharedBuffer(Math.max(wired.length * 2, wireAt + size));
  new Uint8Array(grown).set(wired.subarray(0, wireAt));
  wire = grown;
  wired = new Uint8Array(grown);
};
const putByte = (byte) => {
  room(1);
  wired[wireAt++] = byte;
};
const putCount = (count) => {
  room(4);
  wired[wireAt] = count >>> 24;
  wired[wireAt + 1] = count >>> 16;
  wired[wireAt + 2] = count >>> 8;
  wired[wireAt + 3] = count;
  wireAt += 4;
};
const setCount = (at, count) => {
  wired[at] = count >>> 24;
  wired[at + 1] = count >>> 16;
  wired[at + 2] = count >>> 8;
  wired[at + 3] = count;
};
const putBytes = (bytes) => {
  if (bytes === null) return putCount(0);
  putCount(bytes.length);
  room(bytes.length);
  wired.set(bytes, wireAt);
  wireAt += bytes.length;
};
const floats = new DataView(new ArrayBuffer(8)), floated = new Uint8Array(floats.buffer);
const putFloat = (number) => {
  floats.setFloat64(0, number);
  room(8);
  wired.set(floated, wireAt);
  wireAt += 8;
};
const takeByte = () => wired[wireAt++];
const takeCount = () => {
  const count = (wired[wireAt] * 0x1000000) + (wired[wireAt + 1] << 16 | wired[wireAt + 2] << 8 | wired[wireAt + 3]);
  wireAt += 4;
  return count;
};
// takeBytes are bytes of the buffer itself: what keeps them copies.
const takeBytes = () => {
  const size = takeCount();
  wireAt += size;
  return wired.subarray(wireAt - size, wireAt);
};
const takeFloat = () => {
  floated.set(wired.subarray(wireAt, wireAt + 8));
  wireAt += 8;
  return floats.getFloat64(0);
};
// sent asks light for an operation on what the buffer holds, and
// leaves its answer there to be taken.
const sent = (op, tx, store, index, ...more) => {
  const size = based(op, tx, store, index, wireAt, wire, ...more);
  wireAt = 0;
  if (size < 0) {
    wire = new SharedBuffer(-size);
    wired = new Uint8Array(wire);
    based("answer", 0, 0, 0, 0, wire);
  }
  return size;
};

// ---- keys

// keyInto writes value as a key, and says whether it is one.
const keyInto = (value, out, within) => {
  if (typeof value === "number" || value instanceof Date) {
    const number = typeof value === "number" ? value : Date.prototype.getTime.call(value);
    if (Number.isNaN(number)) return false;
    floats.setFloat64(0, number === 0 ? 0 : number);
    out.push(typeof value === "number" ? 0x10 : 0x20);
    const negative = floated[0] & 0x80;
    for (let i = 0; i < 8; i++) out.push(negative ? floated[i] ^ 0xff : i === 0 ? floated[i] | 0x80 : floated[i]);
    return true;
  }
  if (typeof value === "string") {
    out.push(0x30);
    for (let i = 0; i < value.length; i++) {
      const unit = value.charCodeAt(i);
      if (unit < 0x7f) out.push(unit + 1);
      else if (unit < 0x407f) out.push(0x80 | (unit - 0x7f) >> 8, (unit - 0x7f) & 0xff);
      else out.push(0xc0, unit >> 8, unit & 0xff);
    }
    out.push(0);
    return true;
  }
  if (value instanceof ArrayBuffer || ArrayBuffer.isView(value)) {
    const buffer = value instanceof ArrayBuffer ? value : value.buffer;
    if (!(buffer instanceof ArrayBuffer) || buffer.detached) return false;
    const bytes = value instanceof ArrayBuffer ? new Uint8Array(value) : new Uint8Array(buffer, value.byteOffset, value.byteLength);
    out.push(0x40);
    for (const byte of bytes) {
      if (byte < 2) out.push(1, byte + 1);
      else out.push(byte);
    }
    out.push(0);
    return true;
  }
  if (Array.isArray(value)) {
    if (within.includes(value)) return false;
    within.push(value);
    out.push(0x50);
    const length = value.length;
    for (let i = 0; i < length; i++) {
      if (!Object.hasOwn(value, i) || !keyInto(value[i], out, within)) return false;
    }
    within.pop();
    out.push(0);
    return true;
  }
  return false;
};
// keyOf is the key of a value as light orders it, null for what is no
// key.
const keyOf = (value) => {
  const out = [];
  return keyInto(value, out, []) ? Uint8Array.from(out) : null;
};
const validKey = (value) => {
  const key = keyOf(value);
  if (key === null) throw notKey();
  return key;
};
// keyValue is the value of a key, made anew.
const keyValue = (key) => keyFrom(key, 0)[0];
const keyFrom = (key, at) => {
  const kind = key[at++];
  if (kind === 0x10 || kind === 0x20) {
    const negative = !(key[at] & 0x80);
    for (let i = 0; i < 8; i++) floated[i] = negative ? key[at + i] ^ 0xff : i === 0 ? key[at] & 0x7f : key[at + i];
    const number = floats.getFloat64(0);
    return [kind === 0x10 ? number : new Date(number), at + 8];
  }
  if (kind === 0x30) {
    const units = [];
    for (; key[at] !== 0;) {
      const byte = key[at];
      if (byte < 0x80) { units.push(byte - 1); at += 1; }
      else if (byte < 0xc0) { units.push(((byte & 0x3f) << 8) + key[at + 1] + 0x7f); at += 2; }
      else { units.push((key[at + 1] << 8) + key[at + 2]); at += 3; }
    }
    let text = "";
    for (let i = 0; i < units.length; i += 8192) text += String.fromCharCode.apply(null, units.slice(i, i + 8192));
    return [text, at + 1];
  }
  if (kind === 0x40) {
    const bytes = [];
    for (; key[at] !== 0; at++) bytes.push(key[at] === 1 ? key[++at] - 1 : key[at]);
    return [Uint8Array.from(bytes).buffer, at + 1];
  }
  const list = [];
  while (key[at] !== 0) {
    const [item, next] = keyFrom(key, at);
    list.push(item);
    at = next;
  }
  return [list, at + 1];
};
const compareKeys = (a, b) => {
  const length = Math.min(a.length, b.length);
  for (let i = 0; i < length; i++) {
    if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  }
  return a.length === b.length ? 0 : a.length < b.length ? -1 : 1;
};

// ---- key paths

const identifier = /^[$_\p{ID_Start}][$‌‍\p{ID_Continue}]*$/u;
const validPath = (path) => (Array.isArray(path)
  ? path.length > 0 && path.every((item) => typeof item === "string" && validPath(item))
  : path === "" || path.split(".").every((name) => identifier.test(name)));
// pathOfKeys reads the key path a script gave: null for none, a list
// of strings for a list, else a string.
const pathOfKeys = (given) => {
  if (given === undefined || given === null) return null;
  const path = typeof given === "object" && typeof given[Symbol.iterator] === "function" ? Array.from(given, String) : String(given);
  if (!validPath(path)) throw idbError("SyntaxError", "The keyPath argument contains an invalid key path.");
  return path;
};
const samePath = (path) => (Array.isArray(path) ? path.slice() : path);
const MISSING = Symbol("no value at the key path");
// along is the value at a key path of value, MISSING for none.
const along = (value, path) => {
  if (Array.isArray(path)) {
    const list = [];
    for (const item of path) {
      const found = along(value, item);
      if (found === MISSING) return MISSING;
      list.push(found);
    }
    return list;
  }
  if (path === "") return value;
  for (const name of path.split(".")) {
    if (typeof value === "string" && name === "length") value = value.length;
    else if (Array.isArray(value) && name === "length") value = value.length;
    else if (value instanceof Blob && (name === "size" || name === "type")) value = value[name];
    else if (value instanceof File && (name === "name" || name === "lastModified")) value = value[name];
    else {
      if (value === null || typeof value !== "object" || !Object.hasOwn(value, name)) return MISSING;
      value = value[name];
      if (value === undefined) return MISSING;
    }
  }
  return value;
};
// injectable reports whether a key may be put at a key path of value.
const injectable = (value, path) => {
  const names = path.split(".");
  names.pop();
  for (const name of names) {
    if (value === null || typeof value !== "object") return false;
    if (!Object.hasOwn(value, name)) return true;
    value = value[name];
  }
  return value !== null && typeof value === "object";
};
const inject = (value, path, key) => {
  const names = path.split(".");
  const last = names.pop();
  for (const name of names) {
    if (!Object.hasOwn(value, name)) put(value, name, {});
    value = value[name];
  }
  put(value, last, key);
};
// listed are the keys a value is listed under in an index: none for a
// value the index has no key for.
const listed = (value, index) => {
  const found = along(value, index.keyPath);
  if (found === MISSING) return [];
  if (!index.multiEntry || !Array.isArray(found)) {
    const key = keyOf(found);
    return key === null ? [] : [key];
  }
  const keys = [];
  for (let i = 0; i < found.length; i++) {
    let key = null;
    try {
      key = keyOf(found[i]);
    } catch (_) { /* an item that cannot be read is no key */ }
    if (key !== null && !keys.some((other) => compareKeys(other, key) === 0)) keys.push(key);
  }
  return keys;
};

// ---- values: the bytes of a structured clone

const V = {
  undefined: 0, null: 1, true: 2, false: 3, number: 4, latin: 5, text: 6, bigint: 7, again: 8, boxed: 9, date: 10,
  regexp: 11, buffer: 12, view: 13, map: 14, set: 15, domException: 16, error: 17, blob: 18, file: 19, array: 20,
  object: 21,
};
const viewNames = ["DataView", ...viewKinds.map((kind) => kind.name)];
const packText = (text) => {
  let wide = false;
  for (let i = 0; i < text.length && !wide; i++) wide = text.charCodeAt(i) > 0xff;
  putByte(wide ? V.text : V.latin);
  putCount(text.length);
  room(text.length * 2);
  if (!wide) {
    for (let i = 0; i < text.length; i++) wired[wireAt++] = text.charCodeAt(i);
    return;
  }
  for (let i = 0; i < text.length; i++) {
    const unit = text.charCodeAt(i);
    wired[wireAt++] = unit & 0xff;
    wired[wireAt++] = unit >> 8;
  }
};
// pack writes a value, one that the page's structured clone made, into
// the buffer. Each object met is numbered in the order it is met, so
// one met again, or within itself, is written as its number.
const pack = (value, seen) => {
  switch (typeof value) {
    case "undefined": return putByte(V.undefined);
    case "boolean": return putByte(value ? V.true : V.false);
    case "number": putByte(V.number); return putFloat(value);
    case "string": return packText(value);
    case "bigint": putByte(V.bigint); return packText(value.toString());
  }
  if (value === null) return putByte(V.null);
  if (seen.has(value)) {
    putByte(V.again);
    return putCount(seen.get(value));
  }
  if (ArrayBuffer.isView(value)) {
    putByte(V.view);
    putByte(value instanceof DataView ? 0 : viewNames.indexOf(viewKinds.find((kind) => value instanceof kind).name));
    pack(value.buffer, seen);
    seen.set(value, seen.size);
    putCount(value.byteOffset);
    return putCount(value instanceof DataView ? value.byteLength : value.length);
  }
  seen.set(value, seen.size);
  if (value instanceof Boolean || value instanceof Number || value instanceof String || value instanceof BigInt) {
    putByte(V.boxed);
    return pack(value.valueOf(), seen);
  }
  if (value instanceof Date) { putByte(V.date); return putFloat(value.getTime()); }
  if (value instanceof RegExp) { putByte(V.regexp); packText(value.source); return packText(value.flags); }
  if (value instanceof ArrayBuffer) { putByte(V.buffer); return putBytes(new Uint8Array(value)); }
  if (value instanceof Map) {
    putByte(V.map);
    putCount(value.size);
    for (const [key, item] of value) { pack(key, seen); pack(item, seen); }
    return undefined;
  }
  if (value instanceof Set) {
    putByte(V.set);
    putCount(value.size);
    for (const item of value) pack(item, seen);
    return undefined;
  }
  if (value instanceof DOMException) { putByte(V.domException); packText(String(value.message)); return packText(String(value.name)); }
  if (value instanceof Error) {
    putByte(V.error);
    packText(Object.hasOwn(errorKinds, value.name) ? value.name : "Error");
    packText(String(value.message));
    pack(typeof value.stack === "string" ? value.stack : undefined, seen);
    putByte(Object.hasOwn(value, "cause") ? 1 : 0);
    if (Object.hasOwn(value, "cause")) pack(value.cause, seen);
    return undefined;
  }
  if (value instanceof Blob) {
    const file = value instanceof File;
    putByte(file ? V.file : V.blob);
    if (file) { packText(value.name); putFloat(value.lastModified); }
    packText(value.type);
    return putBytes(value._bytes);
  }
  const keys = Object.keys(value);
  if (Array.isArray(value)) { putByte(V.array); putCount(value.length); } else putByte(V.object);
  putCount(keys.length);
  for (const key of keys) { packText(key); pack(value[key], seen); }
  return undefined;
};
const packed = (kind) => idbError("DataError", `A stored value is not one light wrote (${kind}).`);
const unpackText = (kind) => {
  const length = takeCount();
  let text = "";
  if (kind === V.latin) {
    for (let i = 0; i < length; i += 8192) text += String.fromCharCode.apply(null, wired.subarray(wireAt + i, Math.min(wireAt + length, wireAt + i + 8192)));
    wireAt += length;
    return text;
  }
  const units = new Uint16Array(wired.slice(wireAt, wireAt + length * 2).buffer);
  for (let i = 0; i < length; i += 8192) text += String.fromCharCode.apply(null, units.subarray(i, i + 8192));
  wireAt += length * 2;
  return text;
};
const unpackString = () => {
  const kind = takeByte();
  if (kind !== V.latin && kind !== V.text) throw packed(kind);
  return unpackText(kind);
};
// unpack reads a value pack wrote, made of the page's own objects.
const unpack = (made) => {
  const keep = (object) => { made.push(object); return object; };
  const kind = takeByte();
  switch (kind) {
    case V.undefined: return undefined;
    case V.null: return null;
    case V.true: return true;
    case V.false: return false;
    case V.number: return takeFloat();
    case V.latin: case V.text: return unpackText(kind);
    case V.bigint: return BigInt(unpackString());
    case V.again: return made[takeCount()];
    case V.boxed: {
      const at = made.length;
      made.push(null);
      return made[at] = Object(unpack(made));
    }
    case V.date: return keep(new Date(takeFloat()));
    case V.regexp: { const source = unpackString(); return keep(new RegExp(source, unpackString())); }
    case V.buffer: return keep(takeBytes().slice().buffer);
    case V.view: {
      const name = viewNames[takeByte()];
      const buffer = unpack(made);
      const view = name === "DataView" ? DataView : viewKinds.find((one) => one.name === name);
      if (!view || !(buffer instanceof ArrayBuffer)) throw packed(kind);
      const offset = takeCount();
      return keep(new view(buffer, offset, takeCount()));
    }
    case V.map: {
      const map = keep(new Map());
      for (let left = takeCount(); left > 0; left--) { const key = unpack(made); map.set(key, unpack(made)); }
      return map;
    }
    case V.set: {
      const set = keep(new Set());
      for (let left = takeCount(); left > 0; left--) set.add(unpack(made));
      return set;
    }
    case V.domException: { const message = unpackString(); return keep(new DOMException(message, unpackString())); }
    case V.error: {
      const name = unpackString();
      const error = keep(new (Object.hasOwn(errorKinds, name) ? errorKinds[name] : Error)(unpackString()));
      const stack = unpack(made);
      if (typeof stack === "string") define(error, { stack });
      if (takeByte()) define(error, { cause: unpack(made) });
      return error;
    }
    case V.blob: { const type = unpackString(); return keep(new Blob([takeBytes()], { type })); }
    case V.file: {
      const name = unpackString(), lastModified = takeFloat(), type = unpackString();
      return keep(new File([takeBytes()], name, { type, lastModified }));
    }
    case V.array: case V.object: {
      const object = keep(kind === V.array ? new Array(takeCount()) : {});
      for (let left = takeCount(); left > 0; left--) { const key = unpackString(); put(object, key, unpack(made)); }
      return object;
    }
  }
  throw packed(kind);
};
// packValue writes a value after its length, and takeValue reads one.
const packValue = (value) => {
  const at = wireAt;
  putCount(0);
  pack(value, new Map());
  setCount(at, wireAt - at - 4);
};
const takeValue = () => {
  const end = wireAt + 4 + takeCount();
  const value = unpack([]);
  if (wireAt !== end) throw packed("its length");
  return value;
};

// ---- events: tasks light runs one at a time

const idbState = new WeakMap();
const idbOf = (object, kind) => {
  const state = idbState.get(object);
  if (!state || !(object instanceof kind)) throw new TypeError("Illegal invocation");
  return state;
};
// idbTasks are the events to come, the oldest first, and idbAfter what
// the last one leaves to do once the microtasks of its listeners ran.
// fresh are the transactions made since light last entered the page:
// they are active until it enters again.
const idbTasks = [];
let idbAfter = null, idbAwake = false;
const fresh = [];
const wake = () => {
  if (idbAwake) return;
  idbAwake = true;
  light("idb", "wake");
};
const later = (task) => {
  idbTasks.push(task);
  wake();
};
// settle is what light does of the databases each time it enters the
// page: the microtasks of what ran before have run.
const settle = () => {
  if (idbAfter) {
    const after = idbAfter;
    idbAfter = null;
    after();
  }
  for (const tx of fresh.splice(0)) {
    if (tx.state === "active") tx.state = "inactive";
    proceed(tx);
  }
};
// fired sends an event along a path, the first its target, as a
// browser dispatches it, and says whether a listener ran and whether
// one threw.
const fired = (path, event) => {
  let ran = false, threw = false;
  const state = eventStates.get(event);
  Object.assign(state, { target: path[0], path, stopped: false, halted: false, trusted: true });
  const call = (target, phase) => {
    state.currentTarget = target;
    state.phase = phase;
    const lists = listenersOf(target);
    const list = lists ? (lists.get(event.type) || []).slice() : [];
    const inline = phase !== CAPTURING ? handlerOf(target, event.type) : null;
    if (inline) list.unshift({ callback: inline, capture: false, inline: true });
    for (const listener of list) {
      if (listener.removed) continue;
      if (phase === CAPTURING && !listener.capture) continue;
      if (phase === BUBBLING && listener.capture) continue;
      if (listener.once) target.removeEventListener(event.type, listener.callback, listener.capture);
      const callback = listener.callback;
      ran = true;
      try {
        let result;
        if (typeof callback === "function") result = callback.call(target, event);
        else if (callback && typeof callback.handleEvent === "function") result = callback.handleEvent(event);
        if (listener.inline && result === false) event.preventDefault();
      } catch (error) {
        threw = true;
        report(error);
      }
      if (state.halted) break;
    }
  };
  for (let i = path.length - 1; i > 0 && !state.stopped; i--) call(path[i], CAPTURING);
  if (!state.stopped) call(path[0], AT_TARGET);
  if (state.bubbles) {
    for (let i = 1; i < path.length && !state.stopped; i++) call(path[i], BUBBLING);
  }
  state.currentTarget = null;
  state.phase = NONE;
  return { ran, threw };
};
// task is a task that fires an event, then does after once the
// microtasks of its listeners ran: at once when none ran.
const firing = (path, event, after) => {
  const { ran, threw } = fired(path, event);
  if (ran) {
    idbAfter = () => after(threw);
    return true;
  }
  after(false);
  return false;
};

class IDBVersionChangeEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.oldVersion = Number(init.oldVersion) || 0;
    this.newVersion = init.newVersion === undefined || init.newVersion === null ? null : Number(init.newVersion);
  }
}

class DOMStringList {
  constructor(key, names) {
    if (key !== token) illegal();
    names.forEach((name, i) => { Object.defineProperty(this, i, { value: name, enumerable: true }); });
    Object.defineProperty(this, "length", { value: names.length });
  }
  item(i) { return i >= 0 && i < this.length ? this[i] : null; }
  contains(name) { return Array.prototype.includes.call(this, String(name)); }
  [Symbol.iterator]() { return Array.prototype.values.call(this); }
}
const namesOf = (names) => new DOMStringList(token, Array.from(names).sort());

// ---- requests

class IDBRequest extends EventTarget {
  constructor(key) {
    if (key !== token) illegal();
    super();
    idbState.set(this, { object: this, done: false, dead: false, result: undefined, error: null, source: null, tx: null });
  }
  get result() {
    const state = idbOf(this, IDBRequest);
    if (!state.done) throw idbError("InvalidStateError", "The request has not finished.");
    return state.result;
  }
  get error() {
    const state = idbOf(this, IDBRequest);
    if (!state.done) throw idbError("InvalidStateError", "The request has not finished.");
    return state.error;
  }
  get source() { return idbOf(this, IDBRequest).source; }
  get transaction() { const tx = idbOf(this, IDBRequest).tx; return tx ? tx.object : null; }
  get readyState() { return idbOf(this, IDBRequest).done ? "done" : "pending"; }
}
withHandlers(IDBRequest.prototype, ["success", "error"]);
class IDBOpenDBRequest extends IDBRequest {}
withHandlers(IDBOpenDBRequest.prototype, ["blocked", "upgradeneeded"]);

const unknown = (error) => (error instanceof DOMException ? error : idbError("UnknownError", String(error && error.message || error)));
// placed places a request on a transaction: run does it and gives its
// result, now when the transaction has its stores, else once it has.
// again is the request of a cursor that moves on.
const placed = (tx, source, run, again) => {
  const request = again || idbState.get(new IDBRequest(token));
  request.source = source;
  request.tx = tx;
  request.done = false;
  tx.requests.add(request);
  const act = () => {
    if (request.dead) return;
    let result, error = null;
    try {
      result = run();
    } catch (failure) {
      error = unknown(failure);
    }
    later(() => concluded(tx, request, result, error));
  };
  if (tx.started) act();
  else tx.waiting.push(act);
  return request.object;
};
// concluded is the task of a request's event: its transaction is active
// while its listeners run, and the microtasks they leave.
const concluded = (tx, request, result, error) => {
  if (request.dead || !tx.requests.has(request)) return false;
  tx.requests.delete(request);
  request.done = true;
  request.result = error ? undefined : typeof result === "function" && result.arrival ? result() : result;
  request.error = error;
  const event = error ? new Event("error", { bubbles: true, cancelable: true }) : new Event("success");
  if (tx.state === "inactive") tx.state = "active";
  return firing([request.object, tx.object, tx.db.object], event, (threw) => {
    if (tx.state === "active") tx.state = "inactive";
    if (threw) abortTx(tx, idbError("AbortError", "A listener of the transaction's request threw."));
    else if (error && !event.defaultPrevented) abortTx(tx, error);
    proceed(tx);
  });
};

// ---- transactions

// proceed commits a transaction that has nothing left to wait for.
const proceed = (tx) => {
  if (tx.failed) return abortTx(tx, tx.failed);
  if (tx.requests.size > 0 || (tx.state !== "inactive" && tx.state !== "committing") || tx.committed) return;
  tx.state = "committing";
  tx.committed = true;
  try {
    based("commit", tx.id);
  } catch (error) {
    tx.committed = false;
    abortTx(tx, unknown(error));
    return;
  }
  later(() => {
    tx.state = "finished";
    finished(tx);
    return firing([tx.object, tx.db.object], new Event("complete"), () => {});
  });
};
const finished = (tx) => {
  tx.db.txs.delete(tx);
  transactions.delete(tx.id);
  if (tx.db.upgrade === tx) tx.db.upgrade = null;
  if (tx.opening) tx.opening.tx = null;
};
// abortTx undoes a transaction: its requests fail, then it is told.
const abortTx = (tx, error) => {
  if (tx.state === "finished" || tx.committed) return;
  tx.state = "finished";
  tx.error = error;
  tx.waiting = [];
  try {
    light("idb", "abort", tx.id);
  } catch (_) { /* light undid it already */ }
  if (tx.before) revert(tx);
  for (const request of tx.requests) {
    request.dead = true;
    later(() => {
      request.done = true;
      request.result = undefined;
      request.error = idbError("AbortError", "The transaction was aborted.");
      return firing([request.object, tx.object, tx.db.object], new Event("error", { bubbles: true, cancelable: true }), () => {});
    });
  }
  tx.requests.clear();
  later(() => {
    finished(tx);
    if (tx.opening) {
      tx.opening.result = undefined;
      tx.opening.done = false;
    }
    return firing([tx.object, tx.db.object], new Event("abort", { bubbles: true }), () => {});
  });
};
// revert takes a database back to what it was before the upgrade that
// aborted: the stores and indexes made in it are deleted ones.
const revert = (tx) => {
  const db = tx.db, was = tx.before;
  db.version = was.version;
  db.stores = was.stores;
  const stores = new Map(Array.from(was.stores.values(), (store) => [store.id, store]));
  for (const handle of tx.handles) {
    const store = stores.get(handle.schema.id);
    if (!store) {
      handle.deleted = true;
      continue;
    }
    const indexes = new Map(Array.from(store.indexes.values(), (index) => [index.id, index]));
    handle.schema = store;
    handle.deleted = false;
    for (const index of handle.made) {
      const old = indexes.get(index.schema.id);
      index.deleted = !old;
      if (old) index.schema = old;
    }
  }
};
const copySchema = (stores) => new Map(Array.from(stores, ([name, store]) => [name, {
  ...store, indexes: new Map(Array.from(store.indexes, ([key, index]) => [key, { ...index }])),
}]));

// transactions are the transactions that have not finished, by the
// number light knows each by.
const transactions = new Map();
const modes = ["readonly", "readwrite", "versionchange"];
class IDBTransaction extends EventTarget {
  constructor(key, state) {
    if (key !== token) illegal();
    super();
    idbState.set(this, Object.assign(state, {
      object: this, state: "active", requests: new Set(), waiting: [], error: null, committed: false,
      handles: [], named: new Map(),
    }));
    state.db.txs.add(state);
    transactions.set(state.id, state);
  }
  get objectStoreNames() {
    const tx = idbOf(this, IDBTransaction);
    return namesOf(tx.mode === 2 ? tx.db.stores.keys() : tx.names);
  }
  get mode() { return modes[idbOf(this, IDBTransaction).mode]; }
  get durability() { return idbOf(this, IDBTransaction).durability; }
  get db() { return idbOf(this, IDBTransaction).db.object; }
  get error() { return idbOf(this, IDBTransaction).error; }
  objectStore(name) {
    const tx = idbOf(this, IDBTransaction);
    name = String(name);
    if (tx.state === "finished") throw idbError("InvalidStateError", "The transaction has finished.");
    const schema = tx.db.stores.get(name);
    if (!schema || (tx.mode !== 2 && !tx.names.includes(name))) throw idbError("NotFoundError", "The specified object store was not found.");
    let handle = tx.named.get(schema.id);
    if (!handle) {
      handle = idbState.get(new IDBObjectStore(token, { tx, schema }));
      tx.named.set(schema.id, handle);
      tx.handles.push(handle);
    }
    return handle.object;
  }
  commit() {
    const tx = idbOf(this, IDBTransaction);
    if (tx.state !== "active") throw idbError("InvalidStateError", "The transaction is not active.");
    tx.state = "committing";
    proceed(tx);
  }
  abort() {
    const tx = idbOf(this, IDBTransaction);
    if (tx.state === "committing" || tx.state === "finished") throw idbError("InvalidStateError", "The transaction is committing or has finished.");
    abortTx(tx, null);
  }
}
withHandlers(IDBTransaction.prototype, ["abort", "complete", "error"]);

// ---- key ranges

class IDBKeyRange {
  constructor(key, state) {
    if (key !== token) illegal();
    idbState.set(this, state);
  }
  get lower() { const { lower } = idbOf(this, IDBKeyRange); return lower === null ? undefined : keyValue(lower); }
  get upper() { const { upper } = idbOf(this, IDBKeyRange); return upper === null ? undefined : keyValue(upper); }
  get lowerOpen() { return idbOf(this, IDBKeyRange).lowerOpen; }
  get upperOpen() { return idbOf(this, IDBKeyRange).upperOpen; }
  includes(key) {
    if (arguments.length === 0) throw new TypeError("includes needs a key");
    return within(idbOf(this, IDBKeyRange), validKey(key));
  }
  static only(value) {
    if (arguments.length === 0) throw new TypeError("only needs a key");
    const key = validKey(value);
    return new IDBKeyRange(token, { lower: key, upper: key, lowerOpen: false, upperOpen: false });
  }
  static lowerBound(lower, open = false) {
    if (arguments.length === 0) throw new TypeError("lowerBound needs a key");
    return new IDBKeyRange(token, { lower: validKey(lower), upper: null, lowerOpen: Boolean(open), upperOpen: true });
  }
  static upperBound(upper, open = false) {
    if (arguments.length === 0) throw new TypeError("upperBound needs a key");
    return new IDBKeyRange(token, { lower: null, upper: validKey(upper), lowerOpen: true, upperOpen: Boolean(open) });
  }
  static bound(lower, upper, lowerOpen = false, upperOpen = false) {
    if (arguments.length < 2) throw new TypeError("bound needs two keys");
    const range = { lower: validKey(lower), upper: validKey(upper), lowerOpen: Boolean(lowerOpen), upperOpen: Boolean(upperOpen) };
    const order = compareKeys(range.lower, range.upper);
    if (order > 0 || (order === 0 && (range.lowerOpen || range.upperOpen))) {
      throw idbError("DataError", "The lower key is greater than the upper key.");
    }
    return new IDBKeyRange(token, range);
  }
}
const within = (range, key) => {
  if (range.lower !== null) {
    const order = compareKeys(key, range.lower);
    if (order < 0 || (order === 0 && range.lowerOpen)) return false;
  }
  if (range.upper !== null) {
    const order = compareKeys(key, range.upper);
    if (order > 0 || (order === 0 && range.upperOpen)) return false;
  }
  return true;
};
const everything = { lower: null, upper: null, lowerOpen: false, upperOpen: false };
// rangeOf is the range a query names: a range, a key, or all of them
// for none unless needed.
const rangeOf = (query, needed) => {
  if (query instanceof IDBKeyRange) return idbState.get(query);
  if (query === undefined || query === null) {
    if (needed) throw idbError("DataError", "No key or key range specified.");
    return everything;
  }
  const key = validKey(query);
  return { lower: key, upper: key, lowerOpen: false, upperOpen: false };
};
const putRange = (range) => {
  putBytes(range.lower);
  putBytes(range.upper);
  putByte((range.lower !== null && range.lowerOpen ? 1 : 0) | (range.upper !== null && range.upperOpen ? 2 : 0));
};
const limitOf = (count) => {
  if (count === undefined) return 0;
  const limit = Number(count);
  if (!Number.isFinite(limit) || limit < 0 || limit > 4294967295) throw new TypeError("The count is not an unsigned long.");
  return Math.trunc(limit);
};

// ---- object stores and indexes

// reading checks a handle of a store or an index may be read, and
// gives its transaction; writing, that it may be written too.
const reading = (handle) => {
  if (handle.deleted || (handle.store && handle.store.deleted)) throw idbError("InvalidStateError", "The object store or the index has been deleted.");
  const tx = handle.tx || handle.store.tx;
  if (tx.state !== "active") throw inactive();
  return tx;
};
const writing = (handle) => {
  const tx = reading(handle);
  if (tx.mode === 0) throw idbError("ReadOnlyError", "The transaction is read-only.");
  return tx;
};
// upgrading checks a handle is of the transaction of an upgrade that
// is active.
const upgrading = (handle) => {
  const tx = handle.tx || handle.store.tx;
  if (tx.mode !== 2) throw idbError("InvalidStateError", "The database is not running a version change transaction.");
  return reading(handle);
};
const sourceOf = (handle) => (handle.store ? [handle.store.schema.id, handle.schema.id] : [handle.schema.id, 0]);
const DIRECTIONS = ["next", "nextunique", "prev", "prevunique"];
const WANT = { key: 1, primary: 2, value: 4 };

// The reads a store and an index share: handle is the state of either.
const reads = {
  get(handle, query, wanted) {
    const tx = reading(handle), range = rangeOf(query, true), [store, index] = sourceOf(handle);
    return placed(tx, handle.object, () => {
      wireAt = 0;
      putRange(range);
      sent("get", tx.id, store, index, wanted);
      if (!takeByte()) return undefined;
      takeBytes();
      const primary = takeBytes();
      return wanted === WANT.value ? takeValue() : keyValue(primary);
    });
  },
  all(handle, query, count, wanted) {
    const tx = reading(handle), range = rangeOf(query, false), limit = limitOf(count), [store, index] = sourceOf(handle);
    return placed(tx, handle.object, () => {
      wireAt = 0;
      putRange(range);
      sent("all", tx.id, store, index, limit, wanted);
      const all = new Array(takeCount());
      for (let i = 0; i < all.length; i++) all[i] = wanted === WANT.value ? takeValue() : keyValue(takeBytes());
      return all;
    });
  },
  count(handle, query) {
    const tx = reading(handle), range = rangeOf(query, false), [store, index] = sourceOf(handle);
    return placed(tx, handle.object, () => {
      wireAt = 0;
      putRange(range);
      return sent("count", tx.id, store, index);
    });
  },
  cursor(handle, query, direction, keyOnly) {
    const tx = reading(handle), range = rangeOf(query, false);
    direction = direction === undefined ? "next" : String(direction);
    if (!DIRECTIONS.includes(direction)) throw new TypeError(`'${direction}' is not a direction of a cursor.`);
    const cursor = idbState.get(new (keyOnly ? IDBCursor : IDBCursorWithValue)(token, {
      tx, source: handle, range, direction, keyOnly, position: null, primary: null, got: false,
      key: undefined, primaryKey: undefined, value: undefined, request: null,
    }));
    const request = placed(tx, handle.object, () => step(cursor, null, null, 1));
    cursor.request = idbState.get(request);
    return request;
  },
};
// step moves a cursor: by count records, or to a key and the key of a
// record. It gives what shows the move once the request's event is due:
// the cursor, or null past the last record.
const step = (cursor, key, primary, count) => {
  const [store, index] = sourceOf(cursor.source);
  wireAt = 0;
  putRange(cursor.range);
  putBytes(cursor.position);
  putBytes(cursor.primary);
  putBytes(key);
  putBytes(primary);
  sent("step", cursor.tx.id, store, index, DIRECTIONS.indexOf(cursor.direction), count, !cursor.keyOnly);
  let arrive;
  if (!takeByte()) {
    cursor.position = cursor.primary = null;
    arrive = () => {
      cursor.key = cursor.primaryKey = cursor.value = undefined;
      return null;
    };
  } else {
    cursor.position = takeBytes().slice();
    cursor.primary = takeBytes().slice();
    const value = cursor.keyOnly ? undefined : takeValue();
    arrive = () => {
      cursor.key = keyValue(cursor.position);
      cursor.primaryKey = keyValue(cursor.primary);
      cursor.value = value;
      cursor.got = true;
      return cursor.object;
    };
  }
  arrive.arrival = true;
  return arrive;
};

// cloned is the structured clone of a value to store: its transaction
// is not active while the clone reads the value.
const cloned = (tx, value) => {
  tx.state = "inactive";
  try {
    return copied(value, new Map());
  } finally {
    tx.state = "active";
  }
};
// saving places the request that keeps a value in a store: under key,
// the key at the store's key path, or one the store makes.
const saving = (handle, source, tx, value, key, add) => {
  const schema = handle.schema, path = schema.keyPath;
  let clone, make = false;
  if (key === null) {
    clone = cloned(tx, value);
    const found = along(clone, path);
    if (found !== MISSING) key = validKey(found);
    else if (!schema.autoIncrement || Array.isArray(path) || !injectable(clone, path)) {
      throw idbError("DataError", "The value has no key at the object store's key path, and one cannot be put there.");
    } else make = true;
  } else clone = cloned(tx, value);
  return placed(tx, source, () => {
    if (make) {
      const next = based("nextKey", tx.id, schema.id);
      inject(clone, path, next);
      key = keyOf(next);
    }
    const lists = [];
    for (const index of schema.indexes.values()) {
      const keys = listed(clone, index);
      if (keys.length) lists.push([index.id, keys]);
    }
    wireAt = 0;
    putBytes(key === undefined ? null : key);
    packValue(clone);
    putCount(lists.length);
    for (const [index, keys] of lists) {
      putCount(index);
      putCount(keys.length);
      keys.forEach(putBytes);
    }
    sent("put", tx.id, schema.id, 0, add);
    return keyValue(takeBytes());
  });
};
const keep = (handle, value, key, add) => {
  const tx = writing(handle), schema = handle.schema;
  if (schema.keyPath !== null && key !== undefined) throw idbError("DataError", "The object store has a key path: no key goes with the value.");
  if (schema.keyPath === null && !schema.autoIncrement && key === undefined) throw idbError("DataError", "The object store has no key path and makes no key: one goes with the value.");
  return saving(handle, handle.object, tx, value, schema.keyPath !== null ? null : key === undefined ? undefined : validKey(key), add);
};

class IDBObjectStore {
  constructor(key, state) {
    if (key !== token) illegal();
    idbState.set(this, Object.assign(state, { object: this, deleted: false, indexes: new Map(), made: [] }));
  }
  get name() { return idbOf(this, IDBObjectStore).schema.name; }
  set name(name) {
    const handle = idbOf(this, IDBObjectStore), tx = upgrading(handle), schema = handle.schema;
    name = String(name);
    if (name === schema.name) return;
    based("renameStore", tx.id, schema.id, name);
    tx.db.stores.delete(schema.name);
    schema.name = name;
    tx.db.stores.set(name, schema);
  }
  get keyPath() { return samePath(idbOf(this, IDBObjectStore).schema.keyPath); }
  get indexNames() { return namesOf(idbOf(this, IDBObjectStore).schema.indexes.keys()); }
  get transaction() { return idbOf(this, IDBObjectStore).tx.object; }
  get autoIncrement() { return idbOf(this, IDBObjectStore).schema.autoIncrement; }
  put(value, key) {
    if (arguments.length === 0) throw new TypeError("put needs a value");
    return keep(idbOf(this, IDBObjectStore), value, key, false);
  }
  add(value, key) {
    if (arguments.length === 0) throw new TypeError("add needs a value");
    return keep(idbOf(this, IDBObjectStore), value, key, true);
  }
  delete(query) {
    if (arguments.length === 0) throw new TypeError("delete needs a key or a range");
    const handle = idbOf(this, IDBObjectStore), tx = writing(handle), range = rangeOf(query, true);
    return placed(tx, this, () => {
      wireAt = 0;
      putRange(range);
      sent("remove", tx.id, handle.schema.id, 0);
      return undefined;
    });
  }
  clear() {
    const handle = idbOf(this, IDBObjectStore), tx = writing(handle);
    return placed(tx, this, () => {
      based("empty", tx.id, handle.schema.id);
      return undefined;
    });
  }
  get(query) {
    if (arguments.length === 0) throw new TypeError("get needs a key or a range");
    return reads.get(idbOf(this, IDBObjectStore), query, WANT.value);
  }
  getKey(query) {
    if (arguments.length === 0) throw new TypeError("getKey needs a key or a range");
    return reads.get(idbOf(this, IDBObjectStore), query, WANT.primary);
  }
  getAll(query, count) { return reads.all(idbOf(this, IDBObjectStore), query, count, WANT.value); }
  getAllKeys(query, count) { return reads.all(idbOf(this, IDBObjectStore), query, count, WANT.primary); }
  count(query) { return reads.count(idbOf(this, IDBObjectStore), query); }
  openCursor(query, direction) { return reads.cursor(idbOf(this, IDBObjectStore), query, direction, false); }
  openKeyCursor(query, direction) { return reads.cursor(idbOf(this, IDBObjectStore), query, direction, true); }
  index(name) {
    const handle = idbOf(this, IDBObjectStore);
    name = String(name);
    if (handle.deleted || handle.tx.state === "finished") throw idbError("InvalidStateError", "The object store has been deleted or its transaction has finished.");
    const schema = handle.schema.indexes.get(name);
    if (!schema) throw idbError("NotFoundError", "The specified index was not found.");
    let index = handle.indexes.get(schema.id);
    if (!index) {
      index = idbState.get(new IDBIndex(token, { store: handle, schema }));
      handle.indexes.set(schema.id, index);
      handle.made.push(index);
    }
    return index.object;
  }
  createIndex(name, keyPath, options = {}) {
    if (arguments.length < 2) throw new TypeError("createIndex needs a name and a key path");
    const handle = idbOf(this, IDBObjectStore), tx = upgrading(handle), schema = handle.schema;
    name = String(name);
    if (schema.indexes.has(name)) throw idbError("ConstraintError", "An index with the specified name already exists.");
    const path = pathOfKeys(keyPath);
    if (path === null) throw idbError("SyntaxError", "The keyPath argument contains an invalid key path.");
    const unique = Boolean(options && options.unique), multiEntry = Boolean(options && options.multiEntry);
    if (Array.isArray(path) && multiEntry) throw idbError("InvalidAccessError", "An index of several key paths lists no record under each item of an array.");
    const index = { name, keyPath: path, unique, multiEntry };
    index.id = based("createIndex", tx.id, schema.id, name, JSON.stringify(path), unique, multiEntry);
    schema.indexes.set(name, index);
    try {
      fill(tx, schema, index);
    } catch (error) {
      tx.failed = unknown(error);
    }
    return this.index(name);
  }
  deleteIndex(name) {
    if (arguments.length === 0) throw new TypeError("deleteIndex needs a name");
    const handle = idbOf(this, IDBObjectStore), tx = upgrading(handle), schema = handle.schema;
    name = String(name);
    const index = schema.indexes.get(name);
    if (!index) throw idbError("NotFoundError", "The specified index was not found.");
    based("deleteIndex", tx.id, schema.id, index.id);
    schema.indexes.delete(name);
    const made = handle.indexes.get(index.id);
    if (made) made.deleted = true;
    handle.indexes.delete(index.id);
  }
}
// fill lists the records a store holds already in an index made now.
const fill = (tx, store, index) => {
  wireAt = 0;
  putRange(everything);
  sent("all", tx.id, store.id, 0, 0, WANT.primary | WANT.value);
  const lists = [];
  for (let left = takeCount(); left > 0; left--) {
    const primary = takeBytes().slice();
    const keys = listed(takeValue(), index);
    if (keys.length) lists.push([primary, keys]);
  }
  for (const [primary, keys] of lists) {
    wireAt = 0;
    putBytes(primary);
    putCount(keys.length);
    keys.forEach(putBytes);
    sent("list", tx.id, store.id, index.id);
  }
};

class IDBIndex {
  constructor(key, state) {
    if (key !== token) illegal();
    idbState.set(this, Object.assign(state, { object: this, deleted: false }));
  }
  get name() { return idbOf(this, IDBIndex).schema.name; }
  set name(name) {
    const handle = idbOf(this, IDBIndex), tx = upgrading(handle), schema = handle.schema, store = handle.store.schema;
    name = String(name);
    if (name === schema.name) return;
    based("renameIndex", tx.id, store.id, schema.id, name);
    store.indexes.delete(schema.name);
    schema.name = name;
    store.indexes.set(name, schema);
  }
  get objectStore() { return idbOf(this, IDBIndex).store.object; }
  get keyPath() { return samePath(idbOf(this, IDBIndex).schema.keyPath); }
  get multiEntry() { return idbOf(this, IDBIndex).schema.multiEntry; }
  get unique() { return idbOf(this, IDBIndex).schema.unique; }
  get(query) {
    if (arguments.length === 0) throw new TypeError("get needs a key or a range");
    return reads.get(idbOf(this, IDBIndex), query, WANT.value);
  }
  getKey(query) {
    if (arguments.length === 0) throw new TypeError("getKey needs a key or a range");
    return reads.get(idbOf(this, IDBIndex), query, WANT.primary);
  }
  getAll(query, count) { return reads.all(idbOf(this, IDBIndex), query, count, WANT.value); }
  getAllKeys(query, count) { return reads.all(idbOf(this, IDBIndex), query, count, WANT.primary); }
  count(query) { return reads.count(idbOf(this, IDBIndex), query); }
  openCursor(query, direction) { return reads.cursor(idbOf(this, IDBIndex), query, direction, false); }
  openKeyCursor(query, direction) { return reads.cursor(idbOf(this, IDBIndex), query, direction, true); }
}

// ---- cursors

// moving checks a cursor may move, and gives its transaction.
const moving = (cursor) => {
  if (cursor.tx.state !== "active") throw inactive();
  if (cursor.source.deleted || (cursor.source.store && cursor.source.store.deleted)) throw idbError("InvalidStateError", "The cursor's source has been deleted.");
  if (!cursor.got) throw idbError("InvalidStateError", "The cursor is moving, or is past its last record.");
  return cursor.tx;
};
const shifted = (cursor, key, primary, count) => {
  cursor.got = false;
  placed(cursor.tx, cursor.source.object, () => step(cursor, key, primary, count), cursor.request);
};
// ahead checks a key a cursor is told to go to is ahead of where it is.
const ahead = (cursor, key, primary) => {
  const order = compareKeys(key, cursor.position) || compareKeys(primary, cursor.primary);
  if (cursor.direction.startsWith("prev") ? order >= 0 : order <= 0) {
    throw idbError("DataError", "The key is not ahead of the cursor, the way it goes.");
  }
};
// changing checks the record a cursor is on may be changed.
const changing = (cursor) => {
  if (cursor.tx.state !== "active") throw inactive();
  if (cursor.tx.mode === 0) throw idbError("ReadOnlyError", "The transaction is read-only.");
  if (cursor.source.deleted || (cursor.source.store && cursor.source.store.deleted)) throw idbError("InvalidStateError", "The cursor's source has been deleted.");
  if (!cursor.got || cursor.keyOnly) throw idbError("InvalidStateError", "The cursor is on no record, or reads keys alone.");
  return cursor.source.store || cursor.source;
};

class IDBCursor {
  constructor(key, state) {
    if (key !== token) illegal();
    idbState.set(this, Object.assign(state, { object: this }));
  }
  get source() { return idbOf(this, IDBCursor).source.object; }
  get direction() { return idbOf(this, IDBCursor).direction; }
  get key() { return idbOf(this, IDBCursor).key; }
  get primaryKey() { return idbOf(this, IDBCursor).primaryKey; }
  get request() { return idbOf(this, IDBCursor).request.object; }
  advance(count) {
    const cursor = idbOf(this, IDBCursor);
    count = Number(count);
    if (!Number.isFinite(count) || count < 0 || count > 4294967295) throw new TypeError("The count is not an unsigned long.");
    if (Math.trunc(count) === 0) throw new TypeError("A cursor advances by one record at least.");
    moving(cursor);
    shifted(cursor, null, null, Math.trunc(count));
  }
  continue(key) {
    const cursor = idbOf(this, IDBCursor);
    moving(cursor);
    let to = null;
    if (key !== undefined) {
      to = validKey(key);
      const order = compareKeys(to, cursor.position);
      if (cursor.direction.startsWith("prev") ? order >= 0 : order <= 0) {
        throw idbError("DataError", "The key is not ahead of the cursor, the way it goes.");
      }
    }
    shifted(cursor, to, null, 1);
  }
  continuePrimaryKey(key, primaryKey) {
    if (arguments.length < 2) throw new TypeError("continuePrimaryKey needs a key and a primary key");
    const cursor = idbOf(this, IDBCursor);
    if (cursor.tx.state !== "active") throw inactive();
    if (cursor.source.deleted || (cursor.source.store && cursor.source.store.deleted)) throw idbError("InvalidStateError", "The cursor's source has been deleted.");
    if (!cursor.source.store || cursor.direction.endsWith("unique")) {
      throw idbError("InvalidAccessError", "Only a cursor of an index that goes record by record continues to a primary key.");
    }
    if (!cursor.got) throw idbError("InvalidStateError", "The cursor is moving, or is past its last record.");
    const to = validKey(key), primary = validKey(primaryKey);
    ahead(cursor, to, primary);
    shifted(cursor, to, primary, 1);
  }
  update(value) {
    if (arguments.length === 0) throw new TypeError("update needs a value");
    const cursor = idbOf(this, IDBCursor), store = changing(cursor), tx = cursor.tx, path = store.schema.keyPath;
    const primary = cursor.primary;
    if (path === null) return saving(store, this, tx, value, primary, false);
    const clone = cloned(tx, value), found = along(clone, path);
    if (found === MISSING || compareKeys(validKey(found), primary) !== 0) {
      throw idbError("DataError", "The value's key is not the key of the record the cursor is on.");
    }
    return saving({ schema: { ...store.schema, keyPath: null } }, this, tx, clone, primary, false);
  }
  delete() {
    const cursor = idbOf(this, IDBCursor), store = changing(cursor), tx = cursor.tx, primary = cursor.primary;
    return placed(tx, this, () => {
      wireAt = 0;
      putRange({ lower: primary, upper: primary, lowerOpen: false, upperOpen: false });
      sent("remove", tx.id, store.schema.id, 0);
      return undefined;
    });
  }
}
class IDBCursorWithValue extends IDBCursor {
  get value() { return idbOf(this, IDBCursorWithValue).value; }
}

// ---- databases

class IDBDatabase extends EventTarget {
  constructor(key, state) {
    if (key !== token) illegal();
    super();
    idbState.set(this, Object.assign(state, { object: this, closing: false, upgrade: null, txs: new Set() }));
  }
  get name() { return idbOf(this, IDBDatabase).name; }
  get version() { return idbOf(this, IDBDatabase).version; }
  get objectStoreNames() { return namesOf(idbOf(this, IDBDatabase).stores.keys()); }
  createObjectStore(name, options = {}) {
    if (arguments.length === 0) throw new TypeError("createObjectStore needs a name");
    const db = idbOf(this, IDBDatabase), tx = changingSchema(db);
    name = String(name);
    const keyPath = pathOfKeys(options === null ? undefined : options.keyPath);
    if (db.stores.has(name)) throw idbError("ConstraintError", "An object store with the specified name already exists.");
    const autoIncrement = Boolean(options && options.autoIncrement);
    if (autoIncrement && (keyPath === "" || Array.isArray(keyPath))) {
      throw idbError("InvalidAccessError", "An object store that makes its keys has no empty key path and no list of them.");
    }
    const id = based("createStore", tx.id, name, JSON.stringify(keyPath), autoIncrement);
    db.stores.set(name, { id, name, keyPath, autoIncrement, indexes: new Map() });
    return tx.object.objectStore(name);
  }
  deleteObjectStore(name) {
    if (arguments.length === 0) throw new TypeError("deleteObjectStore needs a name");
    const db = idbOf(this, IDBDatabase), tx = changingSchema(db);
    name = String(name);
    const schema = db.stores.get(name);
    if (!schema) throw idbError("NotFoundError", "The specified object store was not found.");
    based("deleteStore", tx.id, schema.id);
    db.stores.delete(name);
    const handle = tx.named.get(schema.id);
    if (handle) handle.deleted = true;
    tx.named.delete(schema.id);
  }
  transaction(names, mode = "readonly", options = {}) {
    if (arguments.length === 0) throw new TypeError("transaction needs the names of its object stores");
    const db = idbOf(this, IDBDatabase);
    if (db.upgrade) throw idbError("InvalidStateError", "A version change transaction is running.");
    if (db.closing) throw idbError("InvalidStateError", "The database connection is closing.");
    const scope = Array.from(new Set(typeof names === "string" || names === null || typeof names !== "object" || typeof names[Symbol.iterator] !== "function"
      ? [String(names)] : Array.from(names, String))).sort();
    if (scope.some((name) => !db.stores.has(name))) throw idbError("NotFoundError", "One of the specified object stores was not found.");
    if (scope.length === 0) throw idbError("InvalidAccessError", "A transaction needs an object store.");
    mode = String(mode);
    if (mode !== "readonly" && mode !== "readwrite") throw new TypeError(`'${mode}' is not a mode of a transaction.`);
    const durability = options === null || options === undefined || options.durability === undefined ? "default" : String(options.durability);
    if (!["default", "strict", "relaxed"].includes(durability)) throw new TypeError(`'${durability}' is not a durability.`);
    const [id, started] = based("begin", db.id, scope, modes.indexOf(mode));
    const tx = { id, db, mode: modes.indexOf(mode), names: scope, durability, started, opening: null, before: null };
    fresh.push(tx);
    const made = new IDBTransaction(token, tx);
    wake();
    return made;
  }
  close() {
    const db = idbOf(this, IDBDatabase);
    if (db.closing) return;
    db.closing = true;
    connections.delete(db.id);
    based("shut", db.id);
  }
}
withHandlers(IDBDatabase.prototype, ["abort", "close", "error", "versionchange"]);
// changingSchema checks a database is in an upgrade that is active,
// and gives its transaction.
const changingSchema = (db) => {
  if (!db.upgrade) throw idbError("InvalidStateError", "The database is not running a version change transaction.");
  if (db.upgrade.state !== "active") throw inactive();
  return db.upgrade;
};

// connections are the open connections by the number light knows each
// by, and opening the requests to open or delete that wait for light.
const connections = new Map();
const opening = new Map();
const pathFrom = (text) => JSON.parse(text);
const connected = (id, info) => {
  const stores = new Map((info.stores || []).map((store) => [store.name, {
    id: store.id, name: store.name, keyPath: pathFrom(store.keyPath), autoIncrement: store.auto,
    indexes: new Map((store.indexes || []).map((index) => [index.name, {
      id: index.id, name: index.name, keyPath: pathFrom(index.keyPath), unique: index.unique, multiEntry: index.multi,
    }])),
  }]));
  const db = idbState.get(new IDBDatabase(token, { id, name: info.name, version: info.version, stores }));
  connections.set(id, db);
  return db;
};
// refused is the task of a request to open that failed.
const refused = (request, error) => {
  request.done = true;
  request.result = undefined;
  request.error = error;
  return firing([request.object], new Event("error", { bubbles: true, cancelable: true }), () => {});
};
// told is what light tells of the databases, each a task or what a
// transaction waited for.
const outcomes = {
  upgrade(request, e) {
    const db = connected(e.conn, e.database);
    const tx = { id: e.tx, db, mode: 2, names: [], durability: "default", started: true, opening: request };
    tx.before = { version: e.old, stores: copySchema(db.stores) };
    new IDBTransaction(token, tx);
    db.upgrade = tx;
    request.connection = db;
    request.result = db.object;
    request.tx = tx;
    request.done = true;
    request.error = null;
    tx.state = "active";
    const event = new IDBVersionChangeEvent("upgradeneeded", { oldVersion: e.old, newVersion: e.new });
    return firing([request.object], event, (threw) => {
      if (tx.state === "active") tx.state = "inactive";
      if (threw) abortTx(tx, idbError("AbortError", "A listener of the upgrade threw."));
      proceed(tx);
    });
  },
  opened(request, e) {
    const db = request.connection || connected(e.conn, e.database);
    request.tx = null;
    if (db.closing) return refused(request, idbError("AbortError", "The connection was closed."));
    request.done = true;
    request.result = db.object;
    request.error = null;
    return firing([request.object], new Event("success"), () => {});
  },
  failed(request, e) {
    if (request.connection) {
      request.connection.closing = true;
      connections.delete(request.connection.id);
    }
    request.tx = null;
    return refused(request, idbError(e.name, e.message));
  },
  deleted(request, e) {
    request.done = true;
    request.result = undefined;
    request.error = null;
    return firing([request.object], new IDBVersionChangeEvent("success", { oldVersion: e.old, newVersion: null }), () => {});
  },
  blocked(request, e) {
    return firing([request.object], new IDBVersionChangeEvent("blocked", { oldVersion: e.old, newVersion: e.new || null }), () => {});
  },
};
const heard = (id) => {
  try {
    light("idb", "heard", id);
  } catch (_) { /* the connection is no more */ }
};
const idbCalls = {
  // told takes note of what light tells: an event to come, or the
  // stores a transaction waited for.
  told(e) {
    if (e.kind === "started") {
      const tx = transactions.get(e.tx);
      if (!tx || tx.started) return;
      tx.started = true;
      for (const act of tx.waiting.splice(0)) act();
      return;
    }
    if (e.kind === "versionchange") {
      later(() => {
        const db = connections.get(e.conn);
        if (!db || db.closing) {
          heard(e.conn);
          return false;
        }
        const event = new IDBVersionChangeEvent("versionchange", { oldVersion: e.old, newVersion: e.new || null });
        return firing([db.object], event, () => heard(e.conn));
      });
      return;
    }
    if (e.kind === "closed") {
      later(() => {
        const db = connections.get(e.conn);
        if (!db) return false;
        db.closing = true;
        connections.delete(e.conn);
        for (const tx of Array.from(db.txs)) abortTx(tx, idbError("AbortError", "The database was cleared."));
        later(() => firing([db.object], new Event("close"), () => {}));
        return false;
      });
      return;
    }
    const request = opening.get(e.request);
    if (!request) return;
    if (e.kind !== "upgrade" && e.kind !== "blocked") opening.delete(e.request);
    later(() => outcomes[e.kind](request, e));
  },
  // run runs the tasks that wait, up to one whose listeners ran: light
  // calls again for the next, and is told when none is left.
  run() {
    while (idbTasks.length) {
      if (idbTasks.shift()()) return true;
    }
    idbAwake = false;
    return false;
  },
};

// entered is what light calls the page by: it settles the databases as
// light enters, unless it enters while a script of the page runs, and
// takes the calls that are theirs.
let entries = 0, scripts = 0;
const entered = (calls) => function light(call, ...args) {
  if (entries === 0 && scripts === 0) settle();
  if (call === "script") scripts = Math.max(0, scripts + (args[1] ? -1 : 1));
  if (call === "idb") return idbCalls[args[0]](...args.slice(1));
  entries++;
  try {
    return calls(call, ...args);
  } finally {
    entries--;
  }
};

const sandboxed = (what) => {
  if (opaque) throw idbError("SecurityError", `${what}: the document is sandboxed and lacks the 'allow-same-origin' flag.`);
};
const versionOf = (version) => {
  if (version === undefined) return 0;
  const number = Number(version);
  if (!Number.isFinite(number) || Math.trunc(number) < 1 || number > Number.MAX_SAFE_INTEGER) {
    throw new TypeError("The version of a database is a whole number from 1.");
  }
  return Math.trunc(number);
};
class IDBFactory {
  constructor(key) { if (key !== token) illegal(); }
  open(name, version) {
    if (arguments.length === 0) throw new TypeError("open needs a name");
    const wanted = versionOf(version);
    sandboxed("Failed to execute 'open' on 'IDBFactory'");
    const request = new IDBOpenDBRequest(token);
    opening.set(based("open", String(name), wanted), idbState.get(request));
    return request;
  }
  deleteDatabase(name) {
    if (arguments.length === 0) throw new TypeError("deleteDatabase needs a name");
    sandboxed("Failed to execute 'deleteDatabase' on 'IDBFactory'");
    const request = new IDBOpenDBRequest(token);
    opening.set(based("delete", String(name)), idbState.get(request));
    return request;
  }
  databases() {
    return new Promise((resolve, reject) => {
      if (opaque) reject(idbError("SecurityError", "The document is sandboxed and lacks the 'allow-same-origin' flag."));
      else later(() => { resolve(based("databases")); return true; });
    });
  }
  cmp(first, second) {
    if (arguments.length < 2) throw new TypeError("cmp needs two keys");
    return compareKeys(validKey(first), validKey(second));
  }
}
const indexedDB = new IDBFactory(token);

// StorageManager says what the site's databases hold of what they may.
class StorageManager {
  constructor(key) { if (key !== token) illegal(); }
  estimate() {
    return new Promise((resolve) => {
      sandboxed("Failed to execute 'estimate' on 'StorageManager'");
      const [usage, quota] = based("usage");
      resolve({ usage, quota, usageDetails: { indexedDB: usage } });
    });
  }
  persisted() {
    return new Promise((resolve) => {
      sandboxed("Failed to execute 'persisted' on 'StorageManager'");
      resolve(based("usage")[2]);
    });
  }
  persist() { return this.persisted(); }
}
navigator.storage = new StorageManager(token);

define(window, {
  IDBFactory, IDBDatabase, IDBTransaction, IDBObjectStore, IDBIndex, IDBCursor, IDBCursorWithValue, IDBKeyRange,
  IDBRequest, IDBOpenDBRequest, IDBVersionChangeEvent, DOMStringList, StorageManager,
});
accessors(window, { indexedDB: { get() { return indexedDB; } } });
