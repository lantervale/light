// ---- the window: what scripts find as globals

byName(Document.prototype, (page, name) => {
  const as = `[name="${CSS.escape(name)}"]`;
  return page.querySelector(["form", "img", "embed", "object", "iframe"].map((tag) => tag + as).join());
});
const document = new Document(token);

// A window names the elements of its page by their id, where nothing
// else has the name.
class Window extends EventTarget {}
Object.setPrototypeOf(Window.prototype, new Proxy(Object.create(EventTarget.prototype), {
  has(target, name) { return name in target || (typeof name === "string" && host("byId", name) !== 0); },
  get(target, name, receiver) {
    if (name in target || typeof name !== "string") return Reflect.get(target, name, receiver);
    return wrap(host("byId", name)) || undefined;
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
  window, self: window, globalThis: window, top: window, parent: window, frames: window, document, console,
  setTimeout, clearTimeout, setInterval, clearInterval: clearTimeout, requestAnimationFrame, cancelAnimationFrame,
  queueMicrotask, requestIdleCallback, cancelIdleCallback: clearTimeout, setImmediate: undefined,
  history, navigator, performance, crypto, customElements, CSS, structuredClone,
  localStorage: storage("local"), sessionStorage: storage("session"),
  screen: {
    get width() { return host("viewport")[0]; }, get height() { return host("viewport")[1]; },
    get availWidth() { return host("viewport")[0]; }, get availHeight() { return host("viewport")[1]; },
    colorDepth: 24, pixelDepth: 24, orientation: { type: "landscape-primary", angle: 0, addEventListener() {} },
  },
  visualViewport: undefined, devicePixelRatio: 1, name: "", closed: false, opener: null, frameElement: null, length: 0,
  isSecureContext: here.protocol === "https:", origin: here.origin, event: undefined, status: "",
  alert(message) { host("log", "info", "alert: " + (message === undefined ? "" : String(message))); },
  confirm(message) { host("log", "info", "confirm, answered no: " + String(message)); return false; },
  prompt(message) { host("log", "info", "prompt, left unanswered: " + String(message)); return null; },
  print() {}, focus() {}, blur() {}, stop() {}, close() {},
  open() { host("log", "warn", "light opens no window for a script"); return null; },
  postMessage(data) {
    setTimeout(() => dispatch(window, new MessageEvent("message", { data: structuredClone(data), origin: here.origin, source: window })), 0);
  },
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
  MessageEvent, TransitionEvent, AnimationEvent, WheelEvent, TouchEvent, DragEvent, ClipboardEvent,
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
accessors(window, {
  location: { get() { return location; }, set(address) { go(address, false); } },
  innerWidth: { get() { return host("viewport")[0]; } },
  innerHeight: { get() { return host("viewport")[1]; } },
  outerWidth: { get() { return host("viewport")[0]; } },
  outerHeight: { get() { return host("viewport")[1]; } },
  scrollX: { get() { return host("viewport")[2]; } },
  scrollY: { get() { return host("viewport")[3]; } },
  pageXOffset: { get() { return host("viewport")[2]; } },
  pageYOffset: { get() { return host("viewport")[3]; } },
  screenX: { get() { return 0; } }, screenY: { get() { return 0; } },
  screenLeft: { get() { return 0; } }, screenTop: { get() { return 0; } },
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
