// ---- walking the tree: NodeFilter, TreeWalker and NodeIterator

const FILTER_ACCEPT = 1, FILTER_REJECT = 2, FILTER_SKIP = 3;
const SHOW_ALL = 0xFFFFFFFF;
const nodeFilters = {
  FILTER_ACCEPT, FILTER_REJECT, FILTER_SKIP, SHOW_ALL, SHOW_ELEMENT: 0x1, SHOW_ATTRIBUTE: 0x2, SHOW_TEXT: 0x4,
  SHOW_CDATA_SECTION: 0x8, SHOW_ENTITY_REFERENCE: 0x10, SHOW_ENTITY: 0x20, SHOW_PROCESSING_INSTRUCTION: 0x40,
  SHOW_COMMENT: 0x80, SHOW_DOCUMENT: 0x100, SHOW_DOCUMENT_TYPE: 0x200, SHOW_DOCUMENT_FRAGMENT: 0x400,
  SHOW_NOTATION: 0x800,
};
// NodeFilter is the interface of a page's own filters: it has only its
// constants.
const NodeFilter = { NodeFilter: () => illegal() }.NodeFilter;
for (const [name, value] of Object.entries(nodeFilters)) {
  Object.defineProperty(NodeFilter, name, { value, enumerable: true });
}

const walkers = new WeakMap();
const walkerOf = (walker) => {
  const state = walkers.get(walker);
  if (!state) throw new TypeError("Illegal invocation");
  return state;
};
// filtered asks a walker's filter of a node: a function, or an object
// whose acceptNode is asked for each time. A filter that walks while
// it is asked is refused.
const filtered = (state, node) => {
  if (state.active) throw new DOMException("The filter is being asked already.", "InvalidStateError");
  if (!((state.show >>> (node.nodeType - 1)) & 1)) return FILTER_SKIP;
  const filter = state.filter;
  if (filter === null) return FILTER_ACCEPT;
  state.active = true;
  try {
    if (typeof filter === "function") return filter.call(undefined, node) & 0xFFFF;
    const accept = filter.acceptNode;
    if (typeof accept !== "function") throw new TypeError("A filter is a function or has an acceptNode.");
    return accept.call(filter, node) & 0xFFFF;
  } finally {
    state.active = false;
  }
};
const walkerGiven = (root, show, filter) => {
  if (!(root instanceof Node)) throw new TypeError("The root to walk from is not a Node.");
  if (filter !== null && typeof filter !== "object" && typeof filter !== "function") {
    throw new TypeError("A filter is a function or has an acceptNode.");
  }
  return { root, show: show === undefined ? SHOW_ALL : show >>> 0, filter, active: false };
};
const lastUnder = (node) => {
  while (node.lastChild) node = node.lastChild;
  return node;
};
// followingIn is the node after one in tree order that is under a
// root, null for none; skip leaves out what is under the node itself.
const followingIn = (node, root, skip = false) => {
  if (!skip && node.firstChild) return node.firstChild;
  for (let at = node; at && at !== root; at = at.parentNode) {
    if (at.nextSibling) return at.nextSibling;
  }
  return null;
};
const precedingIn = (node, root) => {
  if (node === root) return null;
  const before = node.previousSibling;
  return before ? lastUnder(before) : node.parentNode;
};

class TreeWalker {
  constructor(key, state) {
    if (key !== token) illegal();
    walkers.set(this, { ...state, current: state.root });
  }
  get root() { return walkerOf(this).root; }
  get whatToShow() { return walkerOf(this).show; }
  get filter() { return walkerOf(this).filter; }
  get currentNode() { return walkerOf(this).current; }
  set currentNode(node) {
    if (!(node instanceof Node)) throw new TypeError("The node to walk from is not a Node.");
    walkerOf(this).current = node;
  }
  parentNode() {
    const state = walkerOf(this);
    for (let node = state.current; node && node !== state.root;) {
      node = node.parentNode;
      if (node && filtered(state, node) === FILTER_ACCEPT) return state.current = node;
    }
    return null;
  }
  firstChild() { return walkedChild(walkerOf(this), true); }
  lastChild() { return walkedChild(walkerOf(this), false); }
  nextSibling() { return walkedSibling(walkerOf(this), true); }
  previousSibling() { return walkedSibling(walkerOf(this), false); }
  previousNode() {
    const state = walkerOf(this);
    let node = state.current;
    while (node !== state.root) {
      let sibling = node.previousSibling;
      while (sibling) {
        node = sibling;
        let result = filtered(state, node);
        while (result !== FILTER_REJECT && node.lastChild) {
          node = node.lastChild;
          result = filtered(state, node);
        }
        if (result === FILTER_ACCEPT) return state.current = node;
        sibling = node.previousSibling;
      }
      if (node === state.root || !node.parentNode) return null;
      node = node.parentNode;
      if (filtered(state, node) === FILTER_ACCEPT) return state.current = node;
    }
    return null;
  }
  nextNode() {
    const state = walkerOf(this);
    let node = state.current, result = FILTER_ACCEPT;
    for (;;) {
      while (result !== FILTER_REJECT && node.firstChild) {
        node = node.firstChild;
        result = filtered(state, node);
        if (result === FILTER_ACCEPT) return state.current = node;
      }
      let sibling = null;
      for (let at = node; at && at !== state.root && !sibling; at = at.parentNode) sibling = at.nextSibling;
      if (!sibling) return null;
      node = sibling;
      result = filtered(state, node);
      if (result === FILTER_ACCEPT) return state.current = node;
    }
  }
}
// walkedChild is the first or the last child a walker accepts, under
// the children it skips too.
const walkedChild = (state, first) => {
  let node = first ? state.current.firstChild : state.current.lastChild;
  while (node) {
    const result = filtered(state, node);
    if (result === FILTER_ACCEPT) return state.current = node;
    if (result === FILTER_SKIP) {
      const child = first ? node.firstChild : node.lastChild;
      if (child) {
        node = child;
        continue;
      }
    }
    while (node) {
      const sibling = first ? node.nextSibling : node.previousSibling;
      if (sibling) {
        node = sibling;
        break;
      }
      const parent = node.parentNode;
      if (!parent || parent === state.root || parent === state.current) return null;
      node = parent;
    }
  }
  return null;
};
const walkedSibling = (state, next) => {
  let node = state.current;
  if (node === state.root) return null;
  for (;;) {
    let sibling = next ? node.nextSibling : node.previousSibling;
    while (sibling) {
      node = sibling;
      const result = filtered(state, node);
      if (result === FILTER_ACCEPT) return state.current = node;
      sibling = next ? node.firstChild : node.lastChild;
      if (result === FILTER_REJECT || !sibling) sibling = next ? node.nextSibling : node.previousSibling;
    }
    node = node.parentNode;
    if (!node || node === state.root) return null;
    if (filtered(state, node) === FILTER_ACCEPT) return null;
  }
};

// The iterators that are still held are told of each node about to
// leave the tree, so that none is left on a node that is gone.
const iterators = heldSet();
class NodeIterator {
  constructor(key, state) {
    if (key !== token) illegal();
    const own = { ...state, reference: state.root, before: true };
    walkers.set(this, own);
    held(iterators, this, own);
  }
  get root() { return walkerOf(this).root; }
  get whatToShow() { return walkerOf(this).show; }
  get filter() { return walkerOf(this).filter; }
  get referenceNode() { return walkerOf(this).reference; }
  get pointerBeforeReferenceNode() { return walkerOf(this).before; }
  nextNode() { return iterated(walkerOf(this), true); }
  previousNode() { return iterated(walkerOf(this), false); }
  detach() { walkerOf(this); }
}
// iterated moves an iterator one node on, or back, to the next its
// filter accepts. Where it stands shows only once a node is accepted;
// until then the place it has reached is its flight, which a node
// taken out by the filter moves as it moves the iterator.
const iterated = (state, next) => {
  if (state.active) throw new DOMException("The filter is being asked already.", "InvalidStateError");
  const flight = state.flight = { reference: state.reference, before: state.before };
  try {
    for (;;) {
      if (flight.before !== next) {
        const node = next ? followingIn(flight.reference, state.root) : precedingIn(flight.reference, state.root);
        if (!node) return null;
        flight.reference = node;
      }
      flight.before = !next;
      const node = flight.reference;
      if (filtered(state, node) !== FILTER_ACCEPT) continue;
      state.reference = flight.reference;
      state.before = flight.before;
      return node;
    }
  } finally {
    state.flight = null;
  }
};
// A node about to leave takes what stands in it to the node next to
// it.
const movedOff = (at, root, node, holds) => {
  if (!holds(at.reference)) return;
  if (at.before) {
    const next = followingIn(node, root, true);
    if (next) {
      at.reference = next;
      return;
    }
    at.before = false;
  }
  const sibling = node.previousSibling;
  at.reference = sibling ? lastUnder(sibling) : node.parentNode;
};
const iteratorsLeft = (node, holds) => {
  for (const state of holding(iterators)) {
    if (holds(state.root)) continue;
    movedOff(state, state.root, node, holds);
    if (state.flight) movedOff(state.flight, state.root, node, holds);
  }
};
treeHooks.leaving.push(iteratorsLeft);

for (const Class of [TreeWalker, NodeIterator]) {
  Object.defineProperty(Class.prototype, Symbol.toStringTag, { value: Class.name, configurable: true });
}
define(Document.prototype, {
  createTreeWalker(root, show, filter = null) {
    if (!arguments.length) throw new TypeError("createTreeWalker takes the node to walk from.");
    return new TreeWalker(token, walkerGiven(root, show, filter));
  },
  createNodeIterator(root, show, filter = null) {
    if (!arguments.length) throw new TypeError("createNodeIterator takes the node to walk from.");
    return new NodeIterator(token, walkerGiven(root, show, filter));
  },
});
