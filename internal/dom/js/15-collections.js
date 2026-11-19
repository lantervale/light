// ---- the lists of nodes: NodeList, HTMLCollection and their kin

// A list of nodes is no array: it answers by index, an HTMLCollection
// by the ids and names of its elements too, and a live one is read
// again from the tree once the tree has changed. bag.version counts
// those changes (10-core.js), for all the documents of a kin: every question to light that is not a
// plain read is one, and so is the end of the task a list was read in,
// since light changes the tree between tasks without a word.

// ownAttr is the attribute of an element that has no namespace, the
// one its properties stand for, null for none.
const ownAttr = (element, name) => host("attribute", element[REF], "own", "", name);

let treeWatched = false;
const afterTask = Promise.prototype.then.bind(Promise.resolve());
const treeRead = () => {
  if (treeWatched) return;
  treeWatched = true;
  afterTask(() => {
    treeWatched = false;
    bag.version++;
  });
};

const nodeLists = new WeakMap();
const listState = (list) => {
  const state = nodeLists.get(list);
  if (!state) throw new TypeError("Illegal invocation");
  return state;
};
// itemsOf are the nodes of a list now, as an array light's own code
// reads and never changes.
const itemsOf = (list) => {
  const state = listState(list);
  if (state.read && state.seen !== bag.version) {
    state.nodes = state.read();
    state.names = null;
    state.seen = bag.version;
    treeRead();
  }
  return state.nodes;
};
// listIndex is the index a property name is, -1 for one that is none.
const listIndex = (key) => {
  if (typeof key !== "string") return -1;
  const first = key.charCodeAt(0);
  if (!(first >= 48 && first <= 57)) return -1;
  const i = Number(key);
  return Number.isInteger(i) && i < 4294967295 && String(i) === key ? i : -1;
};
// itemNames are the names an HTMLCollection gives its elements, each
// with the first element of it: an id, then a name where the element
// is HTML's.
const itemNames = (state) => {
  if (state.names && !state.read) return state.names;
  const nodes = itemsOf(state.list);
  if (state.names) return state.names;
  const names = new Map();
  for (const node of nodes) {
    for (const name of state.kind.names(node)) {
      if (name && !names.has(name)) names.set(name, node);
    }
  }
  state.names = names;
  return names;
};
const plainNames = (node) => [ownAttr(node, "id"), node instanceof HTMLElement ? ownAttr(node, "name") : null];
const itemNamed = (state, key) => {
  if (!state.kind.names || typeof key !== "string" || key === "") return undefined;
  if (state.kind.named) return state.kind.named(state, key);
  return itemNames(state).get(key);
};
// A name of the list's own, or of what it inherits, hides an element
// of that name.
const namedShown = (target, state, key) => (key in target ? undefined : itemNamed(state, key));

const listHandler = {
  get(target, key, receiver) {
    const i = listIndex(key);
    if (i >= 0) return itemsOf(target)[i];
    if (typeof key !== "string" || key in target) return Reflect.get(target, key, receiver);
    return itemNamed(nodeLists.get(target), key);
  },
  has(target, key) {
    const i = listIndex(key);
    if (i >= 0) return i < itemsOf(target).length;
    return key in target || itemNamed(nodeLists.get(target), key) !== undefined;
  },
  ownKeys(target) {
    const state = nodeLists.get(target);
    const keys = itemsOf(target).map((_, i) => String(i));
    if (state.kind.names) for (const name of itemNames(state).keys()) if (!(name in target)) keys.push(name);
    return keys.concat(Reflect.ownKeys(target));
  },
  getOwnPropertyDescriptor(target, key) {
    const state = nodeLists.get(target);
    const i = listIndex(key);
    if (i >= 0) {
      const nodes = itemsOf(target);
      if (i >= nodes.length) return undefined;
      return { value: nodes[i], writable: Boolean(state.kind.put), enumerable: true, configurable: true };
    }
    const named = namedShown(target, state, key);
    if (named !== undefined) return { value: named, writable: false, enumerable: false, configurable: true };
    return Reflect.getOwnPropertyDescriptor(target, key);
  },
  set(target, key, value, receiver) {
    const state = nodeLists.get(target);
    const i = listIndex(key);
    // An object that only inherits from the list sets its own
    // property, but for an index the list has.
    if (receiver !== state.list) {
      return !(i >= 0 && i < itemsOf(target).length) && Reflect.set(target, key, value, receiver);
    }
    if (i >= 0) {
      if (!state.kind.put) return false;
      state.kind.put(state, i, value);
      return true;
    }
    if (namedShown(target, state, key) !== undefined) return false;
    return Reflect.set(target, key, value, receiver);
  },
  defineProperty(target, key, descriptor) {
    const state = nodeLists.get(target);
    if (listIndex(key) >= 0 || namedShown(target, state, key) !== undefined) return false;
    return Reflect.defineProperty(target, key, descriptor);
  },
  deleteProperty(target, key) {
    const state = nodeLists.get(target);
    const i = listIndex(key);
    if (i >= 0) return i >= itemsOf(target).length;
    if (namedShown(target, state, key) !== undefined) return false;
    return Reflect.deleteProperty(target, key);
  },
};

// listOfKind makes a list of a class: of the nodes given, or live, of
// those read gives each time the tree has changed.
const listOfKind = (Class, nodes, read = null, more = null) => {
  const target = Object.create(Class.prototype);
  const list = new Proxy(target, listHandler);
  const state = { list, kind: Class.kind, nodes, read, seen: -1, names: null, ...more };
  nodeLists.set(target, state);
  nodeLists.set(list, state);
  return list;
};
const nodeList = (nodes) => listOfKind(NodeList, nodes);
const liveList = (Class, read, more) => listOfKind(Class, [], read, more);
// keptList is the one live list a node gives under a name, made the
// first time it is asked for.
const keptLists = new WeakMap();
const keptList = (node, name, make) => {
  let kept = keptLists.get(node);
  if (!kept) keptLists.set(node, kept = {});
  return kept[name] || (kept[name] = make());
};

// A list is walked as it is at each step, so one that grows meanwhile
// is walked to its new end.
const listWalk = (list, values) => {
  listState(list);
  return Array.prototype[values].call(list);
};
class NodeList {
  constructor() { illegal(); }
  get length() { return itemsOf(this).length; }
  item(i) { return itemsOf(this)[i >>> 0] || null; }
  forEach(callback, thisArg) {
    if (typeof callback !== "function") throw new TypeError("forEach takes a function.");
    for (let i = 0; i < itemsOf(this).length; i++) callback.call(thisArg, itemsOf(this)[i], i, this);
  }
  keys() { return listWalk(this, "keys"); }
  values() { return listWalk(this, "values"); }
  entries() { return listWalk(this, "entries"); }
}
NodeList.kind = {};
Object.defineProperty(NodeList.prototype, Symbol.iterator,
  { value: NodeList.prototype.values, writable: true, configurable: true });

class HTMLCollection {
  constructor() { illegal(); }
  get length() { return itemsOf(this).length; }
  item(i) { return itemsOf(this)[i >>> 0] || null; }
  namedItem(name) {
    const found = itemNamed(listState(this), String(name));
    return found === undefined ? null : found;
  }
  [Symbol.iterator]() { return listWalk(this, "values"); }
}
HTMLCollection.kind = { names: plainNames };

// Every element of a document is named in it by its id, and by its
// name where it is of a kind that had one.
const namedInAll = new Set(["a", "button", "embed", "form", "frame", "frameset", "iframe", "img", "input", "map", "meta",
  "object", "select", "textarea"]);
class HTMLAllCollection extends HTMLCollection {}
HTMLAllCollection.kind = {
  names: (node) => [ownAttr(node, "id"),
    node instanceof HTMLElement && namedInAll.has(node[TAG]) ? ownAttr(node, "name") : null],
};

class RadioNodeList extends NodeList {
  get value() {
    const on = itemsOf(this).find((n) => n.type === "radio" && n.checked);
    if (!on) return "";
    return on.hasAttribute("value") ? on.value : "on";
  }
  set value(value) {
    value = String(value);
    const one = itemsOf(this).find((n) => n.type === "radio" &&
      (n.hasAttribute("value") ? n.value === value : value === "on"));
    if (one) one.checked = true;
  }
}
RadioNodeList.kind = {};
// A form's controls of one name are a list of them, live as the form
// is.
class HTMLFormControlsCollection extends HTMLCollection {}
HTMLFormControlsCollection.kind = {
  names: (node) => [ownAttr(node, "id"), ownAttr(node, "name")],
  named(state, key) {
    const called = (n) => ownAttr(n, "id") === key || ownAttr(n, "name") === key;
    const all = itemsOf(state.list).filter(called);
    if (all.length < 2) return all[0];
    return liveList(RadioNodeList, () => itemsOf(state.list).filter(called));
  },
};

// A select's options are set as they are read: by index, and by how
// many there are.
class HTMLOptionsCollection extends HTMLCollection {
  get length() { return itemsOf(this).length; }
  set length(count) {
    const { select } = listState(this);
    const options = itemsOf(this);
    count >>>= 0;
    for (let i = options.length - 1; i >= count; i--) options[i].remove();
    for (let i = options.length; i < count; i++) select.appendChild(select.ownerDocument.createElement("option"));
  }
  get selectedIndex() { return listState(this).select.selectedIndex; }
  set selectedIndex(i) { listState(this).select.selectedIndex = i; }
  add(option, before = null) {
    const { select } = listState(this);
    if (!(option instanceof HTMLOptionElement || option instanceof HTMLOptGroupElement)) {
      throw new TypeError("An option or a group of them is added.");
    }
    if (option.contains(select)) throw new DOMException("The option holds its select.", "HierarchyRequestError");
    if (typeof before === "number") before = itemsOf(this)[before] || null;
    if (before !== null && !(before instanceof Node && select.contains(before) && before !== select)) {
      throw new DOMException("The option to add before is not the select's.", "NotFoundError");
    }
    if (before === option) return;
    (before ? before.parentNode : select).insertBefore(option, before);
  }
  remove(i) {
    const option = itemsOf(this)[i];
    if (option) option.remove();
  }
}
HTMLOptionsCollection.kind = {
  names: plainNames,
  put(state, i, option) {
    const options = itemsOf(state.list);
    if (option === null || option === undefined) {
      if (options[i]) options[i].remove();
      return;
    }
    if (!(option instanceof HTMLOptionElement)) throw new TypeError("An option is set.");
    if (i > options.length) state.list.length = i;
    if (options[i] && i < options.length) options[i].parentNode.replaceChild(option, options[i]);
    else state.select.appendChild(option);
  },
};

// selected is the live collection a document keeps of the elements a
// selector matches.
const selected = (root, name, selector, Class = HTMLCollection) =>
  keptList(root, name, () => liveList(Class, () => foundIn(root, selector)));

for (const Class of [NodeList, HTMLCollection, HTMLAllCollection, RadioNodeList, HTMLFormControlsCollection,
  HTMLOptionsCollection]) {
  Object.defineProperty(Class.prototype, Symbol.toStringTag, { value: Class.name, configurable: true });
}
