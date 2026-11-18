// The page's side of light's bindings. Every file of this folder is one
// part of a single function, run once per page before its scripts: what
// it declares is local to it, and only what it sets on globalThis shows.
// host is the one way into light: host(op, ...args), plain values only.
const light = globalThis.__light;
delete globalThis.__light;
const window = globalThis;
// What light refuses under a DOM name ("SyntaxError: …") is thrown as
// the DOMException of that name.
const host = (...args) => {
  try {
    return light(...args);
  } catch (error) {
    const named = /^(SyntaxError|QuotaExceededError|HierarchyRequestError|NotFoundError): (.*)$/s.exec(error.message);
    if (named) throw new DOMException(named[2], named[1]);
    throw error;
  }
};

class DOMException extends Error {
  constructor(message = "", name = "Error") {
    super(message);
    this.name = name;
  }
}

const define = (target, properties) => {
  for (const name of Object.keys(properties)) {
    Object.defineProperty(target, name, {
      value: properties[name], writable: true, configurable: true, enumerable: false,
    });
  }
};
const accessors = (prototype, properties) => {
  for (const name of Object.keys(properties)) {
    const { get, set } = properties[name];
    Object.defineProperty(prototype, name, { get, set, configurable: true, enumerable: true });
  }
};
const illegal = () => { throw new TypeError("Illegal constructor"); };


// report tells of an exception nothing caught: the window's error
// listeners first, then light, unless one of them took it.
let reporting = false;
const report = (error) => {
  const message = error instanceof Error ? `${error.name}: ${error.message}` : String(error);
  const stack = error && error.stack ? String(error.stack) : "";
  try {
    if (!reporting) {
      reporting = true;
      const [filename, lineno, colno] = host("where", stack);
      const event = new ErrorEvent("error", { cancelable: true, message: "Uncaught " + message, filename, lineno, colno, error });
      if (!dispatch(window, event)) return;
    }
    host("uncaught", message, stack);
  } catch (_) { /* nothing more to tell */ } finally {
    reporting = false;
  }
};
// guarded calls fn, and reports what it throws in place of throwing it.
const guarded = (fn, self, args) => {
  try {
    return fn.apply(self, args);
  } catch (error) {
    report(error);
    return undefined;
  }
};

// ---- events

const NONE = 0, CAPTURING = 1, AT_TARGET = 2, BUBBLING = 3;
const now = () => host("now");

class Event {
  constructor(type, init = {}) {
    if (arguments.length === 0) throw new TypeError("Event needs a type");
    this.type = String(type);
    this.bubbles = Boolean(init.bubbles);
    this.cancelable = Boolean(init.cancelable);
    this.composed = Boolean(init.composed);
    this.target = null;
    this.currentTarget = null;
    this.eventPhase = NONE;
    this.defaultPrevented = false;
    this.isTrusted = false;
    this.timeStamp = now();
    this._stopped = false;
    this._halted = false;
    this._path = [];
  }
  preventDefault() { if (this.cancelable && !this._passive) this.defaultPrevented = true; }
  stopPropagation() { this._stopped = true; }
  stopImmediatePropagation() { this._stopped = true; this._halted = true; }
  composedPath() { return this._path.slice(); }
  get srcElement() { return this.target; }
  get returnValue() { return !this.defaultPrevented; }
  set returnValue(value) { if (value === false) this.preventDefault(); }
  get cancelBubble() { return this._stopped; }
  set cancelBubble(value) { if (value) this._stopped = true; }
  initEvent(type, bubbles, cancelable) {
    this.type = String(type);
    this.bubbles = Boolean(bubbles);
    this.cancelable = Boolean(cancelable);
  }
}
define(Event, { NONE, CAPTURING_PHASE: CAPTURING, AT_TARGET, BUBBLING_PHASE: BUBBLING });
define(Event.prototype, { NONE, CAPTURING_PHASE: CAPTURING, AT_TARGET, BUBBLING_PHASE: BUBBLING });

class CustomEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.detail = init.detail === undefined ? null : init.detail;
  }
  initCustomEvent(type, bubbles, cancelable, detail) {
    this.initEvent(type, bubbles, cancelable);
    this.detail = detail;
  }
}
class UIEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.view = init.view || window;
    this.detail = init.detail || 0;
  }
}
const modifiers = (event, init) => {
  for (const key of ["ctrlKey", "shiftKey", "altKey", "metaKey"]) event[key] = Boolean(init[key]);
  event.getModifierState = () => false;
};
class MouseEvent extends UIEvent {
  constructor(type, init = {}) {
    super(type, init);
    modifiers(this, init);
    for (const key of ["clientX", "clientY", "screenX", "screenY", "button", "buttons", "movementX", "movementY"]) {
      this[key] = Number(init[key]) || 0;
    }
    this.pageX = this.clientX + window.scrollX;
    this.pageY = this.clientY + window.scrollY;
    this.x = this.offsetX = this.clientX;
    this.y = this.offsetY = this.clientY;
    this.relatedTarget = init.relatedTarget || null;
  }
}
class PointerEvent extends MouseEvent {
  constructor(type, init = {}) {
    super(type, init);
    this.pointerId = init.pointerId || 1;
    this.pointerType = init.pointerType || "mouse";
    this.isPrimary = true;
    this.width = this.height = 1;
    this.pressure = 0;
  }
}
// keyCodeOf is the number older scripts know a key by.
const keyCodes = {
  Backspace: 8, Tab: 9, Enter: 13, Shift: 16, Control: 17, Alt: 18, Escape: 27, " ": 32, PageUp: 33, PageDown: 34,
  End: 35, Home: 36, ArrowLeft: 37, ArrowUp: 38, ArrowRight: 39, ArrowDown: 40, Delete: 46,
};
const keyCodeOf = (key) => {
  if (key in keyCodes) return keyCodes[key];
  return /^[a-z0-9]$/i.test(key) ? key.toUpperCase().charCodeAt(0) : 0;
};
class KeyboardEvent extends UIEvent {
  constructor(type, init = {}) {
    super(type, init);
    modifiers(this, init);
    this.key = init.key || "";
    this.code = init.code || "";
    this.repeat = Boolean(init.repeat);
    this.location = 0;
    this.isComposing = false;
    this.keyCode = this.which = init.keyCode || init.which || keyCodeOf(this.key);
    this.charCode = 0;
  }
}
class InputEvent extends UIEvent {
  constructor(type, init = {}) {
    super(type, init);
    this.data = init.data === undefined ? null : init.data;
    this.inputType = init.inputType || "";
    this.isComposing = false;
  }
}
class FocusEvent extends UIEvent {
  constructor(type, init = {}) {
    super(type, init);
    this.relatedTarget = init.relatedTarget || null;
  }
}
class SubmitEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.submitter = init.submitter || null;
  }
}
class PopStateEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.state = init.state === undefined ? null : init.state;
  }
}
class HashChangeEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.oldURL = init.oldURL || "";
    this.newURL = init.newURL || "";
  }
}
class ErrorEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.message = init.message || "";
    this.filename = init.filename || "";
    this.lineno = init.lineno || 0;
    this.colno = init.colno || 0;
    this.error = init.error === undefined ? null : init.error;
  }
}
class PromiseRejectionEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.promise = init.promise;
    this.reason = init.reason;
  }
}
class ProgressEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.lengthComputable = Boolean(init.lengthComputable);
    this.loaded = init.loaded || 0;
    this.total = init.total || 0;
  }
}
class StorageEvent extends Event {}
class MessageEvent extends Event {
  constructor(type, init = {}) {
    super(type, init);
    this.data = init.data === undefined ? null : init.data;
    this.origin = init.origin === undefined ? "" : String(init.origin);
    this.lastEventId = init.lastEventId === undefined ? "" : String(init.lastEventId);
    this.source = init.source || null;
    this.ports = Object.freeze(Array.from(init.ports || []));
  }
  initMessageEvent(type, bubbles, cancelable, data = null, origin = "", lastEventId = "", source = null, ports = []) {
    this.initEvent(type, bubbles, cancelable);
    this.data = data;
    this.origin = String(origin);
    this.lastEventId = String(lastEventId);
    this.source = source;
    this.ports = Object.freeze(Array.from(ports));
  }
}
class TransitionEvent extends Event {}
class AnimationEvent extends Event {}
class WheelEvent extends MouseEvent {}
class TouchEvent extends UIEvent {}
class DragEvent extends MouseEvent {}
class ClipboardEvent extends Event {}
class CompositionEvent extends UIEvent {}
class BeforeUnloadEvent extends Event {}
class PageTransitionEvent extends Event {}

// listenersOf is the listeners of a target by type, kept apart from the
// target so nothing of them shows on it.
const registry = new WeakMap();
const listenersOf = (target, create) => {
  let lists = registry.get(target);
  if (!lists && create) {
    lists = new Map();
    registry.set(target, lists);
  }
  return lists;
};
const capturing = (options) => (typeof options === "boolean" ? options : Boolean(options && options.capture));

// invoke runs the listeners of one target for one phase.
const invoke = (target, event, phase) => {
  event.currentTarget = target;
  event.eventPhase = phase;
  const lists = listenersOf(target);
  const list = lists ? (lists.get(event.type) || []).slice() : [];
  const inline = phase !== CAPTURING ? handlerOf(target, event.type) : null;
  if (inline) list.unshift({ callback: inline, capture: false, inline: true });
  for (const listener of list) {
    if (listener.removed) continue;
    if (phase === CAPTURING && !listener.capture) continue;
    if (phase === BUBBLING && listener.capture) continue;
    if (listener.once) target.removeEventListener(event.type, listener.callback, listener.capture);
    event._passive = Boolean(listener.passive);
    const callback = listener.callback;
    let result;
    if (listener.inline && target === window && event instanceof ErrorEvent) {
      result = guarded(callback, target, [event.message, event.filename, event.lineno, event.colno, event.error]) === true ? false : undefined;
    } else if (typeof callback === "function") result = guarded(callback, target, [event]);
    else if (callback && typeof callback.handleEvent === "function") result = guarded(callback.handleEvent, callback, [event]);
    event._passive = false;
    if (listener.inline && result === false) event.preventDefault();
    if (event._halted) break;
  }
};

// handlerOf is the on… handler of a target for a type: the property a
// script set, else the attribute the page wrote, compiled when first
// needed.
const handlers = new WeakMap();
const handlerOf = (target, type) => {
  const set = handlers.get(target);
  if (set && type in set) return set[type];
  if (!(target instanceof Element)) return null;
  const code = target.getAttribute("on" + type);
  if (code === null) return null;
  let compiled = null;
  try {
    compiled = new Function("event", `with (document) { with (this) {\n${code}\n} }`);
  } catch (error) {
    report(error);
  }
  setHandler(target, type, compiled);
  return compiled;
};
const setHandler = (target, type, fn) => {
  let set = handlers.get(target);
  if (!set) {
    set = {};
    handlers.set(target, set);
  }
  set[type] = typeof fn === "function" ? fn : null;
};
// forgetHandler drops what was compiled of an attribute that changed.
const forgetHandler = (target, type) => {
  const set = handlers.get(target);
  if (set) delete set[type];
};
const handled = ["abort", "blur", "change", "click", "close", "contextmenu", "dblclick", "error", "focus",
  "focusin", "focusout", "input", "invalid", "keydown", "keypress", "keyup", "load", "mousedown", "mouseenter",
  "mouseleave", "mousemove", "mouseout", "mouseover", "mouseup", "pointerdown", "pointerup", "pointermove",
  "reset", "resize", "scroll", "select", "submit", "toggle", "touchstart", "touchend", "touchmove", "wheel",
  "transitionend", "animationend", "DOMContentLoaded", "readystatechange", "hashchange", "popstate", "message",
  "beforeunload", "unload", "pageshow", "pagehide", "storage", "online", "offline", "unhandledrejection",
  "visibilitychange", "loadstart", "progress", "loadend", "timeout"];
const withHandlers = (prototype, types = handled) => {
  for (const type of types) {
    Object.defineProperty(prototype, "on" + type.toLowerCase(), {
      get() { return handlerOf(this, type); },
      set(fn) { setHandler(this, type, fn); },
      configurable: true, enumerable: true,
    });
  }
};

// called is the target of a method of EventTarget: the window for one
// called bare, as a page's addEventListener("load", …).
const called = (target) => (target === undefined || target === null ? window : target);

class EventTarget {
  addEventListener(type, callback, options) {
    if (callback === null || callback === undefined) return;
    const target = called(this);
    const lists = listenersOf(target, true);
    const capture = capturing(options);
    const list = lists.get(type) || [];
    if (list.some((l) => l.callback === callback && l.capture === capture)) return;
    const listener = {
      callback, capture,
      once: Boolean(options && options.once), passive: Boolean(options && options.passive),
    };
    lists.set(type, list.concat(listener));
    const signal = options && options.signal;
    if (signal && typeof signal.addEventListener === "function") {
      signal.addEventListener("abort", () => target.removeEventListener(type, callback, capture), { once: true });
    }
  }
  removeEventListener(type, callback, options) {
    const lists = listenersOf(called(this));
    if (!lists || !lists.has(type)) return;
    const capture = capturing(options);
    lists.set(type, lists.get(type).filter((l) => {
      const same = l.callback === callback && l.capture === capture;
      if (same) l.removed = true;
      return !same;
    }));
  }
  dispatchEvent(event) {
    if (!(event instanceof Event)) throw new TypeError("dispatchEvent needs an Event");
    return dispatch(called(this), event);
  }
}

// pathOf is where an event on target goes: the target, its ancestors,
// the document and the window. A load does not reach the window.
const pathOf = (target, event) => {
  const path = [target];
  if (target instanceof Node) {
    for (let n = target.parentNode; n; n = n.parentNode) path.push(n);
    if (path[path.length - 1] === document && event.type !== "load") path.push(window);
  }
  return path;
};

// dispatch sends event to target as a browser does: down to it, at it,
// then back up when it bubbles. It reports whether its default stands.
const dispatch = (target, event) => {
  event.target = target;
  event._stopped = event._halted = false;
  const path = event._path = pathOf(target, event);
  for (let i = path.length - 1; i > 0 && !event._stopped; i--) invoke(path[i], event, CAPTURING);
  if (!event._stopped) invoke(target, event, AT_TARGET);
  if (event.bubbles) {
    for (let i = 1; i < path.length && !event._stopped; i++) invoke(path[i], event, BUBBLING);
  }
  event.currentTarget = null;
  event.eventPhase = NONE;
  return !event.defaultPrevented;
};

class AbortSignal extends EventTarget {
  constructor() {
    super();
    this.aborted = false;
    this.reason = undefined;
  }
  throwIfAborted() { if (this.aborted) throw this.reason; }
  static abort(reason) {
    const signal = new AbortSignal();
    signal.aborted = true;
    signal.reason = reason === undefined ? new DOMException("The operation was aborted.", "AbortError") : reason;
    return signal;
  }
  static timeout(ms) {
    const signal = new AbortSignal();
    setTimeout(() => abort(signal, new DOMException("The operation timed out.", "TimeoutError")), ms);
    return signal;
  }
}
withHandlers(AbortSignal.prototype, ["abort"]);
const abort = (signal, reason) => {
  if (signal.aborted) return;
  signal.aborted = true;
  signal.reason = reason === undefined ? new DOMException("The operation was aborted.", "AbortError") : reason;
  dispatch(signal, new Event("abort"));
};
class AbortController {
  constructor() { this.signal = new AbortSignal(); }
  abort(reason) { abort(this.signal, reason); }
}
