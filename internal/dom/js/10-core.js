// The page's side of light's bindings. Every file of this folder is one
// part of a single function, run once per page before its scripts: what
// it declares is local to it, and only what it sets on globalThis shows.
// host is the one way into light: host(op, ...args), plain values only.
const light = globalThis.__light;
delete globalThis.__light;
const window = globalThis;
// treeReads are the questions to light that leave the tree as it is:
// any other makes the live lists of nodes read it again
// (15-collections.js).
const treeReads = new Set(["doc", "top", "tag", "parent", "children", "child", "sibling", "connected", "contains",
  "compare", "text", "html", "query", "matches", "closest", "byId", "attr", "attrs", "value", "selected", "selectedText",
  "checked", "selection", "active", "validity", "box", "laid", "at", "viewport", "now", "where", "log", "url", "urlWith", "document", "attribute", "list", "own",
  "owner", "xml", "tagged", "markup"]);
// What light refuses under a DOM name ("SyntaxError: …") is thrown as
// the DOMException of that name.
const host = (...args) => {
  const op = args[0];
  if ((op === "query" || op === "matches" || op === "closest") && validityAsked.test(args[2])) tellValidity(args[1]);
  if (!treeReads.has(op) || (op === "document" && !treeReads.has(args[1])) ||
    (op === "attribute" && !treeReads.has(args[2]))) bag.version++;
  try {
    return light(...args);
  } catch (error) {
    const named = /^(SyntaxError|QuotaExceededError|HierarchyRequestError|NotFoundError|SecurityError|DataCloneError|NotSupportedError|InvalidCharacterError): (.*)$/s.exec(error.message);
    if (named) throw new DOMException(named[2], named[1]);
    throw error;
  }
};

// errorCodes are the numbers the exceptions of the DOM had before they
// had names, which scripts still read.
const errorCodes = {
  INDEX_SIZE_ERR: 1, DOMSTRING_SIZE_ERR: 2, HIERARCHY_REQUEST_ERR: 3, WRONG_DOCUMENT_ERR: 4, INVALID_CHARACTER_ERR: 5,
  NO_DATA_ALLOWED_ERR: 6, NO_MODIFICATION_ALLOWED_ERR: 7, NOT_FOUND_ERR: 8, NOT_SUPPORTED_ERR: 9,
  INUSE_ATTRIBUTE_ERR: 10, INVALID_STATE_ERR: 11, SYNTAX_ERR: 12, INVALID_MODIFICATION_ERR: 13, NAMESPACE_ERR: 14,
  INVALID_ACCESS_ERR: 15, VALIDATION_ERR: 16, TYPE_MISMATCH_ERR: 17, SECURITY_ERR: 18, NETWORK_ERR: 19, ABORT_ERR: 20,
  URL_MISMATCH_ERR: 21, QUOTA_EXCEEDED_ERR: 22, TIMEOUT_ERR: 23, INVALID_NODE_TYPE_ERR: 24, DATA_CLONE_ERR: 25,
};
const codeOf = {
  IndexSizeError: 1, HierarchyRequestError: 3, WrongDocumentError: 4, InvalidCharacterError: 5,
  NoModificationAllowedError: 7, NotFoundError: 8, NotSupportedError: 9, InUseAttributeError: 10,
  InvalidStateError: 11, SyntaxError: 12, InvalidModificationError: 13, NamespaceError: 14, InvalidAccessError: 15,
  TypeMismatchError: 17, SecurityError: 18, NetworkError: 19, AbortError: 20, URLMismatchError: 21,
  QuotaExceededError: 22, TimeoutError: 23, InvalidNodeTypeError: 24, DataCloneError: 25,
};
class DOMException extends Error {
  constructor(message = "", name = "Error") {
    super(message);
    this.name = name;
  }
  get code() { return Object.hasOwn(codeOf, this.name) ? codeOf[this.name] : 0; }
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
for (const [name, value] of Object.entries(errorCodes)) {
  for (const target of [DOMException, DOMException.prototype]) {
    Object.defineProperty(target, name, { value, enumerable: true });
  }
}


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

// What an event is stays off it, in a map no script of a page reaches:
// a page sees it through the attributes of Event alone, and writes
// none of it, whether light made the event least of all. The map is
// one the windows of kin share (05-kin.js), so an event made in one is
// sent in another.
const eventStates = bag.events || (bag.events = new WeakMap());
const eventState = (event) => {
  const state = eventStates.get(event);
  if (!state) throw new TypeError("Illegal invocation");
  return state;
};
// dictionary is what a script gave where a dictionary is taken:
// nothing is an empty one, and a value that is no object is refused.
const dictionary = (given) => {
  if (given === undefined || given === null) return {};
  if (typeof given !== "object" && typeof given !== "function") throw new TypeError("A dictionary is an object.");
  return given;
};
const trustedOf = Object.getOwnPropertyDescriptor({ get isTrusted() { return eventState(this).trusted; } }, "isTrusted");
trustedOf.configurable = false;
// trusted marks an event as light's own, not a script's.
const trusted = (event) => {
  eventStates.get(event).trusted = true;
  return event;
};
// currentEvent is the event whose listener is running, window.event.
let currentEvent;

class Event {
  constructor(type, init) {
    if (arguments.length === 0) throw new TypeError("Event needs a type");
    init = dictionary(init);
    eventStates.set(this, {
      type: String(type), bubbles: Boolean(init.bubbles), cancelable: Boolean(init.cancelable),
      composed: Boolean(init.composed), target: null, currentTarget: null, phase: NONE, prevented: false,
      trusted: false, time: now(), stopped: false, halted: false, path: [], sent: false, passive: false, made: true,
    });
    Object.defineProperty(this, "isTrusted", trustedOf);
  }
  get type() { return eventState(this).type; }
  get target() { return eventState(this).target; }
  get srcElement() { return eventState(this).target; }
  get currentTarget() { return eventState(this).currentTarget; }
  get eventPhase() { return eventState(this).phase; }
  get bubbles() { return eventState(this).bubbles; }
  get cancelable() { return eventState(this).cancelable; }
  get composed() { return eventState(this).composed; }
  get defaultPrevented() { return eventState(this).prevented; }
  get timeStamp() { return eventState(this).time; }
  preventDefault() {
    const state = eventState(this);
    if (state.cancelable && !state.passive) state.prevented = true;
  }
  stopPropagation() { eventState(this).stopped = true; }
  stopImmediatePropagation() {
    const state = eventState(this);
    state.stopped = state.halted = true;
  }
  composedPath() { return eventState(this).path.slice(); }
  get returnValue() { return !eventState(this).prevented; }
  set returnValue(value) { if (!value) this.preventDefault(); }
  get cancelBubble() { return eventState(this).stopped; }
  set cancelBubble(value) { if (value) eventState(this).stopped = true; }
  initEvent(type, bubbles = false, cancelable = false) {
    const state = eventState(this);
    if (!arguments.length) throw new TypeError("An event is set up with its type.");
    if (state.sent) return;
    Object.assign(state, {
      made: true, stopped: false, halted: false, prevented: false, trusted: false, target: null,
      type: String(type), bubbles: Boolean(bubbles), cancelable: Boolean(cancelable),
    });
  }
}
define(Event, { NONE, CAPTURING_PHASE: CAPTURING, AT_TARGET, BUBBLING_PHASE: BUBBLING });
define(Event.prototype, { NONE, CAPTURING_PHASE: CAPTURING, AT_TARGET, BUBBLING_PHASE: BUBBLING });

class CustomEvent extends Event {
  constructor(type, init) {
    super(type, init);
    eventStates.get(this).detail = dictionary(init).detail === undefined ? null : init.detail;
  }
  get detail() { return eventState(this).detail; }
  initCustomEvent(type, bubbles = false, cancelable = false, detail = null) {
    if (!arguments.length) throw new TypeError("An event is set up with its type.");
    if (eventState(this).sent) return;
    this.initEvent(type, bubbles, cancelable);
    eventStates.get(this).detail = detail;
  }
}
// aView is the window an event is of: null, or a Window.
const aView = (view) => {
  if (view === null || view === undefined) return null;
  if (typeof view !== "object" || view.window !== view) throw new TypeError("The view of an event is a Window.");
  return view;
};
class UIEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.view = aView(init.view);
    this.detail = init.detail || 0;
  }
  initUIEvent(type, bubbles = false, cancelable = false, view = null, detail = 0) {
    if (!arguments.length) throw new TypeError("An event is set up with its type.");
    if (eventState(this).sent) return;
    this.initEvent(type, bubbles, cancelable);
    this.view = aView(view);
    this.detail = detail | 0;
  }
}
const modifiers = (event, init) => {
  for (const key of ["ctrlKey", "shiftKey", "altKey", "metaKey"]) event[key] = Boolean(init[key]);
  event.getModifierState = () => false;
};
class MouseEvent extends UIEvent {
  constructor(type, init) {
    super(type, init = dictionary(init));
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
  initMouseEvent(type, bubbles, cancelable, view, detail, screenX = 0, screenY = 0, clientX = 0, clientY = 0,
    ctrlKey = false, altKey = false, shiftKey = false, metaKey = false, button = 0, relatedTarget = null) {
    if (!arguments.length) throw new TypeError("An event is set up with its type.");
    if (eventState(this).sent) return;
    this.initUIEvent(type, bubbles, cancelable, view, detail);
    modifiers(this, { ctrlKey, altKey, shiftKey, metaKey });
    Object.assign(this, {
      screenX: Number(screenX) || 0, screenY: Number(screenY) || 0, clientX: Number(clientX) || 0,
      clientY: Number(clientY) || 0, button: Number(button) || 0, relatedTarget,
    });
  }
}
class PointerEvent extends MouseEvent {
  constructor(type, init) {
    super(type, init = dictionary(init));
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
  constructor(type, init) {
    super(type, init = dictionary(init));
    modifiers(this, init);
    this.key = init.key || "";
    this.code = init.code || "";
    this.repeat = Boolean(init.repeat);
    this.location = init.location >>> 0;
    this.isComposing = Boolean(init.isComposing);
    this.keyCode = this.which = init.keyCode || init.which || keyCodeOf(this.key);
    this.charCode = init.charCode >>> 0;
  }
  initKeyboardEvent(type, bubbles, cancelable, view, key = "", location = 0, ctrlKey = false, altKey = false,
    shiftKey = false, metaKey = false) {
    if (!arguments.length) throw new TypeError("An event is set up with its type.");
    if (eventState(this).sent) return;
    this.initUIEvent(type, bubbles, cancelable, view, 0);
    modifiers(this, { ctrlKey, altKey, shiftKey, metaKey });
    Object.assign(this, { key: String(key), location: location >>> 0 });
  }
}
class InputEvent extends UIEvent {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.data = init.data === undefined ? null : init.data;
    this.inputType = init.inputType || "";
    this.isComposing = false;
  }
}
class FocusEvent extends UIEvent {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.relatedTarget = init.relatedTarget || null;
  }
}
class SubmitEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.submitter = init.submitter || null;
  }
}
class PopStateEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.state = init.state === undefined ? null : init.state;
  }
}
class HashChangeEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.oldURL = init.oldURL || "";
    this.newURL = init.newURL || "";
  }
}
class ErrorEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.message = init.message || "";
    this.filename = init.filename || "";
    this.lineno = init.lineno || 0;
    this.colno = init.colno || 0;
    this.error = init.error === undefined ? null : init.error;
  }
}
class PromiseRejectionEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.promise = init.promise;
    this.reason = init.reason;
  }
}
class ProgressEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.lengthComputable = Boolean(init.lengthComputable);
    this.loaded = init.loaded || 0;
    this.total = init.total || 0;
  }
}
class StorageEvent extends Event {}
class MessageEvent extends Event {
  constructor(type, init) {
    super(type, init = dictionary(init));
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
class WheelEvent extends MouseEvent {
  constructor(type, init) {
    super(type, init = dictionary(init));
    for (const key of ["deltaX", "deltaY", "deltaZ"]) this[key] = Number(init[key]) || 0;
    this.deltaMode = init.deltaMode >>> 0;
  }
}
define(WheelEvent, { DOM_DELTA_PIXEL: 0, DOM_DELTA_LINE: 1, DOM_DELTA_PAGE: 2 });
class TouchEvent extends UIEvent {}
class DragEvent extends MouseEvent {}
class ClipboardEvent extends Event {}
class CompositionEvent extends UIEvent {
  constructor(type, init) {
    super(type, init = dictionary(init));
    this.data = init.data === undefined ? "" : String(init.data);
  }
}
class BeforeUnloadEvent extends Event {}
class PageTransitionEvent extends Event {}

// listenersOf is the listeners of a target by type, kept apart from the
// target so nothing of them shows on it.
const registry = bag.listeners;
const listenersOf = (target, create) => {
  let lists = registry.get(target);
  if (!lists && create) {
    lists = new Map();
    registry.set(target, lists);
  }
  return lists;
};
const capturing = (options) => (options !== null && (typeof options === "object" || typeof options === "function")
  ? Boolean(options.capture) : Boolean(options));
// A listener of scrolling by touch or by the wheel that does not say is
// passive on the window, the document, its element and its body.
const scrollTypes = new Set(["touchstart", "touchmove", "wheel", "mousewheel"]);
const passiveByDefault = (target, type) => scrollTypes.has(type) && (target === window || target === document ||
  target === document.documentElement || target === document.body);
// activation is what a click does besides its listeners (41-activation.js).
let activation = () => null;

// invoke runs the listeners of one target for one phase.
const invoke = (target, event, phase, capture = phase === CAPTURING) => {
  const state = eventStates.get(event);
  state.currentTarget = target;
  state.phase = phase;
  const lists = listenersOf(target);
  const list = lists ? (lists.get(event.type) || []).slice() : [];
  const inline = !capture && !handsToWindow(target, event.type) ? handlerOf(target, event.type) : null;
  if (inline) list.unshift({ callback: inline, capture: false, inline: true });
  for (const listener of list) {
    if (listener.removed) continue;
    if (Boolean(listener.capture) !== capture) continue;
    if (listener.once) target.removeEventListener(event.type, listener.callback, listener.capture);
    state.passive = Boolean(listener.passive);
    const callback = listener.callback;
    const outer = currentEvent;
    currentEvent = event;
    let result;
    if (listener.inline && target === window && event instanceof ErrorEvent) {
      result = guarded(callback, target, [event.message, event.filename, event.lineno, event.colno, event.error]) === true ? false : undefined;
    } else if (typeof callback === "function") result = guarded(callback, target, [event]);
    else result = guarded(handleEventOf, callback, [event]);
    currentEvent = outer;
    state.passive = false;
    if (listener.inline && result === false) event.preventDefault();
    if (state.halted) break;
  }
};

// handleEventOf calls the handleEvent of a listener that is an object,
// asked for each time an event reaches it.
function handleEventOf(event) {
  const handle = this.handleEvent;
  if (typeof handle !== "function") throw new TypeError("The listener's handleEvent is not a function.");
  return handle.call(this, event);
}

// handlerOf is the on… handler of a target for a type: the property a
// script set, else the attribute the page wrote, compiled when first
// needed.
const handlers = bag.handlers;
// The handlers of these events that a body or a frameset is given are
// the window's.
const windowHandled = new Set(["blur", "error", "focus", "load", "resize", "scroll", "afterprint", "beforeprint",
  "beforeunload", "hashchange", "languagechange", "message", "messageerror", "offline", "online", "pagehide",
  "pageshow", "popstate", "rejectionhandled", "storage", "unhandledrejection", "unload"]);
const handsToWindow = (target, type) => windowHandled.has(type) && target instanceof HTMLElement &&
  (target[TAG] === "body" || target[TAG] === "frameset");
const compiledHandler = (code) => {
  try {
    return new Function("event", `with (document) { with (this) {\n${code}\n} }`);
  } catch (error) {
    report(error);
    return null;
  }
};
const handlerOf = (target, type) => {
  if (handsToWindow(target, type)) target = window;
  const set = handlers.get(target);
  if (set && type in set) return set[type];
  const written = target === window && windowHandled.has(type) ? document.body : target;
  if (!(written instanceof Element)) return null;
  const code = written.getAttribute("on" + type);
  if (code === null) return null;
  // No script of a document a script made runs: it has no window.
  if (strangers && written.ownerDocument !== document) return null;
  const compiled = compiledHandler(code);
  setHandler(target, type, compiled);
  return compiled;
};
const setHandler = (target, type, fn) => {
  if (handsToWindow(target, type)) target = window;
  let set = handlers.get(target);
  if (!set) {
    set = {};
    handlers.set(target, set);
  }
  set[type] = typeof fn === "function" ? fn : null;
};
// forgetHandler drops what was compiled of an attribute that changed.
const forgetHandler = (target, type) => {
  if (handsToWindow(target, type)) {
    const code = target.getAttribute("on" + type);
    if (code !== null && !(strangers && target.ownerDocument !== document)) setHandler(window, type, compiledHandler(code));
    else if (handlers.has(window)) delete handlers.get(window)[type];
    return;
  }
  const set = handlers.get(target);
  if (set) delete set[type];
};
const handled = ["abort", "blur", "change", "click", "close", "contextmenu", "dblclick", "error", "focus",
  "focusin", "focusout", "input", "invalid", "keydown", "keypress", "keyup", "load", "mousedown", "mouseenter",
  "mouseleave", "mousemove", "mouseout", "mouseover", "mouseup", "pointerdown", "pointerup", "pointermove",
  "reset", "resize", "scroll", "select", "submit", "toggle", "touchstart", "touchend", "touchmove", "wheel",
  "transitionend", "animationend", "DOMContentLoaded", "readystatechange", "hashchange", "popstate", "message",
  "beforeunload", "unload", "pageshow", "pagehide", "storage", "online", "offline", "unhandledrejection",
  "visibilitychange", "loadstart", "progress", "loadend", "timeout", "animationstart", "animationiteration",
  "animationcancel", "transitionrun", "transitionstart", "transitioncancel", "webkitAnimationEnd",
  "webkitAnimationIteration", "webkitAnimationStart", "webkitTransitionEnd"];
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
    const target = called(this);
    const capture = capturing(options);
    const given = options !== null && (typeof options === "object" || typeof options === "function") ? options : {};
    const once = Boolean(given.once);
    const said = given.passive;
    const passive = said === undefined ? passiveByDefault(target, String(type)) : Boolean(said);
    const signal = signalIn(given);
    if (callback === null || callback === undefined) return;
    if (typeof callback !== "object" && typeof callback !== "function") {
      throw new TypeError("A listener is a function or an object with a handleEvent.");
    }
    if (signal && signal.aborted) return;
    const lists = listenersOf(target, true);
    type = String(type);
    const list = lists.get(type) || [];
    if (list.some((l) => l.callback === callback && l.capture === capture)) return;
    const listener = { callback, capture, once, passive };
    lists.set(type, list.concat(listener));
    if (signal) onAbort(signal, () => target.removeEventListener(type, callback, capture));
  }
  removeEventListener(type, callback, options) {
    const lists = listenersOf(called(this));
    const capture = capturing(options);
    type = String(type);
    if (!lists || !lists.has(type)) return;
    lists.set(type, lists.get(type).filter((l) => {
      const same = l.callback === callback && l.capture === capture;
      if (same) l.removed = true;
      return !same;
    }));
  }
  dispatchEvent(event) {
    const state = event === null || typeof event !== "object" ? undefined : eventStates.get(event);
    if (!state) throw new TypeError("dispatchEvent needs an Event");
    if (state.sent || !state.made) throw new DOMException("The event is being sent, or was not set up.", "InvalidStateError");
    state.trusted = false;
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
    else if (bag.mixed && event.type !== "load") {
      const keeps = keptBy(path[path.length - 1]);
      const view = keeps && keeps.viewOf(path[path.length - 1]);
      if (view) path.push(view);
    }
  }
  return path;
};

// dispatch sends event to target as a browser does: down to it, at it,
// then back up when it bubbles. It reports whether its default stands.
const dispatch = (target, event) => {
  const state = eventStates.get(event);
  state.target = target;
  state.sent = true;
  const path = state.path = pathOf(target, event);
  const acts = activation(event, path);
  for (let i = path.length - 1; i > 0 && !state.stopped; i--) invoke(path[i], event, CAPTURING);
  if (!state.stopped) invoke(target, event, AT_TARGET, true);
  if (!state.stopped) invoke(target, event, AT_TARGET, false);
  if (state.bubbles) {
    for (let i = 1; i < path.length && !state.stopped; i++) invoke(path[i], event, BUBBLING);
  }
  Object.assign(state, { currentTarget: null, phase: NONE, sent: false, stopped: false, halted: false, path: [] });
  if (acts) acts(!state.prevented);
  return !state.prevented;
};

