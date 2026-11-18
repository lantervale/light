// ---- the console

// own is a stack without the calls of light's own script.
const own = (stack) => String(stack).split("\n")
  .filter((line) => !/^\s+at .*(light:dom|<anonymous>)/.test(line)).join("\n");
// shown is a value as the console prints it.
const shown = (value, depth = 0, seen = new Set()) => {
  if (typeof value === "string") return depth ? JSON.stringify(value) : value;
  if (Object.is(value, -0)) return "-0";
  if (value === null || value === undefined || typeof value === "number" || typeof value === "boolean") return String(value);
  if (typeof value === "bigint") return `${value}n`;
  if (typeof value === "symbol") return value.toString();
  if (typeof value === "function") {
    if (/^class\b/.test(Function.prototype.toString.call(value))) return `class ${value.name || "anonymous"}`;
    return `ƒ ${value.name || "anonymous"}()`;
  }
  if (value instanceof Error) return value.stack ? own(value.stack) : `${value.name}: ${value.message}`;
  if (value instanceof Element) {
    const classes = value.className ? "." + value.className.trim().split(/\s+/).join(".") : "";
    return `<${value.localName}${value.id ? "#" + value.id : ""}${classes}>`;
  }
  if (value instanceof Text) return JSON.stringify(value.data);
  if (value === document) return "#document";
  if (value === window) return "Window";
  if (seen.has(value) || depth > 3) return Array.isArray(value) ? "[…]" : "{…}";
  seen.add(value);
  try {
    if (Array.isArray(value)) {
      const items = value.slice(0, 100).map((item) => shown(item, depth + 1, seen));
      if (value.length > 100) items.push(`… ${value.length - 100} more`);
      return `[${items.join(", ")}]`;
    }
    if (ArrayBuffer.isView(value) && !(value instanceof DataView)) {
      return `${value.constructor.name}(${value.length}) ${shown(Array.from(value), depth, seen)}`;
    }
    if (value instanceof Date) return value.toISOString();
    if (value instanceof RegExp) return String(value);
    if (value instanceof Map) return `Map(${value.size}) ${shown(Object.fromEntries(Array.from(value).slice(0, 50)), depth, seen)}`;
    if (value instanceof Set) return `Set(${value.size}) ${shown(Array.from(value), depth, seen)}`;
    const keys = Object.keys(value);
    const pairs = keys.slice(0, 50).map((key) => `${key}: ${shown(value[key], depth + 1, seen)}`);
    if (keys.length > 50) pairs.push("…");
    const name = value.constructor && value.constructor.name && value.constructor.name !== "Object" ? value.constructor.name + " " : "";
    return `${name}{${pairs.join(", ")}}`;
  } catch (_) {
    return Object.prototype.toString.call(value);
  } finally {
    seen.delete(value);
  }
};
// said is the arguments of a console call as one line, its first
// string's %s, %d, %i, %f, %o, %O and %c filled in.
const said = (args) => {
  args = Array.from(args);
  let text = "";
  if (typeof args[0] === "string" && /%[sdifoOc%]/.test(args[0])) {
    text = args.shift().replace(/%([sdifoOc%])/g, (whole, kind) => {
      if (kind === "%") return "%";
      if (!args.length) return whole;
      const value = args.shift();
      if (kind === "c") return "";
      if (kind === "d" || kind === "i") return String(parseInt(value, 10));
      if (kind === "f") return String(parseFloat(value));
      return shown(value);
    });
    if (args.length) text += " ";
  }
  return text + args.map((value) => shown(value)).join(" ");
};
// quoted is a value as the console's line gives it: a string in its
// quotes.
const quoted = (value) => typeof value === "string" ? JSON.stringify(value) : shown(value);
// tabled is data as console.table lists it, in plain text: a line for
// each of its entries, a column for each of their keys.
const tabled = (data, only) => {
  const entries = Object.entries(data);
  const whole = (value) => value === null || typeof value !== "object";
  const columns = [];
  for (const [, entry] of entries) {
    for (const key of whole(entry) ? ["Value"] : Object.keys(entry)) {
      if (!columns.includes(key) && (!only || key === "Value" || only.includes(key))) columns.push(key);
    }
  }
  const cell = (entry, key) => {
    if (whole(entry)) return key === "Value" ? shown(entry) : "";
    return key in entry ? shown(entry[key]) : "";
  };
  const rows = [["(index)", ...columns], ...entries.map(([index, entry]) => [index, ...columns.map((key) => cell(entry, key))])];
  const widths = rows[0].map((_, at) => Math.max(...rows.map((row) => row[at].length)));
  return rows.map((row) => row.map((text, at) => text.padEnd(widths[at])).join("  ").trimEnd()).join("\n");
};
const counts = new Map(), started = new Map();
// groups is how many groups of the console are open.
let groups = 0;
// tell says one line of the console, from where the page called it,
// with the calls that led there when traced.
const tell = (level, text, traced = false) => { host("log", level, text, new Error().stack || "", groups, traced); };
const print = (level) => function log() { tell(level, said(arguments)); };
const group = function group() {
  tell("log", said(arguments) || "console.group");
  groups++;
};
const console = {
  log: print("log"), info: print("info"), warn: print("warn"), error: print("error"), debug: print("debug"),
  dir(value) { tell("log", shown(value)); }, dirxml: print("log"),
  trace() { tell("log", said(arguments) || "console.trace", true); },
  table(data, columns) { tell("log", data !== null && typeof data === "object" ? tabled(data, columns) : said([data])); },
  group, groupCollapsed: group, groupEnd() { groups = Math.max(0, groups - 1); },
  clear() { groups = 0; host("clear"); },
  assert(ok, ...args) { if (!ok) tell("error", "Assertion failed" + (args.length ? ": " + said(args) : "")); },
  count(label = "default") {
    counts.set(label, (counts.get(label) || 0) + 1);
    tell("log", `${label}: ${counts.get(label)}`);
  },
  countReset(label = "default") { counts.delete(label); },
  time(label = "default") { started.set(label, now()); },
  timeLog(label = "default", ...args) {
    if (!started.has(label)) return tell("warn", `Timer '${label}' does not exist`);
    tell("log", [`${label}: ${(now() - started.get(label)).toFixed(2)} ms`, ...args.map((value) => shown(value))].join(" "));
  },
  timeEnd(label = "default") { console.timeLog(label); started.delete(label); },
  profile() {}, profileEnd() {}, timeStamp() {},
};

// ---- timers and frames

let lastTimer = 0;
const timers = new Map();
let frames = new Map();
// running is the timer whose callback is running: one that never
// returned is dropped, so it does not stop the page again.
let running = 0;
const schedule = (repeat) => (callback, delay, ...args) => {
  if (typeof callback !== "function") {
    const code = String(callback);
    callback = () => (0, eval)(code);
  }
  delay = Math.max(Number(delay) || 0, repeat ? 4 : 0);
  timers.set(++lastTimer, { callback, args, at: now() + delay, every: repeat ? delay : -1, order: lastTimer });
  return lastTimer;
};
const setTimeout = schedule(false), setInterval = schedule(true);
const clearTimeout = (id) => { timers.delete(Number(id)); };
const requestAnimationFrame = (callback) => {
  frames.set(++lastTimer, callback);
  return lastTimer;
};
const cancelAnimationFrame = (id) => { frames.delete(Number(id)); };
const queueMicrotask = (callback) => {
  if (typeof callback !== "function") throw new TypeError("queueMicrotask needs a function");
  Promise.resolve().then(() => guarded(callback, undefined, []));
};
const requestIdleCallback = (callback) => setTimeout(() => callback({ didTimeout: false, timeRemaining: () => 10 }), 1);

// tick runs what is due at time: the timers, oldest first, then the
// frame callbacks asked for before it. It stops once it ran for budget
// milliseconds, the rest left for the next, and says how many timers
// and frames wait.
const tick = (time, budget) => {
  if (running) {
    timers.delete(running);
    running = 0;
  }
  const until = Date.now() + budget;
  const due = Array.from(timers).filter(([, t]) => t.at <= time).sort((a, b) => a[1].at - b[1].at || a[1].order - b[1].order);
  for (const [id, timer] of due) {
    if (!timers.has(id)) continue;
    if (timer.every < 0) timers.delete(id);
    else timer.at = time + timer.every;
    running = id;
    guarded(timer.callback, window, timer.args);
    running = 0;
    if (Date.now() > until) break;
  }
  if (Date.now() <= until) {
    const callbacks = frames;
    frames = new Map();
    for (const callback of callbacks.values()) guarded(callback, window, [time]);
  }
  return timers.size + frames.size;
};

// ---- addresses

const parseURL = (reference, base) => {
  const parsed = host("url", String(reference), base === undefined ? "" : String(base));
  if (!parsed) throw new TypeError(`Failed to construct 'URL': Invalid URL '${reference}'`);
  return parsed;
};
const PARTS = Symbol("parts");
class URLSearchParams {
  constructor(init = "", owner = null) {
    this._list = [];
    this._owner = owner;
    if (typeof init === "string") {
      for (const pair of init.replace(/^\?/, "").split("&")) {
        if (!pair) continue;
        const cut = pair.indexOf("=");
        const decode = (text) => {
          try { return decodeURIComponent(text.replace(/\+/g, " ")); } catch (_) { return text; }
        };
        this._list.push([decode(cut < 0 ? pair : pair.slice(0, cut)), decode(cut < 0 ? "" : pair.slice(cut + 1))]);
      }
    } else if (init && typeof init[Symbol.iterator] === "function") {
      for (const [name, value] of init) this._list.push([String(name), String(value)]);
    } else if (init && typeof init === "object") {
      for (const name of Object.keys(init)) this._list.push([name, String(init[name])]);
    }
  }
  _changed() { if (this._owner) this._owner._set("search", this.toString()); }
  get size() { return this._list.length; }
  append(name, value) { this._list.push([String(name), String(value)]); this._changed(); }
  delete(name) { this._list = this._list.filter(([n]) => n !== String(name)); this._changed(); }
  get(name) { const found = this._list.find(([n]) => n === String(name)); return found ? found[1] : null; }
  getAll(name) { return this._list.filter(([n]) => n === String(name)).map(([, v]) => v); }
  has(name) { return this._list.some(([n]) => n === String(name)); }
  set(name, value) {
    const at = this._list.findIndex(([n]) => n === String(name));
    if (at < 0) this._list.push([String(name), String(value)]);
    else {
      this._list[at][1] = String(value);
      this._list = this._list.filter(([n], i) => i <= at || n !== String(name));
    }
    this._changed();
  }
  sort() { this._list.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0)); this._changed(); }
  forEach(fn, self) { for (const [name, value] of this._list) fn.call(self, value, name, this); }
  keys() { return this._list.map(([n]) => n)[Symbol.iterator](); }
  values() { return this._list.map(([, v]) => v)[Symbol.iterator](); }
  entries() { return this._list.map((pair) => pair.slice())[Symbol.iterator](); }
  [Symbol.iterator]() { return this.entries(); }
  toString() {
    const encode = (text) => encodeURIComponent(text).replace(/%20/g, "+").replace(/[!'()~]/g, (c) => "%" + c.charCodeAt(0).toString(16).toUpperCase());
    return this._list.map(([name, value]) => `${encode(name)}=${encode(value)}`).join("&");
  }
}
class URL {
  constructor(reference, base) {
    this[PARTS] = parseURL(reference, base);
  }
  static canParse(reference, base) { return host("url", String(reference), base === undefined ? "" : String(base)) !== null; }
  static parse(reference, base) { return URL.canParse(reference, base) ? new URL(reference, base) : null; }
  static createObjectURL() { return "blob:light/" + Math.random().toString(36).slice(2); }
  static revokeObjectURL() {}
  _set(part, value) {
    const changed = host("urlWith", this[PARTS].href, part, String(value));
    if (changed) this[PARTS] = changed;
  }
  get searchParams() {
    if (!this._params) define(this, { _params: new URLSearchParams(this[PARTS].search, this) });
    return this._params;
  }
  toString() { return this[PARTS].href; }
  toJSON() { return this[PARTS].href; }
}
for (const part of ["href", "protocol", "username", "password", "host", "hostname", "port", "pathname", "search", "hash"]) {
  Object.defineProperty(URL.prototype, part, {
    get() { return this[PARTS][part]; },
    set(value) {
      if (part === "href") this[PARTS] = parseURL(value);
      else this._set(part, value);
      if (part === "search" || part === "href") define(this, { _params: null });
    },
    configurable: true, enumerable: true,
  });
}
Object.defineProperty(URL.prototype, "origin", { get() { return this[PARTS].origin; }, configurable: true, enumerable: true });

// location is where the page is: setting it, or a part of it, goes
// there, a place in the page itself (#…) without leaving it.
let here = parseURL(host("address"));
const go = (address, replace) => {
  const to = host("url", String(address), here.href);
  if (!to) throw new DOMException(`'${address}' is not a valid URL.`, "SyntaxError");
  if (to.href.split("#")[0] === here.href.split("#")[0] && (to.hash || String(address).includes("#"))) {
    if (to.href !== here.href) moved(to, null, replace ? "replace" : "push", true);
    return;
  }
  host("navigate", to.href, Boolean(replace));
};
// moved is the page at another address without having left: a place in
// it, or what a script pushed.
const moved = (to, state, how, hashed) => {
  const old = here.href;
  here = to;
  historyState = state;
  if (how) host("history", how, to.href);
  if (hashed) dispatch(window, new HashChangeEvent("hashchange", { oldURL: old, newURL: to.href }));
};
const location = {
  assign(address) { go(address, false); },
  replace(address) { go(address, true); },
  reload() { host("reload"); },
  toString() { return here.href; },
  get ancestorOrigins() { return []; },
};
for (const part of ["href", "protocol", "host", "hostname", "port", "pathname", "search", "hash", "origin"]) {
  Object.defineProperty(location, part, {
    get() { return here[part]; },
    set(value) {
      if (part === "href") return go(value, false);
      if (part === "origin") return undefined;
      const changed = host("urlWith", here.href, part, String(value));
      return changed ? go(changed.href, false) : undefined;
    },
    configurable: true, enumerable: true,
  });
}

// history is the tab's, as far as it is this page's: what the page
// pushed, and going back and forth through it.
let historyState = null;
const states = new Map();
const history = {
  get length() { return host("history", "length", ""); },
  get state() { return historyState; },
  scrollRestoration: "auto",
  pushState(state, _, address) { history._put(state, address, "push"); },
  replaceState(state, _, address) { history._put(state, address, "replace"); },
  _put(state, address, how) {
    const to = address === undefined || address === null ? here : host("url", String(address), here.href);
    if (!to || to.origin !== here.origin) {
      throw new DOMException(`A history state object with URL '${address}' cannot be created in a document with origin '${here.origin}'.`, "SecurityError");
    }
    const kept = state === undefined ? null : structuredClone(state);
    states.set(to.href, kept);
    moved(to, kept, how, false);
  },
  back() { host("go", -1); },
  forward() { host("go", 1); },
  go(delta) {
    if (!delta) host("reload");
    else host("go", Number(delta));
  },
};
// arrived is the tab back at an address this page pushed: the state it
// kept for it pops.
const arrived = (address) => {
  const to = parseURL(address);
  const hashed = to.href.split("#")[0] === here.href.split("#")[0] && to.hash !== here.hash;
  const old = here.href;
  here = to;
  historyState = states.has(to.href) ? states.get(to.href) : null;
  dispatch(window, new PopStateEvent("popstate", { state: historyState }));
  if (hashed) dispatch(window, new HashChangeEvent("hashchange", { oldURL: old, newURL: to.href }));
};

// ---- what else a window has

const storage = (area) => {
  const methods = {
    getItem(key) { return host("storage", area, "get", String(key)); },
    setItem(key, value) { host("storage", area, "set", String(key), String(value)); },
    removeItem(key) { host("storage", area, "remove", String(key)); },
    clear() { host("storage", area, "clear", ""); },
    key(i) { const keys = host("storage", area, "keys", ""); return i < keys.length ? keys[i] : null; },
  };
  return new Proxy(methods, {
    get(target, name) {
      if (typeof name !== "string") return undefined;
      if (name === "length") return host("storage", area, "keys", "").length;
      if (name in target) return target[name];
      const value = target.getItem(name);
      return value === null ? undefined : value;
    },
    set(target, name, value) { target.setItem(name, value); return true; },
    has(target, name) { return name in target || target.getItem(name) !== null; },
    deleteProperty(target, name) { target.removeItem(name); return true; },
    ownKeys() { return host("storage", area, "keys", ""); },
    getOwnPropertyDescriptor(target, name) {
      const value = target.getItem(name);
      return value === null ? undefined : { value, enumerable: true, configurable: true, writable: true };
    },
  });
};

const performance = {
  timeOrigin: Date.now() - now(),
  now,
  mark(name) { return { name, entryType: "mark", startTime: now(), duration: 0 }; },
  measure(name) { return { name, entryType: "measure", startTime: 0, duration: now() }; },
  clearMarks() {}, clearMeasures() {}, clearResourceTimings() {},
  getEntries() { return []; }, getEntriesByName() { return []; }, getEntriesByType() { return []; },
  timing: { navigationStart: Date.now() - now() },
  navigation: { type: 0, redirectCount: 0 },
  toJSON() { return {}; },
};

const userAgent = host("agent");
const navigator = {
  userAgent, appName: "Netscape", appCodeName: "Mozilla", appVersion: userAgent, product: "Gecko", vendor: "",
  platform: host("platform"), language: "en-US", languages: ["en-US", "en"], onLine: true,
  get cookieEnabled() { return host("canCookie"); },
  doNotTrack: "1", hardwareConcurrency: 1, maxTouchPoints: 0, webdriver: false, plugins: [], mimeTypes: [],
  javaEnabled: () => false,
  sendBeacon: () => false,
  clipboard: {
    writeText: () => Promise.reject(new DOMException("light keeps the clipboard from scripts.", "NotAllowedError")),
    readText: () => Promise.reject(new DOMException("light keeps the clipboard from scripts.", "NotAllowedError")),
  },
  serviceWorker: undefined,
};

const CSS = {
  escape(text) {
    return String(text).replace(/[^a-zA-Z0-9_ -￿-]/g, (c) => "\\" + c).replace(/^(-?)(\d)/, (_, dash, digit) => `${dash}\\3${digit} `);
  },
  supports() { return false; },
};

// mediaQuery answers the widths, heights and orientation of a media
// query for the page's view; the rest is false.
const mediaQuery = (query) => {
  const [width, height] = host("viewport");
  const length = (text) => parseFloat(text) * (/em$/.test(text) ? 16 : 1);
  const one = (part) => {
    part = part.trim().toLowerCase();
    if (part === "" || part === "all" || part === "screen") return true;
    if (part === "print") return false;
    const feature = /^\(\s*([a-z-]+)\s*(?::\s*([^)]+))?\)$/.exec(part);
    if (!feature) return false;
    const [, name, value] = feature;
    // A feature named alone asks whether the view has it at all.
    if (value === undefined) return /^(width|height|orientation|prefers-color-scheme|hover|pointer|color)$/.test(name);
    switch (name) {
      case "min-width": return width >= length(value);
      case "max-width": return width <= length(value);
      case "min-height": return height >= length(value);
      case "max-height": return height <= length(value);
      case "orientation": return value.trim() === (width >= height ? "landscape" : "portrait");
      case "prefers-color-scheme": return value.trim() === "light";
      case "prefers-reduced-motion": return value.trim() === "no-preference";
      case "hover": return value.trim() === "hover";
      case "pointer": return value.trim() === "fine";
      default: return false;
    }
  };
  return String(query).split(",").some((alternative) => alternative.split(/\band\b/).every((part) => {
    part = part.trim();
    return part.startsWith("not ") ? !one(part.slice(4)) : one(part.replace(/^only\s+/, ""));
  }));
};
class MediaQueryList extends EventTarget {
  constructor(query) {
    super();
    this.media = String(query);
  }
  get matches() { return mediaQuery(this.media); }
  addListener(fn) { this.addEventListener("change", fn); }
  removeListener(fn) { this.removeEventListener("change", fn); }
}
withHandlers(MediaQueryList.prototype, ["change"]);

// computed is the style of an element as ui computed it: every
// property after the cascade, a width and a height as laid out. An
// element ui has not styled yet answers by what light knows of it: what
// its style attribute says, and whether and how wide it is laid out.
const computedStyle = (element) => {
  const own = declarations(element.getAttribute("style") || "");
  const value = (name) => {
    name = kebab(String(name));
    const computed = host("computed", element[REF], name);
    if (computed !== null) return computed;
    const [, , width, height] = host("box", element[REF]);
    const laid = host("laid", element[REF]);
    if (own.has(name)) return own.get(name).replace(/\s*!important$/, "");
    switch (name) {
      case "display": return laid ? (width || height ? "block" : "inline") : "none";
      case "visibility": return "visible";
      case "width": return `${width}px`;
      case "height": return `${height}px`;
      case "opacity": return "1";
      case "position": return "static";
      default: return "";
    }
  };
  return new Proxy({}, {
    get(_, name) {
      if (typeof name !== "string") return undefined;
      if (name === "getPropertyValue") return value;
      if (name === "getPropertyPriority") return () => "";
      if (name === "cssText") return "";
      if (name === "length" || name === "item") {
        const names = (host("computedStyle", element[REF]) || Array.from(own)).map(([property]) => property);
        return name === "length" ? names.length : (i) => names[i] || "";
      }
      return value(name === "cssFloat" ? "float" : name);
    },
    has() { return true; },
    ownKeys() { return (host("computedStyle", element[REF]) || Array.from(own)).map(([property]) => property); },
    getOwnPropertyDescriptor(_, name) { return { value: value(name), enumerable: true, configurable: true }; },
  });
};

const selection = {
  rangeCount: 0, isCollapsed: true, type: "None", anchorNode: null, focusNode: null, anchorOffset: 0, focusOffset: 0,
  toString() { return host("selectedText"); },
  removeAllRanges() {}, addRange() {}, collapse() {}, empty() {}, selectAllChildren() {},
  getRangeAt() { throw new DOMException("light gives scripts no range of the selection.", "IndexSizeError"); },
};

const utf8 = {
  encode(text) { return Uint8Array.from(host("utf8", String(text))); },
  decode(bytes) { return host("text8", Array.from(bytes)); },
};
class TextEncoder {
  get encoding() { return "utf-8"; }
  encode(text = "") { return utf8.encode(text); }
  encodeInto(text, into) {
    const bytes = utf8.encode(text).subarray(0, into.length);
    into.set(bytes);
    return { read: text.length, written: bytes.length };
  }
}
class TextDecoder {
  constructor(label = "utf-8") {
    if (!/^(utf-?8|unicode-1-1-utf-8)$/i.test(label)) throw new RangeError(`light decodes UTF-8 only, not '${label}'`);
  }
  get encoding() { return "utf-8"; }
  decode(input) {
    if (input === undefined) return "";
    const view = input instanceof ArrayBuffer ? new Uint8Array(input) : new Uint8Array(input.buffer, input.byteOffset, input.byteLength);
    return utf8.decode(view);
  }
}

const clone = (value, seen = new Map()) => {
  if (value === null || typeof value !== "object") {
    if (typeof value === "function" || typeof value === "symbol") throw new DOMException("The value could not be cloned.", "DataCloneError");
    return value;
  }
  if (seen.has(value)) return seen.get(value);
  if (value instanceof Date) return new Date(value.getTime());
  if (value instanceof RegExp) return new RegExp(value.source, value.flags);
  if (value instanceof Node) throw new DOMException("A node could not be cloned.", "DataCloneError");
  if (ArrayBuffer.isView(value)) return value.slice();
  if (value instanceof ArrayBuffer) return value.slice(0);
  let copy;
  if (Array.isArray(value)) {
    copy = [];
    seen.set(value, copy);
    for (const item of value) copy.push(clone(item, seen));
  } else if (value instanceof Map) {
    copy = new Map();
    seen.set(value, copy);
    for (const [k, v] of value) copy.set(clone(k, seen), clone(v, seen));
  } else if (value instanceof Set) {
    copy = new Set();
    seen.set(value, copy);
    for (const item of value) copy.add(clone(item, seen));
  } else {
    copy = {};
    seen.set(value, copy);
    for (const key of Object.keys(value)) copy[key] = clone(value[key], seen);
  }
  return copy;
};
const structuredClone = (value) => clone(value);

const crypto = {
  getRandomValues(array) {
    const bytes = host("random", array.byteLength);
    new Uint8Array(array.buffer, array.byteOffset, array.byteLength).set(bytes);
    return array;
  },
  randomUUID() {
    const b = host("random", 16);
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    const hex = b.map((n) => n.toString(16).padStart(2, "0")).join("");
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  },
  subtle: undefined,
};

// Observers of what light does not watch see every element once, as
// shown and at its size, then nothing more.
const once = (entry) => class {
  constructor(callback) { this._callback = callback; }
  observe(target) {
    setTimeout(() => {
      if (this._callback) guarded(this._callback, this, [[entry(target)], this]);
    }, 0);
  }
  unobserve() {}
  disconnect() { this._callback = null; }
  takeRecords() { return []; }
};
const IntersectionObserver = once((target) => {
  const rect = target.getBoundingClientRect();
  return { target, isIntersecting: true, intersectionRatio: 1, boundingClientRect: rect, intersectionRect: rect, rootBounds: null, time: now() };
});
const ResizeObserver = once((target) => {
  const rect = target.getBoundingClientRect();
  const size = [{ inlineSize: rect.width, blockSize: rect.height }];
  return { target, contentRect: rect, borderBoxSize: size, contentBoxSize: size };
});
// light measures nothing for a PerformanceObserver to be told.
class PerformanceObserver {
  observe() {}
  disconnect() {}
  takeRecords() { return []; }
}
PerformanceObserver.supportedEntryTypes = [];

// light has no shadow tree, and finds nothing by XPath: a page that
// asks gets no node.
class ShadowRoot extends DocumentFragment {}
class XPathEvaluator {
  createExpression() { return this; }
  createNSResolver() { return null; }
  evaluate() {
    return {
      resultType: 4, snapshotLength: 0, singleNodeValue: null, invalidIteratorState: false,
      iterateNext: () => null, snapshotItem: () => null,
    };
  }
}

const customElements = {
  _defined: new Map(),
  define(name, constructor) { customElements._defined.set(name, constructor); },
  get(name) { return customElements._defined.get(name); },
  whenDefined(name) { return customElements._defined.has(name) ? Promise.resolve(customElements._defined.get(name)) : new Promise(() => {}); },
  upgrade() {},
};

class DOMParser {
  parseFromString(markup) {
    const parsed = document.implementation.createHTMLDocument();
    parsed.body.innerHTML = String(markup);
    return parsed;
  }
}
class XMLSerializer {
  serializeToString(node) { return node instanceof Element ? node.outerHTML : node.textContent || ""; }
}
