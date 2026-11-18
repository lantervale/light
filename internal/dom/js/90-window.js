// ---- the window: what scripts find as globals

byName(Document.prototype, (page, name) => {
  const as = `[name="${CSS.escape(name)}"]`;
  return page.querySelector(["form", "img", "embed", "object", "iframe"].map((tag) => tag + as).join());
});
const document = new Document(token);

// framed is the window of the frame of the page at that index, or of
// that name, undefined for none.
const framed = (name) => {
  const index = indexOf(name);
  const child = index >= 0 ? host("window", "child", 0, index) : host("window", "named", 0, name);
  return child < 0 ? undefined : windowOf(child);
};
const [opaque, noModals] = host("sandbox");
// asks is a function that would ask the user something, which light
// only notes in the console: a sandboxed frame without allow-modals is
// refused even that.
const asks = (name, said, answer) => function (message) {
  if (noModals) host("log", "error", `Ignored call to '${name}()'. The document is sandboxed, and the 'allow-modals' keyword is not set.`);
  else host("log", "info", said(message));
  return answer;
};
const stored = (area) => {
  const kept = storage(area);
  return {
    get() {
      if (opaque) throw new DOMException(`Failed to read the '${area}Storage' property from 'Window': The document is sandboxed and lacks the 'allow-same-origin' flag.`, "SecurityError");
      return kept;
    },
  };
};

// A window names the frames of its page by their index and their name,
// then its elements by their id, where nothing else has the name.
class Window extends EventTarget {}
Object.setPrototypeOf(Window.prototype, new Proxy(Object.create(EventTarget.prototype), {
  has(target, name) { return name in target || (typeof name === "string" && (framed(name) !== undefined || host("byId", name) !== 0)); },
  get(target, name, receiver) {
    if (name in target || typeof name !== "string") return Reflect.get(target, name, receiver);
    const frame = framed(name);
    return frame !== undefined ? frame : wrap(host("byId", name)) || undefined;
  },
}));
Object.setPrototypeOf(window, Window.prototype);
withHandlers(window);

class Image extends HTMLImageElement {
  constructor(width, height) {
    const made = host("create", "img");
    super(token, made, "img");
    wrappers.set(made, this);
    if (width !== undefined) this.width = width;
    if (height !== undefined) this.height = height;
  }
}
class Option extends HTMLOptionElement {
  constructor(text = "", value, _, selected) {
    const made = host("create", "option");
    super(token, made, "option");
    wrappers.set(made, this);
    this.textContent = text;
    if (value !== undefined) this.value = value;
    if (selected) this.selected = true;
  }
}

define(window, {
  window, self: window, globalThis: window, frames: window, document, console,
  setTimeout, clearTimeout, setInterval, clearInterval: clearTimeout, requestAnimationFrame, cancelAnimationFrame,
  queueMicrotask, requestIdleCallback, cancelIdleCallback: clearTimeout, setImmediate: undefined,
  history, navigator, performance, crypto, customElements, CSS, structuredClone,
  // light is not told its monitor's size: the screen scripts read is
  // the window itself, the part of it light knows.
  screen: {
    get width() { return host("around")[0]; }, get height() { return host("around")[1]; },
    get availWidth() { return host("around")[0]; }, get availHeight() { return host("around")[1]; },
    colorDepth: 24, pixelDepth: 24,
    orientation: {
      get type() { const [width, height] = host("around"); return width >= height ? "landscape-primary" : "portrait-primary"; },
      angle: 0, addEventListener() {}, removeEventListener() {},
    },
  },
  visualViewport: undefined, name: "", closed: false, opener: null, frameElement: null,
  isSecureContext: here.protocol === "https:", origin: host("origin"), crossOriginIsolated: false, event: undefined, status: "",
  alert: asks("alert", (message) => "alert: " + (message === undefined ? "" : String(message)), undefined),
  confirm: asks("confirm", (message) => "confirm, answered no: " + String(message), false),
  prompt: asks("prompt", (message) => "prompt, left unanswered: " + String(message), null),
  print() {}, focus() {}, blur() {}, stop() {}, close() {},
  open() { host("log", "warn", "light opens no window for a script"); return null; },
  postMessage,
  matchMedia: (query) => new MediaQueryList(query),
  getComputedStyle: (element) => computedStyle(element),
  getSelection: () => selection,
  scrollTo(x, y) {
    if (x && typeof x === "object") [x, y] = [x.left === undefined ? window.scrollX : x.left, x.top === undefined ? window.scrollY : x.top];
    host("scroll", Number(x) || 0, Number(y) || 0);
  },
  scroll(...args) { window.scrollTo(...args); },
  scrollBy(x, y) {
    if (x && typeof x === "object") [x, y] = [x.left || 0, x.top || 0];
    window.scrollTo(window.scrollX + (Number(x) || 0), window.scrollY + (Number(y) || 0));
  },
  atob(text) {
    const decoded = host("atob", String(text));
    if (decoded === null) throw new DOMException("The string to be decoded is not correctly encoded.", "InvalidCharacterError");
    return decoded;
  },
  btoa(text) {
    text = String(text);
    if (/[^\0-\xff]/.test(text)) throw new DOMException("The string to be encoded contains characters outside of the Latin1 range.", "InvalidCharacterError");
    return host("btoa", text);
  },
  reportError: (error) => report(error),
  fetch, Headers, Request, Response, XMLHttpRequest, XMLHttpRequestUpload, XMLHttpRequestEventTarget, FormData, Blob,
  File, URL, URLSearchParams, TextEncoder, TextDecoder, AbortController, AbortSignal, DOMException, DOMParser,
  XMLSerializer, ShadowRoot, XPathEvaluator, MutationObserver, IntersectionObserver, ResizeObserver, PerformanceObserver, MediaQueryList,
  Window, EventTarget, Event, CustomEvent, UIEvent, MouseEvent, PointerEvent, KeyboardEvent, InputEvent, FocusEvent,
  SubmitEvent, PopStateEvent, HashChangeEvent, ErrorEvent, PromiseRejectionEvent, ProgressEvent, StorageEvent,
  MessageEvent, MessageChannel, MessagePort, BroadcastChannel, TransitionEvent, AnimationEvent, WheelEvent, TouchEvent, DragEvent, ClipboardEvent,
  CompositionEvent, BeforeUnloadEvent, PageTransitionEvent,
  Node, Element, HTMLElement, HTMLUnknownElement, SVGElement, Text, Comment, CharacterData, DocumentType, DocumentFragment, Document,
  HTMLDocument: Document, NodeList, HTMLCollection, HTMLFormControlsCollection, RadioNodeList, DOMTokenList, NamedNodeMap, Attr, DOMRect, DOMRectReadOnly,
  Range, NodeFilter, TreeWalker, Image, Option, Audio: undefined,
  HTMLAnchorElement, HTMLImageElement, HTMLInputElement, HTMLTextAreaElement, HTMLButtonElement, HTMLSelectElement,
  HTMLOptionElement, HTMLFormElement, HTMLLabelElement, HTMLTemplateElement, HTMLScriptElement, HTMLStyleElement,
  HTMLLinkElement, HTMLMetaElement, HTMLTitleElement, HTMLBaseElement, HTMLDetailsElement, HTMLDialogElement,
  HTMLCanvasElement, HTMLIFrameElement, HTMLMediaElement, HTMLVideoElement, HTMLAudioElement, HTMLTableElement,
  HTMLTableRowElement, HTMLTableCellElement, HTMLProgressElement, HTMLDivElement, HTMLSpanElement,
  HTMLParagraphElement, HTMLHeadingElement, HTMLUListElement, HTMLOListElement, HTMLLIElement, HTMLBodyElement,
  HTMLHtmlElement, HTMLHeadElement, HTMLPreElement, HTMLBRElement, HTMLHRElement, HTMLFieldSetElement,
  HTMLLegendElement, HTMLQuoteElement, HTMLTableSectionElement, HTMLMeterElement, HTMLDListElement, HTMLTimeElement,
  HTMLDataElement, HTMLPictureElement, HTMLSourceElement, HTMLObjectElement, HTMLEmbedElement, HTMLSlotElement,
  HTMLOptGroupElement, HTMLOutputElement, HTMLMenuElement, HTMLFontElement,
});
for (const name of ["setImmediate", "visualViewport", "event", "Audio"]) delete window[name];
delete window.SharedArrayBuffer;
accessors(window, {
  location: { get() { return location; }, set(address) { go(address, false); } },
  parent: { get() { return windowOf(host("window", "parent", 0)); }, set(value) { define(window, { parent: value }); } },
  top: { get() { return windowOf(host("window", "top", 0)); } },
  length: { get() { return host("window", "length", 0); }, set(value) { define(window, { length: value }); } },
  localStorage: stored("local"), sessionStorage: stored("session"),
  innerWidth: { get() { return host("viewport")[0]; } },
  innerHeight: { get() { return host("viewport")[1]; } },
  outerWidth: { get() { return host("around")[0]; } },
  outerHeight: { get() { return host("around")[1]; } },
  devicePixelRatio: { get() { return host("around")[4]; } },
  scrollX: { get() { return host("viewport")[2]; } },
  scrollY: { get() { return host("viewport")[3]; } },
  pageXOffset: { get() { return host("viewport")[2]; } },
  pageYOffset: { get() { return host("viewport")[3]; } },
  screenX: { get() { return host("around")[2]; } }, screenY: { get() { return host("around")[3]; } },
  screenLeft: { get() { return host("around")[2]; } }, screenTop: { get() { return host("around")[3]; } },
});

// ---- what light calls

// eventOf is the event of a type light fires, of the class a browser
// gives it.
const eventOf = (type, init) => {
  const full = { bubbles: true, cancelable: true, composed: true, ...init };
  let event;
  if (/^(click|dblclick|mousedown|mouseup|mousemove|mouseover|mouseout|contextmenu)$/.test(type)) event = new MouseEvent(type, full);
  else if (/^mouse(enter|leave)$/.test(type)) event = new MouseEvent(type, { ...full, bubbles: false, cancelable: false });
  else if (type.startsWith("pointer")) event = new PointerEvent(type, full);
  else if (type.startsWith("key")) event = new KeyboardEvent(type, full);
  else if (type === "input") event = new InputEvent(type, { ...full, cancelable: false });
  else if (type === "focus" || type === "blur") event = new FocusEvent(type, { ...full, bubbles: false, cancelable: false });
  else if (type === "focusin" || type === "focusout") event = new FocusEvent(type, { ...full, cancelable: false });
  else if (type === "submit") event = new SubmitEvent(type, { ...full, submitter: wrap(full.submitter || 0) });
  else if (/^(scroll|resize|load|error)$/.test(type)) event = new Event(type, { ...full, bubbles: false, cancelable: false });
  else event = new Event(type, full);
  event.isTrusted = true;
  return event;
};
const targetOf = (ref) => (ref === "window" ? window : ref === "document" ? document : wrap(ref));

// sweep lets go of the objects of nodes that left the page: one that no
// script holds any more is light's to forget.
const sweep = () => {
  const [detached, attached] = host("handles");
  for (const ref of detached) {
    const node = wrappers.get(ref);
    if (node && !(node instanceof WeakRef)) wrappers.set(ref, new WeakRef(node));
  }
  for (const ref of attached) {
    const node = wrappers.get(ref);
    if (node instanceof WeakRef) {
      const alive = node.deref();
      if (alive) wrappers.set(ref, alive);
    }
  }
  const dead = [];
  for (const [ref, node] of wrappers) {
    if (node instanceof WeakRef && node.deref() === undefined) dead.push(ref);
  }
  for (const ref of dead) wrappers.delete(ref);
  if (dead.length) host("release", dead);
  return wrappers.size;
};

const calls = {
  tick,
  sweep,
  fonts: fontsMoved,
  answered,
  arrived,
  // event fires an event at a node, the window or the document, and
  // says whether its default was prevented.
  event(ref, type, init) {
    const target = targetOf(ref);
    return target ? !dispatch(target, eventOf(type, init || {})) : false;
  },
  // message tells the window of what another posted it: what it wrote
  // down, read here, with the origin and the window light knows it by.
  message(text, origin, source) {
    let data;
    try {
      data = read(text);
    } catch (_) {
      dispatch(window, messageOf({ origin, source: windowOf(source) }, "messageerror"));
      return;
    }
    dispatch(window, messageOf({ data, origin, source: windowOf(source), ports: [] }));
  },
  // script names the script element that runs, null between two.
  script(ref) { document.currentScript = wrap(ref); },
  // ready moves the document on in its load.
  ready(state) {
    document.readyState = state;
    dispatch(document, new Event("readystatechange"));
    if (state === "interactive") dispatch(document, new Event("DOMContentLoaded", { bubbles: true }));
    if (state === "complete") {
      dispatch(window, new Event("load"));
      dispatch(window, new PageTransitionEvent("pageshow"));
    }
  },
  // failed tells the window of an exception light caught for a script.
  failed(message, filename, lineno, colno) {
    reporting = true;
    try {
      const event = new ErrorEvent("error", { cancelable: true, message, filename, lineno, colno, error: new Error(message) });
      return !dispatch(window, event);
    } finally {
      reporting = false;
    }
  },
  // evaluate is what the console's line gives: its value as the console
  // prints it, kept as $_. A let or a const the line begins with is a
  // var, so the next line still has it.
  evaluate(source) {
    const value = (0, eval)(String(source).replace(/^\s*(?:let|const)\s/, "var "));
    define(window, { $_: value });
    return quoted(value);
  },
  // inspect names the node the developer tools have selected, for the
  // console: $0, null for none.
  inspect(ref) { define(window, { $0: wrap(ref) }); },
};
return function light(call, ...args) { return calls[call](...args); };
