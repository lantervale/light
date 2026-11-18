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
const ELEMENT_NODE = 1, TEXT_NODE = 3, CDATA_NODE = 4, INSTRUCTION_NODE = 7, COMMENT_NODE = 8, DOCUMENT_NODE = 9,
  DOCTYPE_NODE = 10, FRAGMENT_NODE = 11;
const FRAGMENT = "#document-fragment";
const HTML_NS = "http://www.w3.org/1999/xhtml";
const XML_NAMESPACE = "http://www.w3.org/XML/1998/namespace", XMLNS_NAMESPACE = "http://www.w3.org/2000/xmlns/";
// namespaces are those of the elements that are not HTML's, by the name
// light knows each by. An element of another namespace is known by the
// namespace itself, and one of none by "-".
const namespaces = { svg: "http://www.w3.org/2000/svg", math: "http://www.w3.org/1998/Math/MathML" };
const NO_NS = "-";
const namespaceOf = (uri) => Object.keys(namespaces).find((name) => namespaces[name] === uri) || "";
const spaceOf = (uri) => (uri === null || uri === undefined || uri === "" ? NO_NS
  : uri === HTML_NS ? "" : namespaceOf(String(uri)) || String(uri));
// The page's document is a node as any other: pageRef is its number,
// and pageInfo its type and whether it was read by XML's rules.
// strangers is whether a script made a document of its own, and
// xmlAbout whether any document is of XML: until then every node is
// the page's, and every name is HTML's.
const PAGE = Symbol("page");
const INFO = Symbol("info");
const pageRef = host("document", "page");
const pageInfo = host("document", "info", pageRef);
let strangers = false;
let xmlAbout = pageInfo[1];
// prefixed is whether a script made an element with a prefix; those a
// document of XML was written with are asked for while one is about.
let prefixed = false;
const inXML = (node) => xmlAbout && host("document", "xml", node[REF]);
// objectOf is the object of the node of a number, of that name.
const objectOf = (ref, tag) => {
  if (tag === FRAGMENT) return new DocumentFragment(token, ref);
  if (tag === "#comment") return new Comment(token, ref);
  if (tag === "#doctype") return new DocumentType(token, ref);
  if (tag === "#cdata-section") return new CDATASection(token, ref);
  if (tag === "#document:xml") return new XMLDocument(token, ref);
  if (tag === "#document:html") return new Document(token, ref);
  if (tag[0] === "?") return new ProcessingInstruction(token, ref, tag.slice(1));
  if (tag[0] === "{") {
    const end = tag.indexOf("}");
    return new Element(token, ref, tag.slice(end + 1), tag.slice(1, end) || NO_NS);
  }
  const colon = tag.indexOf(":");
  if (colon < 0) return new (classOf(tag))(token, ref, tag);
  if (colon === 0) return new HTMLUnknownElement(token, ref, tag.slice(1));
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
const wrapAll = (refs) => nodeList(refs.map(wrap));
// kidsOf are the children of a node now, foundIn the elements under it
// a selector matches: arrays, for light's own code.
const kidsOf = (node) => host("children", node[REF], false).map(wrap);
const foundIn = (node, selector) => host("query", node[PAGE] ? 0 : node[REF], selector, true).map(wrap);
const refOf = (node, what = "a node") => {
  if (!(node instanceof Node) || node[REF] === undefined) throw new TypeError(`${what} is not a Node`);
  return node[REF];
};

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
  get ownerDocument() { return (strangers && wrap(host("document", "owner", this[REF]))) || document; }
  get isConnected() { return host("connected", this[REF]); }
  get parentNode() {
    const parent = host("parent", this[REF]);
    return parent === "document" ? document : wrap(parent);
  }
  get parentElement() {
    const parent = this.parentNode;
    return parent instanceof Element ? parent : null;
  }
  get childNodes() { return keptList(this, "childNodes", () => liveList(NodeList, () => kidsOf(this))); }
  get firstChild() { return wrap(host("child", this[REF], 0, false)); }
  get lastChild() { return wrap(host("child", this[REF], -1, false)); }
  get nextSibling() { return wrap(host("sibling", this[REF], 1, false)); }
  get previousSibling() { return wrap(host("sibling", this[REF], -1, false)); }
  get textContent() { return host("text", this[REF]); }
  set textContent(text) {
    const removed = watching ? kidsOf(this) : null;
    host("setText", this[REF], text === null || text === undefined ? "" : String(text));
    if (watching) observed({ type: "childList", target: this, removedNodes: removed, addedNodes: kidsOf(this) });
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
  appendChild(child) { return insert(this, aNode(child), null); }
  insertBefore(child, before) {
    if (arguments.length < 2) throw new TypeError("insertBefore takes a node and the one it goes before.");
    return insert(this, aNode(child), before === undefined || before === null ? null : aNode(before));
  }
  removeChild(child) {
    if (aNode(child).parentNode !== this) {
      throw new DOMException("The node to be removed is not a child of this node.", "NotFoundError");
    }
    detach(child);
    return child;
  }
  replaceChild(child, old) {
    aNode(child);
    if (aNode(old).parentNode !== this) {
      throw new DOMException("The node to be replaced is not a child of this node.", "NotFoundError");
    }
    if (child === old) return old;
    insert(this, child, old, old);
    detach(old);
    return old;
  }
  cloneNode(deep) { return wrap(host("clone", this[REF], Boolean(deep))); }
  isSameNode(other) { return this === other; }
  isEqualNode(other) { return other instanceof Node && equalNodes(this, other); }
  compareDocumentPosition(other) {
    if (this === other) return 0;
    if (other instanceof Attr) {
      const from = other.compareDocumentPosition(this);
      return (from & 33) | (from & 2 ? 4 : 0) | (from & 4 ? 2 : 0) | (from & 8 ? 16 : 0) | (from & 16 ? 8 : 0);
    }
    if (!(other instanceof Node) || other[REF] === undefined) return 1;
    return host("compare", this[REF], other[REF]);
  }
  normalize() {
    for (const child of kidsOf(this)) {
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
  lookupNamespaceURI(prefix) { return namespaceFrom(this, prefix === "" || prefix === undefined ? null : prefix); }
  lookupPrefix(uri) { return uri === null || uri === undefined || uri === "" ? null : prefixFrom(this, String(uri)); }
  isDefaultNamespace(uri) {
    return namespaceFrom(this, null) === (uri === "" || uri === undefined ? null : uri);
  }
}
const nodeTypes = {
  ELEMENT_NODE, ATTRIBUTE_NODE: 2, TEXT_NODE, CDATA_SECTION_NODE: CDATA_NODE, ENTITY_REFERENCE_NODE: 5, ENTITY_NODE: 6,
  PROCESSING_INSTRUCTION_NODE: INSTRUCTION_NODE, COMMENT_NODE, DOCUMENT_NODE, DOCUMENT_TYPE_NODE: 10,
  DOCUMENT_FRAGMENT_NODE: FRAGMENT_NODE, NOTATION_NODE: 12,
  DOCUMENT_POSITION_DISCONNECTED: 1, DOCUMENT_POSITION_PRECEDING: 2, DOCUMENT_POSITION_FOLLOWING: 4,
  DOCUMENT_POSITION_CONTAINS: 8, DOCUMENT_POSITION_CONTAINED_BY: 16, DOCUMENT_POSITION_IMPLEMENTATION_SPECIFIC: 32,
};

// declaringFrom is the element a namespace is looked up from for a node
// that is none: a document's own, else the one the node is in.
const declaringFrom = (node) => (node instanceof Document ? node.documentElement
  : node instanceof DocumentType || node instanceof DocumentFragment ? null : node.parentElement);
// declaredOn are the attributes of an element that declare a
// namespace: those of the namespace of such declarations, and the
// xmlns HTML's parser leaves without one on an element that is not
// HTML's.
const declaredOn = (element) => attrRows(element).filter((row) => row[2] === XMLNS_NAMESPACE ||
  (element[NS] && row[2] === "" && row[0] === "xmlns"));
// namespaceFrom is the namespace a prefix stands for at a node, the
// default one for null, as the elements from it up declare them.
const namespaceFrom = (node, prefix) => {
  if (!(node instanceof Element)) {
    const from = declaringFrom(node);
    return from ? namespaceFrom(from, prefix) : null;
  }
  if (prefix === "xml") return XML_NAMESPACE;
  if (prefix === "xmlns") return XMLNS_NAMESPACE;
  for (let at = node; at; at = at.parentElement) {
    if (at.namespaceURI !== null && at.prefix === prefix) return at.namespaceURI;
    const declared = declaredOn(at).find((row) => row[4] === (prefix === null ? "xmlns" : prefix) &&
      (row[3] === "") === (prefix === null));
    if (declared) return declared[1] || null;
  }
  return null;
};
// prefixFrom is a prefix the elements from a node up declare for a
// namespace, null for none.
const prefixFrom = (node, uri) => {
  if (!(node instanceof Element)) {
    const from = declaringFrom(node);
    return from ? prefixFrom(from, uri) : null;
  }
  for (let at = node; at; at = at.parentElement) {
    if (at.namespaceURI === uri && at.prefix !== null) return at.prefix;
    const declared = declaredOn(at).find((row) => row[3] === "xmlns" && row[1] === uri);
    if (declared) return declared[4];
  }
  return null;
};
define(Node, nodeTypes);
define(Node.prototype, nodeTypes);

const refuse = (why) => { throw new DOMException(why, "HierarchyRequestError"); };
// admit refuses what a document cannot hold: more than one element,
// more than one doctype, or a doctype after its element. replaced is
// the child the new one takes the place of.
const admit = (parent, child, before, replaced) => {
  const kids = kidsOf(parent);
  const at = before ? kids.indexOf(before) : kids.length;
  const has = (from, kind) => kids.slice(...from).some((n) => n !== replaced && n instanceof kind);
  const parts = child instanceof DocumentFragment ? kidsOf(child) : [child];
  const elements = parts.filter((n) => n instanceof Element).length;
  if (elements > 1 || parts.some((n) => n instanceof Text)) refuse("A document holds one element, and no text.");
  if (elements && (has([0], Element) || has([at], DocumentType))) {
    refuse("Only one element on document allowed.");
  }
  if (child instanceof DocumentType && (has([0], DocumentType) || has([0, at], Element))) {
    refuse("Only one doctype on document allowed, before its element.");
  }
};
// insert puts child in parent before another of its children, at its
// end for none: a fragment gives its children in its place.
const insert = (parent, child, before, replaced = null) => {
  if (!(parent instanceof Element || parent instanceof Document || parent instanceof DocumentFragment)) {
    refuse("This node type does not support this method.");
  }
  const into = refOf(parent);
  if (child instanceof Attr) refuse("An attribute is held by its element only.");
  const ref = refOf(child, "the child");
  if (child === parent || host("contains", ref, into)) refuse("The new child element contains the parent.");
  if (before === child && replaced === null) before = child.nextSibling;
  if (before !== null && (!(before instanceof Node) || before.parentNode !== parent)) {
    throw new DOMException("The node before which the new node is to be inserted is not a child of this node.",
      "NotFoundError");
  }
  if (child instanceof Document) refuse("Nodes of type '#document' may not be inserted inside nodes of type '" + parent.nodeName + "'.");
  if (parent instanceof Document) admit(parent, child, before, replaced);
  else if (child instanceof DocumentType) refuse("A doctype is held by a document only.");
  if (child instanceof DocumentFragment) {
    for (const one of kidsOf(child)) insert(parent, one, before);
    return child;
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
  if (!parent || (parent === document && child instanceof Element)) return;
  const previousSibling = watching ? child.previousSibling : null, nextSibling = watching ? child.nextSibling : null;
  host("remove", child[REF]);
  if (watching) observed({ type: "childList", target: parent, addedNodes: [], removedNodes: [child], previousSibling, nextSibling });
};
// nodesOf turns what append and its kin take, nodes and strings, into
// one node.
// aNode is what a script gave where a node is taken.
const aNode = (given) => {
  if (!(given instanceof Node)) throw new TypeError("The value given is not a Node.");
  return given;
};
// beside is the nearest sibling of a node, one way, that is not among
// the nodes about to be moved.
const beside = (node, items, next) => {
  let at = next ? node.nextSibling : node.previousSibling;
  while (at && items.includes(at)) at = next ? at.nextSibling : at.previousSibling;
  return at;
};
const putBefore = (node, items) => {
  const parent = node.parentNode;
  if (!parent) return;
  const kept = beside(node, items, false);
  const nodes = nodesOf(items);
  insert(parent, nodes, kept ? kept.nextSibling : parent.firstChild);
};
const putAfter = (node, items) => {
  const parent = node.parentNode;
  if (!parent) return;
  const kept = beside(node, items, true);
  insert(parent, nodesOf(items), kept);
};
const putInPlaceOf = (node, items) => {
  const parent = node.parentNode;
  if (!parent) return;
  const kept = beside(node, items, true);
  const nodes = items.length ? nodesOf(items) : null;
  if (node.parentNode === parent) detach(node);
  if (nodes) insert(parent, nodes, kept);
};
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
  get childNodes() { return keptList(this, "childNodes", () => nodeList([])); }
  get firstChild() { return null; }
  get lastChild() { return null; }
  hasChildNodes() { return false; }
  get nextElementSibling() { return wrap(host("sibling", this[REF], 1, true)); }
  get previousElementSibling() { return wrap(host("sibling", this[REF], -1, true)); }
  appendData(text) {
    if (!arguments.length) throw new TypeError("appendData takes the text to add.");
    this.data += text;
  }
  substringData(at, count) { return this.data.substr(offsetIn(this, at), count >>> 0); }
  insertData(at, text) { this.replaceData(at, 0, text); }
  deleteData(at, count) { this.replaceData(at, count, ""); }
  replaceData(at, count, text) {
    const data = this.data;
    at = offsetIn(this, at);
    this.data = data.slice(0, at) + String(text) + data.slice(at + (count >>> 0));
  }
  remove() { detach(this); }
  before(...items) { putBefore(this, items); }
  after(...items) { putAfter(this, items); }
  replaceWith(...items) { putInPlaceOf(this, items); }
}
// offsetIn is an offset into a node's text, refused past its end.
const offsetIn = (node, at) => {
  at >>>= 0;
  if (at > node.length) throw new DOMException("The offset is past the end of the text.", "IndexSizeError");
  return at;
};
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
  get publicId() { return host("attr", this[REF], "public") || ""; }
  get systemId() { return host("attr", this[REF], "system") || ""; }
  get textContent() { return null; }
  set textContent(_) {}
  get childNodes() { return keptList(this, "childNodes", () => nodeList([])); }
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
  children: {
    get() {
      return keptList(this, "children", () => liveList(HTMLCollection, () => host("children", this[REF], true).map(wrap)));
    },
  },
  firstElementChild: { get() { return wrap(host("child", this[REF], 0, true)); } },
  lastElementChild: { get() { return wrap(host("child", this[REF], -1, true)); } },
  childElementCount: { get() { return host("children", this[REF], true).length; } },
};
const parentMethods = {
  append(...items) { if (items.length) insert(this, nodesOf(items), null); },
  prepend(...items) { if (items.length) insert(this, nodesOf(items), this.firstChild); },
  replaceChildren(...items) {
    for (const child of kidsOf(this)) detach(child);
    if (items.length) insert(this, nodesOf(items), null);
  },
  querySelector(selector) { return wrap(host("query", this[REF], String(selector), false)); },
  querySelectorAll(selector) { return wrapAll(host("query", this[REF], String(selector), true)); },
};
accessors(DocumentFragment.prototype, parentMixin);
define(DocumentFragment.prototype, parentMethods);

// ---- tokens, styles, data

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
    get map() { return declarations(ownAttr(element, "style") || ""); },
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

class Element extends Node {
  constructor(key, ref, tag, namespace = "") {
    super(key, ref);
    this[TAG] = tag;
    this[NS] = namespace;
  }
  get nodeType() { return ELEMENT_NODE; }
  get localName() { return this[TAG]; }
  get tagName() {
    const prefix = this.prefix;
    const name = prefix ? prefix + ":" + this[TAG] : this[TAG];
    return this[NS] || inXML(this) ? name : name.replace(/[a-z]+/g, (run) => run.toUpperCase());
  }
  get nodeName() { return this.tagName; }
  get namespaceURI() {
    const space = this[NS];
    return space === "" ? HTML_NS : space === NO_NS ? null : namespaces[space] || space;
  }
  get prefix() { return ((prefixed || xmlAbout) && host("document", "prefix", this[REF])) || null; }

  // _named is an attribute's name as this element keeps it: in lower
  // case in HTML, as it is written elsewhere (viewBox) and in a
  // document of XML.
  _named(name) { return this[NS] || inXML(this) ? String(name) : asciiLower(String(name)); }
  getAttribute(name) { return host("attr", this[REF], this._named(name)); }
  hasAttribute(name) { return this.getAttribute(name) !== null; }
  hasAttributes() { return host("attrs", this[REF]).length > 0; }
  getAttributeNames() { return host("attrs", this[REF]).map((a) => a[0]); }
  setAttribute(name, value) {
    name = this._named(name);
    if (!/^[^\t\n\f\r \0/=>]+$/.test(name)) invalid("attribute name", name);
    const oldValue = watching ? this.getAttribute(name) : null;
    host("setAttr", this[REF], name, String(value));
    if (name.startsWith("on")) forgetHandler(this, name.slice(2));
    if (name === "nonce") nonces.delete(this);
    if (watching) observed({ type: "attributes", target: this, attributeName: name, oldValue });
  }
  toggleAttribute(name, force) {
    if (!/^[^\t\n\f\r \0/=>]+$/.test(name)) invalid("attribute name", name);
    const has = this.hasAttribute(name);
    const on = force === undefined ? !has : Boolean(force);
    if (on && !has) this.setAttribute(name, "");
    if (!on && has) this.removeAttribute(name);
    return on;
  }

  get id() { return ownAttr(this, "id") || ""; }
  set id(value) { this.setAttribute("id", value); }
  get className() { return ownAttr(this, "class") || ""; }
  set className(value) { this.setAttribute("class", value); }

  get innerHTML() { return host("html", this[REF], false); }
  set innerHTML(markup) {
    const removed = watching ? kidsOf(this) : null;
    host("setHTML", this[REF], markup === null ? "" : String(markup));
    if (watching) observed({ type: "childList", target: this, removedNodes: removed, addedNodes: kidsOf(this) });
  }
  get outerHTML() { return host("html", this[REF], true); }
  set outerHTML(markup) {
    const parent = this.parentNode;
    if (!parent) return;
    const next = this.nextSibling;
    const within = parent instanceof Element ? parent : null;
    const nodes = wrapAll(host("fragment", String(markup), within ? within.localName : "div", within ? within[REF] : 0));
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
    const element = within instanceof Element;
    for (const node of wrapAll(host("fragment", String(markup), element ? within.localName : "div", element ? within[REF] : 0))) {
      fragment.appendChild(node);
    }
    this.insertAdjacentElement(where, fragment);
  }
  insertAdjacentText(where, text) { this.insertAdjacentElement(where, document.createTextNode(String(text))); }

  get nextElementSibling() { return wrap(host("sibling", this[REF], 1, true)); }
  get previousElementSibling() { return wrap(host("sibling", this[REF], -1, true)); }
  remove() { detach(this); }
  before(...items) { putBefore(this, items); }
  after(...items) { putAfter(this, items); }
  replaceWith(...items) { putInPlaceOf(this, items); }
  matches(selector) { return host("matches", this[REF], String(selector)); }
  webkitMatchesSelector(selector) { return this.matches(selector); }
  closest(selector) { return wrap(host("closest", this[REF], String(selector))); }
  getElementsByTagName(tag) { return tagged(this, tag); }
  getElementsByTagNameNS(uri, tag) { return tagged(this, tag, uri === "*" ? "*" : spaceOf(uri)); }
  getElementsByClassName(names) {
    return classed(this, names);
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
// tagged are the elements under a node of a name, as it is written in
// a document of XML and in lower case for HTML's, or of a local name
// in a namespace.
const tagged = (root, name, space = null) => {
  name = String(name);
  return liveList(HTMLCollection, () => host("document", "tagged", root[REF], name, space).map(wrap));
};
// classed are the elements under root of every class of a list of
// them.
const classed = (root, names) => {
  const classes = String(names).split(/[\t\n\f\r ]+/).filter(Boolean);
  if (!classes.length) return liveList(HTMLCollection, () => []);
  const selector = classes.map((c) => "." + CSS.escape(c)).join("");
  return liveList(HTMLCollection, () => foundIn(root, selector));
};
accessors(Element.prototype, parentMixin);
define(Element.prototype, parentMethods);
