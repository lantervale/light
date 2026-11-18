// ---- messages: structured clones, channels, their ports, broadcasts

const uncloneable = (what) => new DOMException(`${what} could not be cloned.`, "DataCloneError");
const errorKinds = { Error, EvalError, RangeError, ReferenceError, SyntaxError, TypeError, URIError };
const viewKinds = [Int8Array, Uint8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array, Uint32Array,
  Float32Array, Float64Array, BigInt64Array, BigUint64Array];
// platform are the window's own objects that are no EventTarget.
const platform = new Set([location, history, navigator, performance, crypto, console, customElements]);
const put = (target, key, value) => {
  Object.defineProperty(target, key, { value, writable: true, enumerable: true, configurable: true });
};

// copied is the structured clone of value. seen holds the copy of each
// object met so far, so one met twice, or within itself, is copied
// once, and what a transfer moves stands there before the copy begins.
const copied = (value, seen) => {
  if (typeof value === "function") throw uncloneable("A function");
  if (typeof value === "symbol") throw uncloneable("A symbol");
  if (value === null || typeof value !== "object") return value;
  if (seen.has(value)) return seen.get(value);
  const keep = (copy) => {
    seen.set(value, copy);
    return copy;
  };
  if (value instanceof Boolean || value instanceof Number || value instanceof String || value instanceof BigInt) {
    return keep(Object(value.valueOf()));
  }
  if (value instanceof Date) return keep(new Date(value.getTime()));
  if (value instanceof RegExp) return keep(new RegExp(value.source, value.flags));
  if (value instanceof ArrayBuffer) {
    if (value.detached) throw uncloneable("An ArrayBuffer that was transferred");
    return keep(value.slice(0));
  }
  if (ArrayBuffer.isView(value)) {
    if (!(value.buffer instanceof ArrayBuffer)) throw uncloneable("A view of shared memory");
    const buffer = copied(value.buffer, seen);
    if (value instanceof DataView) return keep(new DataView(buffer, value.byteOffset, value.byteLength));
    const kind = viewKinds.find((view) => value instanceof view);
    if (!kind) throw uncloneable("A view");
    return keep(new kind(buffer, value.byteOffset, value.length));
  }
  if (value instanceof Map) {
    const copy = keep(new Map());
    for (const [key, item] of Array.from(value)) copy.set(copied(key, seen), copied(item, seen));
    return copy;
  }
  if (value instanceof Set) {
    const copy = keep(new Set());
    for (const item of Array.from(value)) copy.add(copied(item, seen));
    return copy;
  }
  if (value instanceof DOMException) return keep(new DOMException(value.message, value.name));
  if (value instanceof Error) {
    const kind = Object.hasOwn(errorKinds, value.name) ? errorKinds[value.name] : Error;
    const copy = keep(new kind(String(value.message)));
    if (typeof value.stack === "string") define(copy, { stack: value.stack });
    if (Object.hasOwn(value, "cause")) define(copy, { cause: copied(value.cause, seen) });
    return copy;
  }
  if (value instanceof File) return keep(new File([value], value.name, { type: value.type, lastModified: value.lastModified }));
  if (value instanceof Blob) return keep(new Blob([value], { type: value.type }));
  if (value instanceof EventTarget || value instanceof Event || platform.has(value) || facades.has(value)) throw uncloneable("An object of the page");
  if (value instanceof WeakMap || value instanceof WeakSet || value instanceof WeakRef || value instanceof Promise
    || value instanceof SharedBuffer) {
    throw uncloneable(`A ${value.constructor && value.constructor.name || "value"}`);
  }
  const copy = keep(Array.isArray(value) ? new Array(value.length) : {});
  for (const key of Object.keys(value)) put(copy, key, copied(value[key], seen));
  return copy;
};

// transferOf is the list of what a call gives away: its second
// argument when a list, else that argument's transfer.
const transferOf = (options) => {
  if (options === undefined || options === null) return [];
  if (typeof options !== "object") throw new TypeError("The options of a message are an object or a list to transfer.");
  if (typeof options[Symbol.iterator] === "function") return Array.from(options);
  return options.transfer === undefined ? [] : Array.from(options.transfer);
};

// serialized is value copied for a message, and the ports that leave
// with it. What is transferred is copied whole before anything is taken
// from the sender, so a value that cannot be cloned takes nothing.
const serialized = (value, transfer) => {
  const seen = new Map();
  for (const item of transfer) {
    if (seen.has(item)) throw uncloneable("A value listed twice to transfer");
    if (item instanceof MessagePort) {
      if (portOf(item).detached) throw uncloneable("A port that was closed or transferred");
      seen.set(item, new MessagePort(token));
    } else if (item instanceof ArrayBuffer) {
      if (item.detached) throw uncloneable("An ArrayBuffer that was transferred");
      seen.set(item, item.slice(0));
    } else throw uncloneable("A value that cannot be transferred");
  }
  const data = copied(value, seen);
  const ports = [];
  for (const item of transfer) {
    if (item instanceof MessagePort) {
      handOver(seen.get(item), item);
      ports.push(seen.get(item));
    } else item.transfer();
  }
  return { data, ports };
};

const structuredClone = function structuredClone(value, options) {
  if (arguments.length === 0) throw new TypeError("structuredClone needs a value");
  const transfer = options === undefined || options === null || options.transfer === undefined ? [] : Array.from(options.transfer);
  return serialized(value, transfer).data;
};

// ports is what a port is, kept apart from it: the port it is entangled
// with, or the number of its end when the other is in another document
// (57-windows.js), the messages that wait in it, whether it was
// started, and whether it was closed or left with a message.
const ports = new WeakMap();
const portOf = (port) => {
  const state = ports.get(port);
  if (!state) throw new TypeError("Illegal invocation");
  return state;
};
const entangle = (one, other) => {
  portOf(one).other = other;
  portOf(other).other = one;
};
// handOver makes to the port from was: its messages and its other end go
// to it, and from is left detached.
const handOver = (to, from) => {
  const old = portOf(from), state = portOf(to);
  state.queue = old.queue;
  old.queue = [];
  if (old.other) entangle(to, old.other);
  if (old.remote) remotes.set(state.remote = old.remote, to);
  old.other = null;
  old.remote = 0;
  old.detached = true;
};
const messageOf = (init, type = "message") => {
  const event = new MessageEvent(type, init);
  event.isTrusted = true;
  return event;
};
// arrive is the task of one message of a started port: the oldest that
// waits in it, when the port is still there to hear it.
const arrive = (port) => {
  const state = portOf(port);
  if (state.started && !state.detached && state.queue.length) dispatch(port, messageOf(state.queue.shift()));
};

class MessagePort extends EventTarget {
  constructor(key) {
    if (key !== token) illegal();
    super();
    ports.set(this, { other: null, remote: 0, queue: [], started: false, detached: false });
  }
  postMessage(message, options) {
    const state = portOf(this);
    if (arguments.length === 0) throw new TypeError("postMessage needs a message");
    const transfer = transferOf(options);
    if (transfer.includes(this)) throw uncloneable("The port a message is posted through");
    if (state.remote) {
      host("channel", "send", state.remote, ...given(written(message, transfer)));
      return;
    }
    const target = state.other;
    const doomed = target !== null && transfer.includes(target);
    const sent = serialized(message, transfer);
    if (target === null || doomed) return;
    const to = portOf(target);
    to.queue.push(sent);
    if (to.started) setTimeout(arrive, 0, target);
  }
  start() {
    const state = portOf(this);
    if (state.started) return;
    state.started = true;
    for (let i = 0; i < state.queue.length; i++) setTimeout(arrive, 0, this);
  }
  close() {
    const state = portOf(this);
    state.detached = true;
    state.queue = [];
    if (state.remote) {
      remotes.delete(state.remote);
      host("channel", "shut", state.remote);
      state.remote = 0;
    }
    const other = state.other;
    if (other === null) return;
    state.other = portOf(other).other = null;
    setTimeout(dispatch, 0, other, new Event("close"));
  }
}
withHandlers(MessagePort.prototype, ["messageerror", "close"]);
accessors(MessagePort.prototype, {
  onmessage: {
    get() { return handlerOf(this, "message"); },
    set(fn) {
      setHandler(this, "message", fn);
      this.start();
    },
  },
});

const channels = new WeakMap();
class MessageChannel {
  constructor() {
    const pair = [new MessagePort(token), new MessagePort(token)];
    entangle(pair[0], pair[1]);
    channels.set(this, pair);
  }
  get port1() { return channels.get(this)[0]; }
  get port2() { return channels.get(this)[1]; }
}

// postMessage is the window's own, a page posting to itself: what it
// posts to another frame's goes through that window (57-windows.js).
// The origin is the one light holds the page to be of.
const postMessage = function postMessage(message, target, transfer) {
  if (arguments.length === 0) throw new TypeError("postMessage needs a message");
  const [origin, list] = targeted(target, transfer);
  const from = host("origin");
  let reached = true;
  if (origin !== "*" && origin !== "/") {
    const to = host("url", origin, "");
    if (!to) throw new DOMException(`Invalid target origin '${origin}' in a call to 'postMessage'.`, "SyntaxError");
    reached = to.origin === from && from !== "null";
  }
  const sent = serialized(message, list);
  if (!reached) return;
  setTimeout(() => dispatch(window, messageOf({ ...sent, origin: from, source: window })), 0);
};

// broadcasts are the open channels of each name, in the order they were
// made. A page's channels reach the other channels of the same page,
// and through light those of the other documents of its origin that
// keep their storage where it does.
const broadcasts = new Map();
const broadcastOf = (channel) => {
  const state = broadcastStates.get(channel);
  if (!state) throw new TypeError("Illegal invocation");
  return state;
};
const broadcastStates = new WeakMap();
class BroadcastChannel extends EventTarget {
  constructor(name) {
    if (arguments.length === 0) throw new TypeError("BroadcastChannel needs a name");
    super();
    name = String(name);
    broadcastStates.set(this, { name, closed: false });
    if (!broadcasts.has(name)) broadcasts.set(name, new Set());
    broadcasts.get(name).add(this);
  }
  get name() { return broadcastOf(this).name; }
  postMessage(message) {
    const state = broadcastOf(this);
    if (arguments.length === 0) throw new TypeError("postMessage needs a message");
    if (state.closed) throw new DOMException("The channel is closed.", "InvalidStateError");
    const { data } = serialized(message, []);
    const from = host("origin");
    host("broadcast", state.name, ...given(written(message, [])));
    for (const channel of broadcasts.get(state.name)) {
      if (channel === this) continue;
      setTimeout(() => {
        if (!broadcastOf(channel).closed) dispatch(channel, messageOf({ data: copied(data, new Map()), origin: from }));
      }, 0);
    }
  }
  close() {
    const state = broadcastOf(this);
    if (state.closed) return;
    state.closed = true;
    const open = broadcasts.get(state.name);
    open.delete(this);
    if (!open.size) broadcasts.delete(state.name);
  }
}
withHandlers(BroadcastChannel.prototype, ["message", "messageerror"]);
