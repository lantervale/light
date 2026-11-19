// What light's WebDriver asks of a page, in the page's own context: the
// elements it refers to, what it reads of them, and the scripts it runs
// with their arguments and results as the standard copies them. light
// evaluates `__lightwd.call(json)` and reads the JSON it answers.
(function () {
  "use strict";
  if (window.__lightwd) return 1;
  const ELEMENT = "element-6066-11e4-a52e-4f735466cecf";
  const WINDOW = "window-fcc6-11e5-b4f8-330a88ab9d7f";
  const FRAME = "frame-075b-4da1-b6ba-e579c2d3230a";
  const SHADOW = "shadow-6066-11e4-a52e-4f735466cecf";

  class Failure extends Error {
    constructor(code, message, stack) {
      super(message);
      this.code = code;
      this.calls = stack || "";
    }
  }
  const fail = (code, message) => { throw new Failure(code, message); };

  const prefix = Math.random().toString(16).slice(2, 10) + "-";
  const nodes = new Map();
  const ids = new WeakMap();
  let last = 0;
  let handle = "";

  const refer = (node) => {
    let id = ids.get(node);
    if (!id) {
      id = prefix + (++last);
      ids.set(node, id);
      nodes.set(id, node);
    }
    return id;
  };
  const known = (id) => {
    const node = nodes.get(String(id));
    if (!node) fail("no such element", "no element " + id + " is known in this document");
    if (!node.isConnected) fail("stale element reference", "the element " + id + " is no longer in its document");
    return node;
  };

  const isCollection = (value) => Array.isArray(value) || value instanceof NodeList ||
    value instanceof HTMLCollection || Object.prototype.toString.call(value) === "[object Arguments]";

  // clone is a value as the standard sends it back: elements as
  // references, collections as lists, objects by their own properties.
  const clone = (value, seen) => {
    if (value === undefined || value === null) return null;
    const type = typeof value;
    if (type === "boolean" || type === "string") return value;
    if (type === "number") return Number.isFinite(value) ? value : null;
    if (type === "bigint" || type === "symbol") fail("javascript error", "a " + type + " cannot be sent back");
    if (value instanceof Element) {
      if (!value.isConnected) fail("stale element reference", "the element returned is in no document");
      return { [ELEMENT]: refer(value) };
    }
    if (value === window) return { [window.top === window ? WINDOW : FRAME]: handle };
    if (type === "object" && typeof value.toJSON === "function") return clone(value.toJSON(), seen);
    if (seen.includes(value)) fail("javascript error", "the value returned refers to itself");
    seen.push(value);
    let out;
    if (isCollection(value)) {
      out = Array.from(value, (item) => clone(item, seen));
    } else {
      out = {};
      for (const key of Object.keys(value)) out[key] = clone(value[key], seen);
    }
    seen.pop();
    return out;
  };

  // revive is an argument as the script gets it: references as their
  // elements.
  const revive = (value) => {
    if (value === null || typeof value !== "object") return value;
    if (Array.isArray(value)) return value.map(revive);
    if (Object.prototype.hasOwnProperty.call(value, ELEMENT)) return known(value[ELEMENT]);
    if (Object.prototype.hasOwnProperty.call(value, WINDOW) || Object.prototype.hasOwnProperty.call(value, FRAME)) {
      return window;
    }
    if (Object.prototype.hasOwnProperty.call(value, SHADOW)) fail("no such shadow root", "light has no shadow tree");
    const out = {};
    for (const key of Object.keys(value)) out[key] = revive(value[key]);
    return out;
  };

  const thrown = (error) => {
    if (error instanceof Failure) return error;
    const message = error instanceof Error ? error.name + ": " + error.message : String(error);
    return new Failure("javascript error", message, error instanceof Error && error.stack ? String(error.stack) : "");
  };

  // jobs are the scripts that run, by their number: pending, then a
  // value or a failure.
  const jobs = new Map();
  let lastJob = 0;
  const start = (source, args, async) => {
    const id = ++lastJob;
    const job = { done: false };
    jobs.set(id, job);
    const settle = (value) => {
      if (job.done) return;
      try {
        job.value = clone(value, []);
      } catch (error) {
        job.error = thrown(error);
      }
      job.done = true;
    };
    const reject = (error) => {
      if (job.done) return;
      job.error = thrown(error);
      job.done = true;
    };
    try {
      const run = new Function(source);
      const given = revive(args);
      if (async) {
        const result = run.apply(window, given.concat([settle]));
        if (result && typeof result.then === "function") result.then(() => {}, reject);
      } else {
        Promise.resolve(run.apply(window, given)).then(settle, reject);
      }
    } catch (error) {
      reject(error);
    }
    return id;
  };
  const poll = (id) => {
    const job = jobs.get(id);
    if (!job) fail("unknown error", "no script " + id + " runs");
    if (!job.done) return { pending: true };
    jobs.delete(id);
    if (job.error) throw job.error;
    return { value: job.value };
  };

  const textOf = (element) => {
    const text = typeof element.innerText === "string" ? element.innerText : element.textContent;
    return String(text || "").replace(/[ \t\r\n\f]+/g, " ").trim();
  };

  const find = (using, value, from) => {
    const root = from === "" ? document : known(from);
    let found;
    switch (using) {
      case "css selector":
        try {
          found = root.querySelectorAll(value);
        } catch (error) {
          fail("invalid selector", String(value) + " is no selector light reads: " + error.message);
        }
        break;
      case "tag name":
        found = root.getElementsByTagName(value);
        break;
      case "link text":
        found = Array.from(root.querySelectorAll("a[href]")).filter((a) => textOf(a) === String(value).trim());
        break;
      case "partial link text":
        found = Array.from(root.querySelectorAll("a[href]")).filter((a) => textOf(a).includes(value));
        break;
      case "xpath": {
        let snapshot;
        try {
          snapshot = document.evaluate(value, root, null, XPathResult.ORDERED_NODE_SNAPSHOT_TYPE, null);
        } catch (error) {
          fail("invalid selector", String(value) + " is no XPath that selects nodes: " + error.message);
        }
        found = Array.from({ length: snapshot.snapshotLength }, (_, i) => snapshot.snapshotItem(i));
        if (found.some((node) => node.nodeType !== 1)) {
          fail("invalid selector", String(value) + " selects what is not an element");
        }
        break;
      }
      default:
        fail("unsupported operation", "light finds no element by " + using);
    }
    return Array.from(found, refer);
  };

  const booleans = new Set(["allowfullscreen", "async", "autofocus", "autoplay", "checked", "controls", "default",
    "defer", "disabled", "formnovalidate", "hidden", "inert", "ismap", "itemscope", "loop", "multiple", "muted",
    "nomodule", "novalidate", "open", "playsinline", "readonly", "required", "reversed", "selected"]);

  const controls = new Set(["button", "input", "select", "textarea", "optgroup", "option", "fieldset"]);
  const tagOf = (element) => String(element.tagName || "").toLowerCase();

  const enabled = (element) => {
    if (!controls.has(tagOf(element))) return true;
    for (let at = element; at && at.nodeType === 1; at = at.parentNode) {
      if (controls.has(tagOf(at)) && at.hasAttribute("disabled")) return false;
    }
    return true;
  };

  const displayed = (element) => {
    for (let at = element; at && at.nodeType === 1; at = at.parentNode) {
      if (at.hasAttribute("hidden")) return false;
      const style = getComputedStyle(at);
      if (style.display === "none") return false;
      if (at === element && (style.visibility === "hidden" || style.visibility === "collapse")) return false;
    }
    return true;
  };

  const rect = (element) => {
    const box = element.getBoundingClientRect();
    return { x: box.left, y: box.top, width: box.width, height: box.height };
  };

  const typed = new Set(["text", "search", "url", "tel", "email", "password", "number", "date", "month", "week",
    "time", "datetime-local", "color", "range", "file", ""]);
  const editable = (element) => {
    const tag = tagOf(element);
    if (tag === "textarea") return true;
    if (tag === "input") return typed.has(String(element.getAttribute("type") || "").toLowerCase());
    return element.isContentEditable === true;
  };

  const commands = {
    handle(name) { handle = name; return null; },
    start(source, args, async) { return start(source, args, async); },
    poll(id) { return poll(id); },
    drop(id) { jobs.delete(id); return null; },
    ready() { return document.readyState; },
    title() { return document.title; },
    source() {
      const doctype = document.doctype ? "<!DOCTYPE " + document.doctype.name + ">" : "";
      return doctype + (document.documentElement ? document.documentElement.outerHTML : "");
    },
    find(using, value, from) { return find(using, value, from); },
    active() {
      const element = document.activeElement || document.body;
      if (!element) fail("no such element", "the document has no active element");
      return refer(element);
    },
    attribute(id, name) {
      const element = known(id);
      if (!element.hasAttribute(name)) return null;
      return booleans.has(String(name).toLowerCase()) ? "true" : element.getAttribute(name);
    },
    property(id, name) { return clone(known(id)[name], []); },
    css(id, name) { return String(getComputedStyle(known(id)).getPropertyValue(name)); },
    text(id) { return textOf(known(id)); },
    tag(id) { return tagOf(known(id)); },
    rect(id) { return rect(known(id)); },
    enabled(id) { return enabled(known(id)); },
    selected(id) {
      const element = known(id);
      const tag = tagOf(element);
      if (tag === "option") return element.selected === true;
      if (tag === "input") return element.checked === true;
      return false;
    },
    displayed(id) { return displayed(known(id)); },
    // aim scrolls an element into view, to be pressed or typed in.
    aim(id) {
      const element = known(id);
      element.scrollIntoView();
      return null;
    },
    // target is where an element is pressed: its box, and whether it is
    // shown and may be used.
    target(id) {
      const element = known(id);
      return { rect: rect(element), displayed: displayed(element), enabled: enabled(element), tag: tagOf(element),
        type: String(element.getAttribute("type") || "").toLowerCase() };
    },
    focus(id) {
      const element = known(id);
      if (document.activeElement !== element) element.focus();
      if (document.activeElement === element && typeof element.value === "string" && element.setSelectionRange) {
        try {
          element.setSelectionRange(element.value.length, element.value.length);
        } catch (error) { /* a field that has no caret */ }
      }
      return { focused: document.activeElement === element, editable: editable(element), enabled: enabled(element) };
    },
    // clear empties what is editable and no field, and says of a field
    // whether it holds text, which light deletes as its user would.
    clear(id) {
      const element = known(id);
      if (!editable(element) || !enabled(element) || element.readOnly === true) {
        fail("invalid element state", "the element is not one that can be cleared: it is not editable");
      }
      if (!displayed(element)) fail("element not interactable", "the element is not shown");
      if (typeof element.value === "string") return element.value !== "";
      element.textContent = "";
      return false;
    },
    // frame is the place among the window's frames of the one an
    // element shows, -1 for an element that shows none.
    frame(id) {
      const element = known(id);
      const tag = tagOf(element);
      if (tag !== "iframe" && tag !== "frame") fail("no such frame", "the element is no frame");
      const shown = element.contentWindow;
      for (let i = 0; shown && i < window.length; i++) {
        if (window[i] === shown) return i;
      }
      return -1;
    },
  };

  const call = (json) => {
    try {
      const asked = JSON.parse(json);
      const value = commands[asked.command].apply(null, asked.args || []);
      return JSON.stringify({ value: value === undefined ? null : value });
    } catch (error) {
      const failure = thrown(error);
      return JSON.stringify({ error: failure.code, message: failure.message, stack: failure.calls });
    }
  };

  Object.defineProperty(window, "__lightwd", { value: Object.freeze({ call }), enumerable: false, configurable: true });
  return 1;
})()
