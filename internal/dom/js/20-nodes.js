// ---- nodes
//
// A node of the page's tree is known to scripts by a number light gave
// it: a text node's is negative. wrap gives the one object a script
// ever sees for it.

const REF = Symbol("ref");
const TAG = Symbol("tag");
const NS = Symbol("namespace");
const token = Symbol("light");
const wrappers = new Map();
const ELEMENT_NODE = 1, TEXT_NODE = 3, COMMENT_NODE = 8, DOCUMENT_NODE = 9, DOCTYPE_NODE = 10, FRAGMENT_NODE = 11;
const FRAGMENT = "#document-fragment";
const HTML_NS = "http://www.w3.org/1999/xhtml";
// namespaces are those of the elements that are not HTML's, by the name
// light knows each by.
const namespaces = { svg: "http://www.w3.org/2000/svg", math: "http://www.w3.org/1998/Math/MathML" };
const namespaceOf = (uri) => Object.keys(namespaces).find((name) => namespaces[name] === uri) || "";
// objectOf is the object of the node of a number, of that name.
const objectOf = (ref, tag) => {
  if (tag === FRAGMENT) return new DocumentFragment(token, ref);
  if (tag === "#comment") return new Comment(token, ref);
  if (tag === "#doctype") return new DocumentType(token, ref);
  const colon = tag.indexOf(":");
  if (colon < 0) return new (classOf(tag))(token, ref, tag);
  const namespace = tag.slice(0, colon);
  return new (namespace === "svg" ? SVGElement : Element)(token, ref, tag.slice(colon + 1), namespace);
};

const wrap = (ref) => {
  if (!ref) return null;
  let node = wrappers.get(ref);
  if (node) {
    if (node instanceof WeakRef) {
      const alive = node.deref();
      if (alive) return alive;
    } else {
      return node;
    }
  }
  node = ref < 0 ? new Text(token, ref) : objectOf(ref, host("tag", ref));
  wrappers.set(ref, node);
  return node;
};
const wrapAll = (refs) => NodeList.from(refs.map(wrap));
const refOf = (node, what = "a node") => {
  if (!(node instanceof Node) || node[REF] === undefined) throw new TypeError(`${what} is not a Node`);
  return node[REF];
};

class NodeList extends Array {
  static get [Symbol.species]() { return Array; }
  item(i) { return this[i] === undefined ? null : this[i]; }
  namedItem(name) {
    return this.find((n) => n instanceof Element && (n.id === name || n.getAttribute("name") === name)) || null;
  }
  static from(nodes) {
    const list = new NodeList();
    for (const n of nodes) list.push(n);
    return list;
  }
}
// An HTMLCollection also names its elements by their id or name.
const namedItems = {
  has(list, name) { return name in list || (typeof name === "string" && list.namedItem(name) !== null); },
  get(list, name) {
    if (typeof name !== "string" || name in list) return Reflect.get(list, name, list);
    const found = list.namedItem(name);
    return found === null ? undefined : found;
  },
};
class HTMLCollection extends NodeList {
  static from(nodes) {
    const list = new this();
    for (const n of nodes) list.push(n);
    return new Proxy(list, namedItems);
  }
}
class RadioNodeList extends NodeList {
  get value() {
    const on = this.find((n) => n.checked);
    return on ? on.value : "";
  }
  set value(value) {
    const one = this.find((n) => n.type === "radio" && n.value === String(value));
    if (one) one.checked = true;
  }
}
// A form's controls of one name are a list of them.
class HTMLFormControlsCollection extends HTMLCollection {
  namedItem(name) {
    const all = this.filter((n) => n.id === name || n.getAttribute("name") === name);
    if (all.length < 2) return all[0] || null;
    const list = new RadioNodeList();
    for (const n of all) list.push(n);
    return list;
  }
}

// byName has the objects of a prototype answer, for a name nothing else
// of them has, with what find finds under it: a form its controls, the
// document its forms and pictures.
const byName = (prototype, find) => {
  let looking = false;
  const own = (target, name, receiver) => typeof name !== "string" || name in target || looking ||
    !(receiver instanceof Node) || !Object.hasOwn(receiver, REF);
  const found = (receiver, name) => {
    looking = true;
    try {
      return find(receiver, name);
    } finally {
      looking = false;
    }
  };
  Object.setPrototypeOf(prototype, new Proxy(Object.create(Object.getPrototypeOf(prototype)), {
    has(target, name) { return name in target; },
    get(target, name, receiver) {
      if (own(target, name, receiver)) return Reflect.get(target, name, receiver);
      const item = found(receiver, name);
      return item === null ? undefined : item;
    },
  }));
};

// observed tells the mutation observers of a change (60-observers.js),
// and watching is whether there is any to tell.
let observed = () => {};
let watching = false;

class Node extends EventTarget {
  constructor(key, ref) {
    super();
    if (key !== token) illegal();
    this[REF] = ref;
  }
  get nodeType() { return ELEMENT_NODE; }
  get nodeName() { return ""; }
  get nodeValue() { return null; }
  set nodeValue(_) { /* an element has none */ }
  get ownerDocument() { return document; }
  get isConnected() { return host("connected", this[REF]); }
  get parentNode() {
    const parent = host("parent", this[REF]);
    return parent === "document" ? document : wrap(parent);
  }
  get parentElement() {
    const parent = this.parentNode;
    return parent instanceof Element ? parent : null;
  }
  get childNodes() { return wrapAll(host("children", this[REF], false)); }
  get firstChild() { return wrap(host("child", this[REF], 0, false)); }
  get lastChild() { return wrap(host("child", this[REF], -1, false)); }
  get nextSibling() { return wrap(host("sibling", this[REF], 1, false)); }
  get previousSibling() { return wrap(host("sibling", this[REF], -1, false)); }
  get textContent() { return host("text", this[REF]); }
  set textContent(text) {
    const removed = watching ? this.childNodes : null;
    host("setText", this[REF], text === null || text === undefined ? "" : String(text));
    if (watching) observed({ type: "childList", target: this, removedNodes: removed, addedNodes: this.childNodes });
  }
  hasChildNodes() { return host("child", this[REF], 0, false) !== 0; }
  getRootNode() {
    let root = this;
    while (root.parentNode) root = root.parentNode;
    return root;
  }
  contains(other) {
    if (other === null || other === undefined) return false;
    if (this === document) return other === document || other.isConnected;
    return other instanceof Node && other[REF] !== undefined && host("contains", this[REF], other[REF]);
  }
  appendChild(child) { return insert(this, child, null); }
  insertBefore(child, before) { return insert(this, child, before || null); }
  removeChild(child) {
    if (!(child instanceof Node) || child.parentNode !== this) {
      throw new DOMException("The node to be removed is not a child of this node.", "NotFoundError");
    }
    detach(child);
    return child;
  }
  replaceChild(child, old) {
    if (!(old instanceof Node) || old.parentNode !== this) {
      throw new DOMException("The node to be replaced is not a child of this node.", "NotFoundError");
    }
    if (child === old) return old;
    insert(this, child, old);
    detach(old);
    return old;
  }
  cloneNode(deep) { return wrap(host("clone", this[REF], Boolean(deep))); }
  isSameNode(other) { return this === other; }
  isEqualNode(other) {
    return other instanceof Node && this.nodeType === other.nodeType &&
      (this instanceof Element ? this.outerHTML === other.outerHTML : this.textContent === other.textContent);
  }
  compareDocumentPosition(other) {
    if (this === other) return 0;
    if (!(other instanceof Node) || other[REF] === undefined) return 1;
    return host("compare", this[REF], other[REF]);
  }
  normalize() {
    for (const child of this.childNodes) {
      if (!(child instanceof Text)) {
        child.normalize();
        continue;
      }
      const before = child.previousSibling;
      if (child.data === "") detach(child);
      else if (before instanceof Text) {
        before.data += child.data;
        detach(child);
      }
    }
  }
  lookupNamespaceURI() { return null; }
  isDefaultNamespace() { return true; }
}
const nodeTypes = {
  ELEMENT_NODE, ATTRIBUTE_NODE: 2, TEXT_NODE, CDATA_SECTION_NODE: 4, PROCESSING_INSTRUCTION_NODE: 7, COMMENT_NODE,
  DOCUMENT_NODE, DOCUMENT_TYPE_NODE: 10, DOCUMENT_FRAGMENT_NODE: FRAGMENT_NODE,
  DOCUMENT_POSITION_DISCONNECTED: 1, DOCUMENT_POSITION_PRECEDING: 2, DOCUMENT_POSITION_FOLLOWING: 4,
  DOCUMENT_POSITION_CONTAINS: 8, DOCUMENT_POSITION_CONTAINED_BY: 16,
};
define(Node, nodeTypes);
define(Node.prototype, nodeTypes);

// insert puts child in parent before another of its children, at its
// end for none: a fragment gives its children in its place.
const insert = (parent, child, before) => {
  if (parent === document) parent = document.documentElement;
  const into = refOf(parent);
  if (before !== null && (!(before instanceof Node) || before.parentNode !== parent)) {
    throw new DOMException("The node before which the new node is to be inserted is not a child of this node.",
      "NotFoundError");
  }
  if (child instanceof DocumentFragment) {
    for (const one of child.childNodes) insert(parent, one, before);
    return child;
  }
  const ref = refOf(child, "the child");
  if (child === parent || (child instanceof Element && host("contains", ref, into))) {
    throw new DOMException("The new child element contains the parent.", "HierarchyRequestError");
  }
  if (child.parentNode) detach(child);
  host("insert", into, ref, before ? before[REF] : 0);
  if (watching) {
    observed({
      type: "childList", target: parent, addedNodes: [child], removedNodes: [],
      previousSibling: child.previousSibling, nextSibling: child.nextSibling,
    });
  }
  return child;
};
const detach = (child) => {
  const parent = child.parentNode;
  if (!parent || parent === document) return;
  const previousSibling = watching ? child.previousSibling : null, nextSibling = watching ? child.nextSibling : null;
  host("remove", child[REF]);
  if (watching) observed({ type: "childList", target: parent, addedNodes: [], removedNodes: [child], previousSibling, nextSibling });
};
// nodesOf turns what append and its kin take, nodes and strings, into
// one node.
const nodesOf = (items) => {
  if (items.length === 1) return items[0] instanceof Node ? items[0] : document.createTextNode(String(items[0]));
  const fragment = document.createDocumentFragment();
  for (const item of items) fragment.appendChild(item instanceof Node ? item : document.createTextNode(String(item)));
  return fragment;
};

class CharacterData extends Node {
  get data() { return host("text", this[REF]); }
  set data(text) {
    const oldValue = watching ? this.data : null;
    host("setText", this[REF], String(text));
    if (watching) observed({ type: "characterData", target: this, oldValue });
  }
  get nodeValue() { return this.data; }
  set nodeValue(text) { this.data = text; }
  get textContent() { return this.data; }
  set textContent(text) { this.data = text === null ? "" : text; }
  get length() { return this.data.length; }
  get childNodes() { return new NodeList(); }
  get firstChild() { return null; }
  get lastChild() { return null; }
  hasChildNodes() { return false; }
  get nextElementSibling() { return wrap(host("sibling", this[REF], 1, true)); }
  get previousElementSibling() { return wrap(host("sibling", this[REF], -1, true)); }
  appendData(text) { this.data += text; }
  remove() { detach(this); }
  before(...items) { if (this.parentNode) insert(this.parentNode, nodesOf(items), this); }
  after(...items) { if (this.parentNode) insert(this.parentNode, nodesOf(items), this.nextSibling); }
  replaceWith(...items) {
    const parent = this.parentNode;
    if (!parent) return;
    const next = this.nextSibling;
    detach(this);
    insert(parent, nodesOf(items), next);
  }
}
class Text extends CharacterData {
  constructor(key, ref) {
    if (key === token) {
      super(token, ref);
      return;
    }
    const made = host("createText", key === undefined ? "" : String(key));
    super(token, made);
    wrappers.set(made, this);
  }
  get nodeType() { return TEXT_NODE; }
  get nodeName() { return "#text"; }
  get wholeText() { return this.data; }
  splitText(at) {
    const rest = document.createTextNode(this.data.slice(at));
    this.data = this.data.slice(0, at);
    if (this.parentNode) insert(this.parentNode, rest, this.nextSibling);
    return rest;
  }
}
class Comment extends CharacterData {
  constructor(key, ref) {
    if (key === token) {
      super(token, ref);
      return;
    }
    const made = host("createComment", key === undefined ? "" : String(key));
    super(token, made);
    wrappers.set(made, this);
  }
  get nodeType() { return COMMENT_NODE; }
  get nodeName() { return "#comment"; }
}
class DocumentType extends Node {
  constructor(key, ref) {
    if (key !== token) illegal();
    super(token, ref);
  }
  get nodeType() { return DOCTYPE_NODE; }
  get name() { return host("text", this[REF]); }
  get nodeName() { return this.name; }
  get publicId() { return ""; }
  get systemId() { return ""; }
  get textContent() { return null; }
  set textContent(_) {}
  get childNodes() { return new NodeList(); }
  get firstChild() { return null; }
  get lastChild() { return null; }
  hasChildNodes() { return false; }
  remove() { detach(this); }
}

class DocumentFragment extends Node {
  constructor(key, ref) {
    if (key === token) {
      super(token, ref);
      return;
    }
    const made = host("create", FRAGMENT);
    super(token, made);
    wrappers.set(made, this);
  }
  get nodeType() { return FRAGMENT_NODE; }
  get nodeName() { return FRAGMENT; }
  get parentNode() { return null; }
  get isConnected() { return false; }
  getElementById(id) { return this.querySelector("#" + CSS.escape(id)); }
}

// parentMixin is what elements, fragments and the document share.
const parentMixin = {
  children: { get() { return HTMLCollection.from(host("children", this[REF], true).map(wrap)); } },
  firstElementChild: { get() { return wrap(host("child", this[REF], 0, true)); } },
  lastElementChild: { get() { return wrap(host("child", this[REF], -1, true)); } },
  childElementCount: { get() { return host("children", this[REF], true).length; } },
};
const parentMethods = {
  append(...items) { if (items.length) insert(this, nodesOf(items), null); },
  prepend(...items) { if (items.length) insert(this, nodesOf(items), this.firstChild); },
  replaceChildren(...items) {
    for (const child of this.childNodes) detach(child);
    if (items.length) insert(this, nodesOf(items), null);
  },
  querySelector(selector) { return wrap(host("query", this[REF], String(selector), false)); },
  querySelectorAll(selector) { return wrapAll(host("query", this[REF], String(selector), true)); },
};
accessors(DocumentFragment.prototype, parentMixin);
define(DocumentFragment.prototype, parentMethods);

// ---- tokens, styles, data

class DOMTokenList {
  constructor(element, attribute) {
    this._element = element;
    this._attribute = attribute;
  }
  _tokens() { return (this._element.getAttribute(this._attribute) || "").split(/\s+/).filter(Boolean); }
  _set(tokens) { this._element.setAttribute(this._attribute, tokens.join(" ")); }
  get length() { return this._tokens().length; }
  get value() { return this._element.getAttribute(this._attribute) || ""; }
  set value(text) { this._element.setAttribute(this._attribute, String(text)); }
  item(i) { const token = this._tokens()[i]; return token === undefined ? null : token; }
  contains(token) { return this._tokens().includes(String(token)); }
  add(...tokens) {
    const now = this._tokens();
    for (const token of tokens.map(String)) if (!now.includes(token)) now.push(token);
    this._set(now);
  }
  remove(...tokens) {
    const gone = tokens.map(String);
    this._set(this._tokens().filter((token) => !gone.includes(token)));
  }
  toggle(token, force) {
    const has = this.contains(token);
    const on = force === undefined ? !has : Boolean(force);
    if (on && !has) this.add(token);
    if (!on && has) this.remove(token);
    return on;
  }
  replace(old, token) {
    if (!this.contains(old)) return false;
    this._set(this._tokens().map((t) => (t === String(old) ? String(token) : t)));
    return true;
  }
  supports() { return true; }
  forEach(fn, self) { this._tokens().forEach((token, i) => fn.call(self, token, i, this)); }
  keys() { return this._tokens().keys(); }
  values() { return this._tokens().values(); }
  entries() { return this._tokens().entries(); }
  [Symbol.iterator]() { return this._tokens()[Symbol.iterator](); }
  toString() { return this.value; }
}

const kebab = (name) => (name.startsWith("--") ? name
  : name.replace(/^(webkit|moz|ms|o)(?=[A-Z])/, "-$1").replace(/[A-Z]/g, (c) => "-" + c.toLowerCase()));
const declarations = (text) => {
  const out = new Map();
  let depth = 0, quote = "", start = 0;
  const take = (end) => {
    const part = text.slice(start, end);
    const colon = part.indexOf(":");
    if (colon > 0) {
      const name = part.slice(0, colon).trim();
      const value = part.slice(colon + 1).trim();
      if (name && value) out.set(name.startsWith("--") ? name : name.toLowerCase(), value);
    }
    start = end + 1;
  };
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quote) { if (c === quote) quote = ""; continue; }
    if (c === '"' || c === "'") quote = c;
    else if (c === "(") depth++;
    else if (c === ")") depth--;
    else if (c === ";" && depth === 0) take(i);
  }
  take(text.length);
  return out;
};
const styleMethods = {
  getPropertyValue(style, name) {
    const value = style.map.get(kebab(String(name)));
    return value === undefined ? "" : value.replace(/\s*!important$/, "");
  },
  getPropertyPriority(style, name) {
    return /!important$/.test(style.map.get(kebab(String(name))) || "") ? "important" : "";
  },
  setProperty(style, name, value, priority) {
    if (value === null || value === undefined || value === "") return styleMethods.removeProperty(style, name);
    style.map.set(kebab(String(name)), String(value) + (priority ? " !important" : ""));
    style.write();
    return undefined;
  },
  removeProperty(style, name) {
    const old = styleMethods.getPropertyValue(style, name);
    style.map.delete(kebab(String(name)));
    style.write();
    return old;
  },
  item(style, i) { return Array.from(style.map.keys())[i] || ""; },
};
// styleOf is an element's style attribute as scripts write it:
// el.style.color, el.style.setProperty(…), el.style.cssText.
const styles = new WeakMap();
const styleOf = (element) => {
  let proxy = styles.get(element);
  if (proxy) return proxy;
  const style = {
    get map() { return declarations(element.getAttribute("style") || ""); },
    write: null,
  };
  const view = (read) => {
    const map = style.map;
    return read({
      map,
      write() {
        const text = Array.from(map, ([name, value]) => `${name}: ${value};`).join(" ");
        if (text) element.setAttribute("style", text);
        else element.removeAttribute("style");
      },
    });
  };
  proxy = new Proxy({}, {
    get(_, name) {
      if (typeof name !== "string") return undefined;
      if (name === "cssText") return element.getAttribute("style") || "";
      if (name === "length") return style.map.size;
      if (name === "parentRule") return null;
      if (name in styleMethods) return (...args) => view((s) => styleMethods[name](s, ...args));
      if (name === "cssFloat") name = "float";
      return view((s) => styleMethods.getPropertyValue(s, name));
    },
    set(_, name, value) {
      if (typeof name !== "string") return true;
      if (name === "cssText") {
        if (value) element.setAttribute("style", String(value));
        else element.removeAttribute("style");
        return true;
      }
      if (name === "cssFloat") name = "float";
      view((s) => styleMethods.setProperty(s, name, value));
      return true;
    },
    has(_, name) { return typeof name === "string"; },
    ownKeys() { return Array.from(style.map.keys()); },
    getOwnPropertyDescriptor(_, name) {
      return { value: view((s) => styleMethods.getPropertyValue(s, name)), enumerable: true, configurable: true, writable: true };
    },
  });
  styles.set(element, proxy);
  return proxy;
};

const dataName = (name) => "data-" + name.replace(/[A-Z]/g, (c) => "-" + c.toLowerCase());
const datasets = new WeakMap();
const datasetOf = (element) => {
  let proxy = datasets.get(element);
  if (proxy) return proxy;
  const names = () => element.getAttributeNames().filter((n) => n.startsWith("data-"))
    .map((n) => n.slice(5).replace(/-([a-z])/g, (_, c) => c.toUpperCase()));
  proxy = new Proxy({}, {
    get(_, name) {
      if (typeof name !== "string") return undefined;
      const value = element.getAttribute(dataName(name));
      return value === null ? undefined : value;
    },
    set(_, name, value) { element.setAttribute(dataName(String(name)), String(value)); return true; },
    has(_, name) { return typeof name === "string" && element.hasAttribute(dataName(name)); },
    deleteProperty(_, name) { element.removeAttribute(dataName(String(name))); return true; },
    ownKeys: names,
    getOwnPropertyDescriptor(_, name) {
      const value = element.getAttribute(dataName(String(name)));
      return value === null ? undefined : { value, enumerable: true, configurable: true, writable: true };
    },
  });
  datasets.set(element, proxy);
  return proxy;
};

class DOMRect {
  constructor(x = 0, y = 0, width = 0, height = 0) {
    this.x = x; this.y = y; this.width = width; this.height = height;
  }
  get left() { return this.x; }
  get top() { return this.y; }
  get right() { return this.x + this.width; }
  get bottom() { return this.y + this.height; }
  toJSON() {
    return { x: this.x, y: this.y, width: this.width, height: this.height, top: this.top, left: this.left,
      right: this.right, bottom: this.bottom };
  }
}
const DOMRectReadOnly = DOMRect;

// ---- elements

class Attr {
  constructor(element, name, value) {
    this.ownerElement = element;
    this.name = this.localName = this.nodeName = name;
    this.value = this.nodeValue = value;
    this.specified = true;
  }
}
class NamedNodeMap extends Array {
  static get [Symbol.species]() { return Array; }
  item(i) { return this[i] || null; }
  getNamedItem(name) { return this.find((a) => a.name === name) || null; }
}

class Element extends Node {
  constructor(key, ref, tag, namespace = "") {
    super(key, ref);
    this[TAG] = tag;
    this[NS] = namespace;
  }
  get nodeType() { return ELEMENT_NODE; }
  get localName() { return this[TAG]; }
  get tagName() { return this[NS] ? this[TAG] : this[TAG].toUpperCase(); }
  get nodeName() { return this.tagName; }
  get namespaceURI() { return this[NS] ? namespaces[this[NS]] : HTML_NS; }
  get prefix() { return null; }

  // _named is an attribute's name as this element keeps it: in lower
  // case in HTML, as it is written elsewhere (viewBox).
  _named(name) { return this[NS] ? String(name) : String(name).toLowerCase(); }
  getAttribute(name) { return host("attr", this[REF], this._named(name)); }
  hasAttribute(name) { return this.getAttribute(name) !== null; }
  hasAttributes() { return host("attrs", this[REF]).length > 0; }
  getAttributeNames() { return host("attrs", this[REF]).map((a) => a[0]); }
  get attributes() {
    const map = new NamedNodeMap();
    for (const [name, value] of host("attrs", this[REF])) map.push(new Attr(this, name, value));
    return map;
  }
  getAttributeNode(name) { return this.attributes.getNamedItem(this._named(name)); }
  setAttribute(name, value) {
    name = this._named(name);
    if (!/^[^\s"'>/=]+$/.test(name)) throw new DOMException(`'${name}' is not a valid attribute name.`, "InvalidCharacterError");
    const oldValue = watching ? this.getAttribute(name) : null;
    host("setAttr", this[REF], name, String(value));
    if (name.startsWith("on")) forgetHandler(this, name.slice(2));
    if (watching) observed({ type: "attributes", target: this, attributeName: name, oldValue });
  }
  removeAttribute(name) {
    name = this._named(name);
    const oldValue = this.getAttribute(name);
    if (oldValue === null) return;
    host("removeAttr", this[REF], name);
    if (name.startsWith("on")) forgetHandler(this, name.slice(2));
    if (watching) observed({ type: "attributes", target: this, attributeName: name, oldValue });
  }
  toggleAttribute(name, force) {
    const has = this.hasAttribute(name);
    const on = force === undefined ? !has : Boolean(force);
    if (on && !has) this.setAttribute(name, "");
    if (!on && has) this.removeAttribute(name);
    return on;
  }
  getAttributeNS(_, name) { return this.getAttribute(name); }
  setAttributeNS(namespace, name, value) { this.setAttribute(namespace ? name : String(name).replace(/^.*:/, ""), value); }
  removeAttributeNS(_, name) { this.removeAttribute(name); }
  hasAttributeNS(_, name) { return this.hasAttribute(name); }

  get id() { return this.getAttribute("id") || ""; }
  set id(value) { this.setAttribute("id", value); }
  get className() { return this.getAttribute("class") || ""; }
  set className(value) { this.setAttribute("class", value); }
  get classList() {
    if (!this._classList) define(this, { _classList: new DOMTokenList(this, "class") });
    return this._classList;
  }
  set classList(value) { this.setAttribute("class", value); }
  get slot() { return this.getAttribute("slot") || ""; }

  get innerHTML() { return host("html", this[REF], false); }
  set innerHTML(markup) {
    const removed = watching ? this.childNodes : null;
    host("setHTML", this[REF], markup === null ? "" : String(markup));
    if (watching) observed({ type: "childList", target: this, removedNodes: removed, addedNodes: this.childNodes });
  }
  get outerHTML() { return host("html", this[REF], true); }
  set outerHTML(markup) {
    const parent = this.parentNode;
    if (!parent) return;
    const next = this.nextSibling;
    const nodes = wrapAll(host("fragment", String(markup), parent instanceof Element ? parent.localName : "div"));
    detach(this);
    for (const node of nodes) insert(parent, node, next);
  }
  insertAdjacentElement(where, element) {
    switch (String(where).toLowerCase()) {
      case "beforebegin": if (this.parentNode) insert(this.parentNode, element, this); break;
      case "afterbegin": insert(this, element, this.firstChild); break;
      case "beforeend": insert(this, element, null); break;
      case "afterend": if (this.parentNode) insert(this.parentNode, element, this.nextSibling); break;
      default: throw new DOMException(`The value provided ('${where}') is not one of 'beforeBegin', 'afterBegin', 'beforeEnd', or 'afterEnd'.`, "SyntaxError");
    }
    return element;
  }
  insertAdjacentHTML(where, markup) {
    const inside = /^(afterbegin|beforeend)$/i.test(where);
    const within = inside ? this : this.parentNode;
    const fragment = document.createDocumentFragment();
    for (const node of wrapAll(host("fragment", String(markup), within instanceof Element ? within.localName : "div"))) {
      fragment.appendChild(node);
    }
    this.insertAdjacentElement(where, fragment);
  }
  insertAdjacentText(where, text) { this.insertAdjacentElement(where, document.createTextNode(String(text))); }

  get nextElementSibling() { return wrap(host("sibling", this[REF], 1, true)); }
  get previousElementSibling() { return wrap(host("sibling", this[REF], -1, true)); }
  remove() { detach(this); }
  before(...items) { if (this.parentNode) insert(this.parentNode, nodesOf(items), this); }
  after(...items) { if (this.parentNode) insert(this.parentNode, nodesOf(items), this.nextSibling); }
  replaceWith(...items) {
    const parent = this.parentNode;
    if (!parent) return;
    const next = this.nextSibling;
    detach(this);
    if (items.length) insert(parent, nodesOf(items), next);
  }
  matches(selector) { return host("matches", this[REF], String(selector)); }
  webkitMatchesSelector(selector) { return this.matches(selector); }
  closest(selector) { return wrap(host("closest", this[REF], String(selector))); }
  getElementsByTagName(tag) {
    return HTMLCollection.from(host("query", this[REF], tag === "*" ? "*" : CSS.escape(String(tag).toLowerCase()), true).map(wrap));
  }
  getElementsByTagNameNS(_, tag) { return this.getElementsByTagName(tag); }
  getElementsByClassName(names) {
    const classes = String(names).split(/\s+/).filter(Boolean);
    if (!classes.length) return new HTMLCollection();
    return HTMLCollection.from(host("query", this[REF], classes.map((c) => "." + CSS.escape(c)).join(""), true).map(wrap));
  }

  getBoundingClientRect() { return new DOMRect(...host("box", this[REF])); }
  getClientRects() {
    const rect = this.getBoundingClientRect();
    return rect.width || rect.height ? [rect] : [];
  }
  get clientWidth() { return this === document.documentElement ? window.innerWidth : Math.round(host("box", this[REF])[2]); }
  get clientHeight() { return this === document.documentElement ? window.innerHeight : Math.round(host("box", this[REF])[3]); }
  get clientLeft() { return 0; }
  get clientTop() { return 0; }
  get scrollWidth() { return this === document.documentElement ? host("viewport")[4] : this.clientWidth; }
  get scrollHeight() { return this === document.documentElement ? host("viewport")[5] : this.clientHeight; }
  get scrollTop() { return scrolls(this) ? window.scrollY : 0; }
  set scrollTop(y) { if (scrolls(this)) window.scrollTo(window.scrollX, y); }
  get scrollLeft() { return scrolls(this) ? window.scrollX : 0; }
  set scrollLeft(x) { if (scrolls(this)) window.scrollTo(x, window.scrollY); }
  scrollIntoView() { host("reveal", this[REF]); }
  scrollIntoViewIfNeeded() { host("reveal", this[REF]); }
  scrollTo(...args) { if (scrolls(this)) window.scrollTo(...args); }
  scroll(...args) { this.scrollTo(...args); }
  scrollBy() { /* an element of a page does not scroll by script */ }
  attachShadow() { throw new DOMException("light has no shadow trees.", "NotSupportedError"); }
  get shadowRoot() { return null; }
  animate() {
    const done = Promise.resolve();
    return { finished: done, ready: done, cancel() {}, finish() {}, play() {}, pause() {}, addEventListener() {} };
  }
  getAnimations() { return []; }
  setPointerCapture() {}
  releasePointerCapture() {}
  hasPointerCapture() { return false; }
  requestFullscreen() { return Promise.reject(new TypeError("light has no fullscreen")); }
}
const scrolls = (element) => element === document.documentElement || element === document.body;
accessors(Element.prototype, parentMixin);
define(Element.prototype, parentMethods);

// reflect makes properties that are attributes: a string, a flag when
// the default is false, a number when it is one.
const reflect = (prototype, properties) => {
  for (const name of Object.keys(properties)) {
    const [attribute, fallback] = [].concat(properties[name]);
    const kind = typeof fallback;
    Object.defineProperty(prototype, name, {
      get() {
        const value = this.getAttribute(attribute);
        if (kind === "boolean") return value !== null;
        if (kind === "number") return value === null || isNaN(parseInt(value, 10)) ? fallback : parseInt(value, 10);
        return value === null ? (fallback || "") : value;
      },
      set(value) {
        if (kind === "boolean") this.toggleAttribute(attribute, Boolean(value));
        else this.setAttribute(attribute, value);
      },
      configurable: true, enumerable: true,
    });
  }
};
