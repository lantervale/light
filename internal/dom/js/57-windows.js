// ---- the windows of other frames

// written is value as another document reads it back (read): its
// structured clone, written down as a text and the bytes of what it
// holds that is no text. Each object met is numbered in the order it is
// met, so one met again, or within itself, is named by its number, and
// a port by its place among those that leave. Nothing is taken from the
// page until given says so, and nothing of the page goes with it.
const written = (value, transfer) => {
  const leaving = [], buffers = [];
  for (const item of transfer) {
    if (item instanceof MessagePort) {
      if (portOf(item).detached) throw uncloneable("A port that was closed or transferred");
      leaving.push(item);
    } else if (item instanceof ArrayBuffer) {
      if (item.detached) throw uncloneable("An ArrayBuffer that was transferred");
      buffers.push(item);
    } else throw uncloneable("A value that cannot be transferred");
  }
  if (new Set(transfer).size !== transfer.length) throw uncloneable("A value listed twice to transfer");
  const seen = new Map(), chunks = [];
  let size = 0;
  const note = (object) => { seen.set(object, seen.size); };
  const bytes = (view) => {
    const at = size;
    chunks.push(view);
    size += view.length;
    return [at, view.length];
  };
  const pairs = (object) => Object.keys(object).map((key) => [key, walk(object[key])]);
  const walk = (v) => {
    switch (typeof v) {
      case "undefined": return ["u"];
      case "boolean": case "string": return v;
      case "number": return Number.isFinite(v) && !Object.is(v, -0) ? v : ["n", Object.is(v, -0) ? "-0" : String(v)];
      case "bigint": return ["i", v.toString()];
      case "function": throw uncloneable("A function");
      case "symbol": throw uncloneable("A symbol");
    }
    if (v === null) return null;
    if (v instanceof MessagePort) {
      if (!leaving.includes(v)) throw uncloneable("A MessagePort that is not transferred");
      return ["P", leaving.indexOf(v)];
    }
    if (seen.has(v)) return ["r", seen.get(v)];
    if (v instanceof Boolean || v instanceof Number || v instanceof String || v instanceof BigInt) {
      note(v);
      return ["p", walk(v.valueOf())];
    }
    if (v instanceof Date) { note(v); return ["d", walk(v.getTime())]; }
    if (v instanceof RegExp) { note(v); return ["x", v.source, v.flags]; }
    if (v instanceof ArrayBuffer) {
      if (v.detached) throw uncloneable("An ArrayBuffer that was transferred");
      note(v);
      return ["b", ...bytes(new Uint8Array(v))];
    }
    if (ArrayBuffer.isView(v)) {
      if (!(v.buffer instanceof ArrayBuffer)) throw uncloneable("A view of shared memory");
      const buffer = walk(v.buffer);
      note(v);
      if (v instanceof DataView) return ["v", "DataView", buffer, v.byteOffset, v.byteLength];
      const kind = viewKinds.find((view) => v instanceof view);
      if (!kind) throw uncloneable("A view");
      return ["v", kind.name, buffer, v.byteOffset, v.length];
    }
    if (v instanceof Map) { note(v); return ["m", Array.from(v).map(([key, item]) => [walk(key), walk(item)])]; }
    if (v instanceof Set) { note(v); return ["s", Array.from(v).map(walk)]; }
    if (v instanceof DOMException) { note(v); return ["e", "DOMException", String(v.message), String(v.name)]; }
    if (v instanceof Error) {
      note(v);
      const kind = Object.hasOwn(errorKinds, v.name) ? v.name : "Error";
      const stack = typeof v.stack === "string" ? v.stack : null;
      return Object.hasOwn(v, "cause") ? ["e", kind, String(v.message), stack, walk(v.cause)] : ["e", kind, String(v.message), stack];
    }
    if (v instanceof File) { note(v); return ["F", String(v.name), String(v.type), Number(v.lastModified), ...bytes(v._bytes)]; }
    if (v instanceof Blob) { note(v); return ["B", String(v.type), ...bytes(v._bytes)]; }
    if (v instanceof EventTarget || v instanceof Event || platform.has(v) || facades.has(v)) throw uncloneable("An object of the page");
    if (v instanceof WeakMap || v instanceof WeakSet || v instanceof WeakRef || v instanceof Promise || v instanceof SharedBuffer) {
      throw uncloneable(`A ${v.constructor && v.constructor.name || "value"}`);
    }
    note(v);
    return Array.isArray(v) ? ["a", v.length, pairs(v)] : ["o", pairs(v)];
  };
  const text = JSON.stringify(walk(value));
  return { text, bytes: size ? joined(chunks) : null, leaving, buffers };
};

// remotes are the ports whose other end is in another document, by
// the number light knows their own end by.
const remotes = new Map();

// leave takes a port out of this document, to go with a message: the
// number of the end that goes. A port whose other end is here gets a
// channel through light, that end kept; what waited in the port goes
// with it.
const leave = (port) => {
  const state = portOf(port);
  let end = state.remote;
  if (end) {
    remotes.delete(end);
    for (const m of state.queue) host("channel", "requeue", end, ...given(written(m.data, m.ports)));
  } else {
    const pair = host("channel", "pipe");
    end = pair[1];
    const other = state.other;
    if (other) {
      const kept = portOf(other);
      kept.other = null;
      kept.remote = pair[0];
      remotes.set(pair[0], other);
      for (const m of state.queue) host("channel", "send", pair[0], ...given(written(m.data, m.ports)));
    } else host("channel", "shut", pair[0]);
  }
  state.queue = [];
  state.other = null;
  state.remote = 0;
  state.detached = true;
  return end;
};

// given is what light is handed of a message written down, once what
// it transfers has left the page: its text, its bytes and its ends.
const given = (w) => {
  const ends = w.leaving.map(leave);
  for (const buffer of w.buffers) buffer.transfer();
  return [w.text, w.bytes ? shared(w.bytes) : null, ends];
};

// taken are the bytes of the message light is telling the page of.
const taken = (size) => {
  if (!size) return new Uint8Array(0);
  const buffer = new SharedBuffer(size);
  host("window", "bytes", buffer);
  return new Uint8Array(buffer);
};

// marks are the marks read knows, those of frames.Carries.
const marks = ["u", "n", "i", "r", "p", "d", "x", "b", "v", "m", "s", "e", "a", "o", "B", "F", "P"];

// read is the value another document wrote down, made of this page's
// own objects alone, and the ports that came with it, each the holder
// of one of ends. A text that is not one written throws.
const read = (text, bytes, ends) => {
  const made = [];
  const ports = ends.map((end) => {
    const port = new MessagePort(token);
    portOf(port).remote = end;
    remotes.set(end, port);
    return port;
  });
  const keep = (object) => { made.push(object); return object; };
  const wrong = () => new TypeError("The message is not one light wrote down.");
  const count = (n, most) => {
    if (!Number.isInteger(n) || n < 0 || n > most) throw wrong();
    return n;
  };
  const list = (items) => {
    if (!Array.isArray(items)) throw wrong();
    return items;
  };
  const cut = (at, length) => {
    count(at, bytes.length);
    return bytes.slice(at, at + count(length, bytes.length - at));
  };
  const fill = (object, entries) => {
    for (const entry of list(entries)) {
      if (list(entry).length !== 2 || typeof entry[0] !== "string") throw wrong();
      put(object, entry[0], build(entry[1]));
    }
    return object;
  };
  const build = (n) => {
    if (n === null || typeof n === "boolean" || typeof n === "string") return n;
    if (typeof n === "number") return n;
    const [kind, a, b, c, d, e] = list(n);
    switch (kind) {
      case "u": return undefined;
      case "n": if (!["NaN", "Infinity", "-Infinity", "-0"].includes(a)) throw wrong(); return Number(a);
      case "i": if (typeof a !== "string" || !/^-?\d+$/.test(a)) throw wrong(); return BigInt(a);
      case "r": return made[count(a, made.length - 1)];
      case "P": return ports[count(a, ports.length - 1)];
      case "p": {
        const value = build(a);
        if (value === null || typeof value === "object" || value === undefined) throw wrong();
        return keep(Object(value));
      }
      case "d": return keep(new Date(Number(build(a))));
      case "x": return keep(new RegExp(String(a), String(b)));
      case "b": return keep(cut(a, b).buffer);
      case "B": return keep(new Blob([cut(b, c)], { type: String(a) }));
      case "F": return keep(new File([cut(d, e)], String(a), { type: String(b), lastModified: Number(c) }));
      case "v": {
        const buffer = build(b);
        if (!(buffer instanceof ArrayBuffer)) throw wrong();
        const view = a === "DataView" ? DataView : viewKinds.find((one) => one.name === a);
        if (!view) throw wrong();
        return keep(new view(buffer, count(c, buffer.byteLength), count(d, buffer.byteLength)));
      }
      case "m": {
        const map = keep(new Map());
        for (const entry of list(a)) {
          if (list(entry).length !== 2) throw wrong();
          const key = build(entry[0]);
          map.set(key, build(entry[1]));
        }
        return map;
      }
      case "s": {
        const set = keep(new Set());
        for (const item of list(a)) set.add(build(item));
        return set;
      }
      case "e": {
        if (a === "DOMException") return keep(new DOMException(String(b), String(c)));
        const error = keep(new (Object.hasOwn(errorKinds, a) ? errorKinds[a] : Error)(String(b)));
        if (typeof c === "string") define(error, { stack: c });
        if (n.length > 4) define(error, { cause: build(d) });
        return error;
      }
      case "a": return fill(keep(new Array(count(a, 4294967295))), b);
      case "o": return fill(keep({}), a);
    }
    throw wrong();
  };
  try {
    return { data: build(JSON.parse(text)), ports };
  } catch (failure) {
    for (const port of ports) port.close();
    throw failure;
  }
};

// incoming is a message light tells the page of, read: null, with what
// came with it closed, for one that cannot be.
const incoming = (text, ends, size) => {
  try {
    return read(text, taken(size), ends);
  } catch (_) {
    return null;
  }
};

// A window of another frame is a facade made here, of this page's own
// objects: what it answers is asked of light by the names of one table
// (frames.WindowMembers), and whatever is not in it is refused. Nothing
// of the other page is ever held: light runs each in an engine of its
// own.
const crossTable = host("window", "table");
const crossed = () => new DOMException(`Blocked a frame with origin "${host("origin")}" from accessing a cross-origin frame.`, "SecurityError");
const quietSymbols = new Set([Symbol.toStringTag, Symbol.hasInstance, Symbol.isConcatSpreadable]);
const quiet = (name) => (typeof name === "symbol" ? quietSymbols.has(name) : crossTable.quiet.includes(name));
const indexOf = (name) => (typeof name === "string" && /^(0|[1-9]\d{0,8})$/.test(name) ? Number(name) : -1);

// shield is the object a script holds for what is another page's: it
// answers names by read and takes those write takes, more answering
// what is no name of the table, undefined for what it does not know.
const shield = (names, read, write, more, keys) => {
  const known = (name) => typeof name === "string" && Object.hasOwn(names, name);
  const answer = (name) => {
    if (known(name)) return names[name] === "setter" ? undefined : { value: read(name) };
    const value = typeof name === "string" ? more(name) : undefined;
    return value === undefined ? undefined : { value };
  };
  const found = (name) => {
    if (known(name) && names[name] === "setter") throw crossed();
    const held = answer(name);
    if (!held) throw crossed();
    return held.value;
  };
  return new Proxy(Object.create(null), {
    get(_, name) { return quiet(name) ? undefined : found(name); },
    set(_, name, value) {
      if (typeof name !== "string" || !write(name, value)) throw crossed();
      return true;
    },
    has(_, name) {
      if (quiet(name)) return false;
      if (!known(name) && !answer(name)) throw crossed();
      return true;
    },
    getOwnPropertyDescriptor(_, name) {
      if (quiet(name)) return undefined;
      if (known(name) && names[name] === "setter") {
        return { get: undefined, set: (value) => { write(name, value); }, enumerable: false, configurable: true };
      }
      return { value: found(name), writable: false, enumerable: false, configurable: true };
    },
    ownKeys() { return [...keys(), ...Object.keys(names)]; },
    defineProperty() { throw crossed(); },
    deleteProperty() { throw crossed(); },
    getPrototypeOf() { return null; },
    setPrototypeOf(_, prototype) { return prototype === null; },
    isExtensible() { return true; },
    preventExtensions() { return false; },
  });
};

const facades = new Set();
const facadeOf = new Map();
// windowOf is the window light names by a number: this page's own for
// 0, null for none.
const windowOf = (id) => {
  if (id === 0) return window;
  if (!Number.isInteger(id) || id < 0) return null;
  if (!facadeOf.has(id)) facadeOf.set(id, facade(id));
  return facadeOf.get(id);
};

// targeted reads the second and third arguments of a postMessage: the
// origin the message is for and what goes with it.
const targeted = (target, transfer) => {
  let origin = "/";
  if (target !== null && typeof target === "object") {
    if (target.targetOrigin !== undefined) origin = String(target.targetOrigin);
    transfer = target.transfer;
  } else if (target !== undefined && target !== null) origin = String(target);
  return [origin, transfer === undefined || transfer === null ? [] : Array.from(transfer)];
};

const facade = (id) => {
  const absolute = (address) => {
    const to = host("url", String(address), here.href);
    if (!to) throw new DOMException(`'${address}' is not a valid URL.`, "SyntaxError");
    return to.href;
  };
  const place = shield(crossTable.location, (name) => placed[name], (name, value) => {
    if (name !== "href") return false;
    host("window", "go", id, absolute(value), false);
    return true;
  }, () => undefined, () => []);
  const placed = {
    replace: function replace(address) { host("window", "go", id, absolute(address), true); },
  };
  const methods = {
    postMessage: function postMessage(message, target, transfer) {
      if (arguments.length === 0) throw new TypeError("postMessage needs a message");
      const [origin, list] = targeted(target, transfer);
      host("window", "aims", origin);
      host("window", "post", id, origin, ...given(written(message, list)));
    },
    // A frame is no window a script opened, so close does nothing; and
    // blur gives the keyboard to no one else.
    close: function close() {},
    focus: function focus() { host("window", "keyboard", id); },
    blur: function blur() {},
  };
  const shapes = {
    self: () => made,
    window: (name) => windowOf(host("window", name, id)),
    null: () => null,
    number: (name) => host("window", name, id),
    boolean: (name) => host("window", name, id),
    location: () => place,
    method: (name) => methods[name],
  };
  const made = shield(crossTable.window, (name) => shapes[crossTable.window[name]](name), (name, value) => {
    if (name !== "location") return false;
    host("window", "go", id, absolute(value), false);
    return true;
  }, (name) => {
    const index = indexOf(name);
    const child = index >= 0 ? host("window", "child", id, index) : host("window", "named", id, name);
    return child < 0 ? undefined : windowOf(child);
  }, () => Array.from({ length: host("window", "length", id) }, (_, i) => String(i)));
  facades.add(made);
  facades.add(place);
  for (const [table, held] of [[crossTable.window, methods], [crossTable.location, placed]]) {
    const listed = Object.keys(table).filter((name) => table[name] === "method").sort().join();
    if (listed !== Object.keys(held).sort().join()) throw new Error("light: the windows of other frames and their table differ");
  }
  for (const name of Object.keys(crossTable.window)) {
    if (!Object.hasOwn(shapes, crossTable.window[name])) throw new Error(`light: no window answers a ${crossTable.window[name]}`);
  }
  if (marks.join() !== crossTable.carries.join()) throw new Error("light: what a message carries and its table differ");
  return made;
};
// The table is read once, as the page begins: a name with nothing to
// answer it stops the page there, not at its first frame.
facade(-1);
facades.clear();
