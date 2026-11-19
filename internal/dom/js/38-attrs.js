// ---- attributes

// An attribute is a node of its own: the same one as long as its
// element has it, which says its element's value and sets it, and one
// that keeps its own once it is taken off or before it is put on. Its
// element keeps the value; these keep which node stands for which.
const attrStates = new WeakMap();
const attrsKept = new WeakMap();
const attrMaps = new WeakMap();
const attrKey = (space, local) => space + "\n" + local;
const attrState = (attr) => attrStates.get(attr) || illegalInvocation();
// The rows of an element's attributes are each one's name as written,
// its value, its namespace, its prefix and its local name.
const attrRows = (element) => host("attribute", element[REF], "list");
const attrRow = (element, space, local) => attrRows(element).find((row) => row[2] === space && row[4] === local);
const spaceGiven = (namespace) => (namespace === null || namespace === undefined ? "" : String(namespace));

class Attr extends Node {
  constructor(key, owner, namespace, prefix, local, value = "") {
    super(key, undefined);
    attrStates.set(this, { element: null, owner, space: namespace, prefix, local, value });
  }
  get nodeType() { return 2; }
  get namespaceURI() { return attrState(this).space || null; }
  get prefix() { return attrState(this).prefix || null; }
  get localName() { return attrState(this).local; }
  get name() {
    const { prefix, local } = attrState(this);
    return prefix ? prefix + ":" + local : local;
  }
  get nodeName() { return this.name; }
  get value() {
    const state = attrState(this);
    if (!state.element) return state.value;
    const row = attrRow(state.element, state.space, state.local);
    return row ? row[1] : "";
  }
  set value(value) {
    const state = attrState(this);
    if (state.element) putAttr(state.element, state.space, state.prefix, state.local, String(value));
    else state.value = String(value);
  }
  get nodeValue() { return this.value; }
  set nodeValue(value) { this.value = value === null ? "" : value; }
  get textContent() { return this.value; }
  set textContent(value) { this.value = value === null ? "" : value; }
  get specified() { return true; }
  get ownerElement() { return attrState(this).element; }
  get ownerDocument() {
    const { element, owner } = attrState(this);
    return element ? element.ownerDocument : owner;
  }
  get isConnected() { return false; }
  get parentNode() { return null; }
  get childNodes() { return keptList(this, "childNodes", () => nodeList([])); }
  get firstChild() { return null; }
  get lastChild() { return null; }
  get nextSibling() { return null; }
  get previousSibling() { return null; }
  hasChildNodes() { return false; }
  contains(other) { return other === this; }
  cloneNode() {
    const { space, prefix, local } = attrState(this);
    return new Attr(token, this.ownerDocument, space, prefix, local, this.value);
  }
  isEqualNode(other) {
    return other instanceof Attr && other.namespaceURI === this.namespaceURI && other.localName === this.localName &&
      other.value === this.value;
  }
  // An attribute stands after its element and before what its element
  // holds, among its element's others in their order.
  compareDocumentPosition(other) {
    if (other === this) return 0;
    const mine = this.ownerElement, theirs = other instanceof Attr ? other.ownerElement : other;
    if (!mine || !(theirs instanceof Node)) return 35;
    if (mine !== theirs) {
      const between = mine.compareDocumentPosition(theirs);
      return between & 16 ? 4 : between;
    }
    if (!(other instanceof Attr)) return 10;
    const all = attrsIn(mine);
    return 32 + (all.indexOf(other) < all.indexOf(this) ? 2 : 4);
  }
  lookupNamespaceURI(prefix) { return this.ownerElement ? this.ownerElement.lookupNamespaceURI(prefix) : null; }
  lookupPrefix(namespace) { return this.ownerElement ? this.ownerElement.lookupPrefix(namespace) : null; }
  isDefaultNamespace(namespace) { return this.ownerElement ? this.ownerElement.isDefaultNamespace(namespace) : !namespace; }
}

// attrsIn are the attributes of an element as nodes, in the order it
// has them.
const attrsIn = (element, rows = attrRows(element)) => {
  let kept = attrsKept.get(element);
  if (!kept) {
    if (!rows.length) return [];
    attrsKept.set(element, kept = new Map());
  }
  const nodes = rows.map(([, , space, prefix, local]) => {
    const key = attrKey(space, local);
    if (!kept.has(key)) {
      const attr = new Attr(token, null, space, prefix, local);
      attrStates.get(attr).element = element;
      kept.set(key, attr);
    }
    return kept.get(key);
  });
  if (kept.size > nodes.length) {
    for (const [key, attr] of kept) {
      if (!nodes.includes(attr)) letGo(element, key, "");
    }
  }
  return nodes;
};
// letGo has the node of an attribute its element lost keep the value
// it had.
const letGo = (element, key, value) => {
  const kept = attrsKept.get(element);
  const attr = kept && kept.get(key);
  if (!attr) return;
  kept.delete(key);
  Object.assign(attrStates.get(attr), { element: null, owner: element.ownerDocument, value });
};
const attrChanged = (element, space, local, oldValue) => {
  if (!space && local.startsWith("on")) forgetHandler(element, local.slice(2));
  if (!space && local === "nonce") nonces.delete(element);
  if (watching) {
    observed({ type: "attributes", target: element, attributeName: local, attributeNamespace: space || null, oldValue });
  }
};
const putAttr = (element, space, prefix, local, value) => {
  const old = watching ? attrRow(element, space, local) : undefined;
  host("attribute", element[REF], "set", space, prefix || "", local, value);
  attrChanged(element, space, local, old ? old[1] : null);
};
const dropAttr = (element, [, value, space, , local]) => {
  host("attribute", element[REF], "remove", space, local);
  letGo(element, attrKey(space, local), value);
  attrChanged(element, space, local, value);
};
// attrNamed is the attribute of an element a script names as it is
// written, in lower case on an element of HTML in a document of HTML.
const attrNamed = (element, name) => {
  name = element._named(name);
  const rows = attrRows(element);
  const at = rows.findIndex((row) => row[0] === name);
  return at < 0 ? null : attrsIn(element, rows)[at];
};
const attrIn = (element, namespace, local) => {
  const space = spaceGiven(namespace), rows = attrRows(element);
  const at = rows.findIndex((row) => row[2] === space && row[4] === String(local));
  return at < 0 ? null : attrsIn(element, rows)[at];
};
// attrPut puts a node on an element in the place of the one it has of
// that name, which it gives back.
const attrPut = (element, attr) => {
  if (!(attr instanceof Attr)) throw new TypeError("The attribute provided is not of type 'Attr'.");
  const state = attrStates.get(attr);
  if (state.element === element) return attr;
  if (state.element) {
    throw new DOMException("The node provided is an attribute node that is already an attribute of another Element; " +
      "attribute nodes must be explicitly cloned.", "InUseAttributeError");
  }
  const old = attrIn(element, state.space, state.local);
  const key = attrKey(state.space, state.local), oldValue = old ? old.value : null;
  if (old) letGo(element, key, oldValue);
  if (!attrsKept.has(element)) attrsKept.set(element, new Map());
  attrsKept.get(element).set(key, attr);
  state.element = element;
  host("attribute", element[REF], "replace", state.space, state.prefix, state.local, state.value);
  attrChanged(element, state.space, state.local, oldValue);
  return old;
};
const attrTaken = (element, attr, row) => {
  if (!row) throw new DOMException("No item with the name provided was found.", "NotFoundError");
  dropAttr(element, row);
  return attr;
};

const mapIndex = (key) => (typeof key === "string" && /^(0|[1-9]\d*)$/.test(key) ? Number(key) : -1);
// The names a map answers to are those its attributes are written
// with, but for one with a capital on an element of HTML in a document
// of HTML, which no script could ask for.
const mapNames = (element) => {
  const names = [...new Set(attrRows(element).map((row) => row[0]))];
  return element[NS] || inXML(element) ? names : names.filter((name) => !/[A-Z]/.test(name));
};
const mapNamed = (element, key) => (typeof key === "string" && mapNames(element).includes(key)
  ? attrNamed(element, key) : undefined);
const namedAttrs = {
  get(target, key, receiver) {
    const element = attrMaps.get(target), at = mapIndex(key);
    if (at >= 0) return attrsIn(element)[at];
    return key in target ? Reflect.get(target, key, receiver) : mapNamed(element, key);
  },
  has(target, key) {
    const element = attrMaps.get(target), at = mapIndex(key);
    return at >= 0 ? at < attrRows(element).length : key in target || mapNamed(element, key) !== undefined;
  },
  set(target, key, value, receiver) { return mapIndex(key) < 0 && Reflect.set(target, key, value, receiver); },
  ownKeys(target) {
    const element = attrMaps.get(target);
    return attrRows(element).map((_, at) => String(at)).concat(mapNames(element), Reflect.ownKeys(target));
  },
  getOwnPropertyDescriptor(target, key) {
    const element = attrMaps.get(target), at = mapIndex(key);
    const value = at >= 0 ? attrsIn(element)[at] : Object.hasOwn(target, key) ? undefined : mapNamed(element, key);
    if (value === undefined) return Reflect.getOwnPropertyDescriptor(target, key);
    return { value, writable: false, enumerable: at >= 0, configurable: true };
  },
};

class NamedNodeMap {
  constructor(key, element) {
    if (key !== token) illegal();
    const map = new Proxy(this, namedAttrs);
    attrMaps.set(this, element);
    attrMaps.set(map, element);
    return map;
  }
  get length() { return attrRows(attrMaps.get(this)).length; }
  item(at) { return attrsIn(attrMaps.get(this))[Number(at) >>> 0] || null; }
  getNamedItem(name) { return attrNamed(attrMaps.get(this), name); }
  getNamedItemNS(namespace, local) { return attrIn(attrMaps.get(this), namespace || "", local); }
  setNamedItem(attr) { return attrPut(attrMaps.get(this), attr); }
  setNamedItemNS(attr) { return attrPut(attrMaps.get(this), attr); }
  removeNamedItem(name) {
    const element = attrMaps.get(this), attr = attrNamed(element, name);
    return attrTaken(element, attr, attr && attrRow(element, spaceGiven(attr.namespaceURI), attr.localName));
  }
  removeNamedItemNS(namespace, local) {
    const element = attrMaps.get(this), attr = attrIn(element, namespace || "", local);
    return attrTaken(element, attr, attr && attrRow(element, spaceGiven(attr.namespaceURI), attr.localName));
  }
  get [Symbol.toStringTag]() { return "NamedNodeMap"; }
}
Object.defineProperty(NamedNodeMap.prototype, Symbol.iterator, {
  value: Array.prototype.values, writable: true, configurable: true,
});

accessors(Element.prototype, {
  attributes: {
    get() {
      if (!attrMaps.has(this)) attrMaps.set(this, new NamedNodeMap(token, this));
      return attrMaps.get(this);
    },
  },
});
define(Element.prototype, {
  removeAttribute(name) {
    name = this._named(name);
    const row = attrRows(this).find((one) => one[0] === name);
    if (row) dropAttr(this, row);
  },
  getAttributeNS(namespace, local) {
    const row = attrRow(this, namespace || "", String(local));
    return row ? row[1] : null;
  },
  hasAttributeNS(namespace, local) { return attrRow(this, namespace || "", String(local)) !== undefined; },
  setAttributeNS(namespace, name, value) {
    const [space, prefix, local] = namedIn(namespace, name);
    putAttr(this, space || "", prefix, local, String(value));
  },
  removeAttributeNS(namespace, local) {
    const row = attrRow(this, namespace || "", String(local));
    if (row) dropAttr(this, row);
  },
  getAttributeNode(name) { return attrNamed(this, name); },
  getAttributeNodeNS(namespace, local) { return attrIn(this, namespace || "", local); },
  setAttributeNode(attr) { return attrPut(this, attr); },
  setAttributeNodeNS(attr) { return attrPut(this, attr); },
  removeAttributeNode(attr) {
    const state = attr instanceof Attr ? attrStates.get(attr) : null;
    if (!state) throw new TypeError("The attribute provided is not of type 'Attr'.");
    return attrTaken(this, attr, state.element === this ? attrRow(this, state.space, state.local) : undefined);
  },
});
define(Document.prototype, {
  createAttribute(name) {
    name = String(name);
    if (name === "" || /[\t\n\f\r \0/=>]/.test(name)) invalid("attribute name", name);
    return new Attr(token, this, "", "", infoOf(this)[1] ? name : asciiLower(name));
  },
  createAttributeNS(namespace, name) {
    const [space, prefix, local] = namedIn(namespace, name);
    return new Attr(token, this, space || "", prefix || "", local);
  },
});
// attrAdopted is an attribute taken into a document, off its element.
const attrAdopted = (owner, attr) => {
  const state = attrStates.get(attr);
  if (state.element) state.element.removeAttributeNode(attr);
  state.owner = owner;
  return attr;
};

// Two nodes are equal when they are of one kind, say the same of
// themselves, and hold equal nodes in the same order: an element its
// namespace, prefix and name, and the same attributes in any order.
const equalNodes = (a, b) => {
  if (a.nodeType !== b.nodeType) return false;
  if (a instanceof Attr) return a.isEqualNode(b);
  if (a instanceof DocumentType) {
    if (a.name !== b.name || a.publicId !== b.publicId || a.systemId !== b.systemId) return false;
  } else if (a instanceof Element) {
    if (a.namespaceURI !== b.namespaceURI || a.prefix !== b.prefix || a.localName !== b.localName) return false;
    const mine = attrRows(a), theirs = attrRows(b);
    if (mine.length !== theirs.length || !mine.every((row) => theirs.some((other) => other[2] === row[2] &&
      other[4] === row[4] && other[1] === row[1]))) return false;
  } else if (a instanceof ProcessingInstruction) {
    if (a.target !== b.target || a.data !== b.data) return false;
  } else if (a instanceof CharacterData && a.data !== b.data) {
    return false;
  }
  const kids = kidsOf(a), others = kidsOf(b);
  return kids.length === others.length && kids.every((kid, at) => equalNodes(kid, others[at]));
};
