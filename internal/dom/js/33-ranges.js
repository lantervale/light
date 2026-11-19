// ---- ranges: a start and an end in the tree, moving as it changes

const ranges = new WeakMap();
const liveRanges = heldSet();
const rangeEnds = [["sn", "so"], ["en", "eo"]];
const rangeState = (range) => {
  const state = ranges.get(range);
  if (!state) throw new TypeError("Illegal invocation");
  return state;
};
const rooted = (node) => node.getRootNode();
const sized = (node) => {
  if (node instanceof DocumentType || node instanceof Attr) return 0;
  return node instanceof CharacterData ? node.length : kidsOf(node).length;
};
const indexIn = (node) => kidsOf(node.parentNode).indexOf(node);
// kidAt is the child of a node at an offset, null where it has none.
const kidAt = (node, at) => (sized(node) && !(node instanceof CharacterData) ? kidsOf(node)[at] || null : null);
// pointsCompared is -1 for a point of the tree before another, 1 for
// one after, 0 for the same.
const pointsCompared = (a, ao, b, bo) => {
  if (a === b) return ao === bo ? 0 : ao < bo ? -1 : 1;
  const how = b.compareDocumentPosition(a);
  if (how & 4) return -pointsCompared(b, bo, a, ao);
  if (how & 8) {
    let child = b;
    while (child.parentNode !== a) child = child.parentNode;
    if (indexIn(child) < ao) return 1;
  }
  return -1;
};
const pointGiven = (node, offset) => {
  aNode(node);
  if (node instanceof DocumentType) throw new DOMException("A doctype holds no point.", "InvalidNodeTypeError");
  offset >>>= 0;
  if (offset > sized(node)) throw new DOMException("The offset is past the end of the node.", "IndexSizeError");
  return offset;
};
const startAt = (state, node, offset) => {
  offset = pointGiven(node, offset);
  if (rooted(state.sn) !== rooted(node) || pointsCompared(node, offset, state.en, state.eo) > 0) {
    state.en = node;
    state.eo = offset;
  }
  state.sn = node;
  state.so = offset;
};
const endAt = (state, node, offset) => {
  offset = pointGiven(node, offset);
  if (rooted(state.sn) !== rooted(node) || pointsCompared(node, offset, state.sn, state.so) < 0) {
    state.sn = node;
    state.so = offset;
  }
  state.en = node;
  state.eo = offset;
};
const parentGiven = (node) => {
  const parent = aNode(node).parentNode;
  if (!parent) throw new DOMException("The node has no parent.", "InvalidNodeTypeError");
  return parent;
};
const collapsedAt = (state) => state.sn === state.en && state.so === state.eo;
const commonOf = (state) => {
  let common = state.sn;
  while (!common.contains(state.en)) common = common.parentNode;
  return common;
};
// containedIn reports whether a node is wholly between the ends of a
// range.
const containedIn = (state, node) => pointsCompared(node, 0, state.sn, state.so) > 0 &&
  pointsCompared(node, sized(node), state.en, state.eo) < 0;
const partlyIn = (state, node) => node.contains(state.sn) !== node.contains(state.en);
// afterCut is where a range collapses to once what it holds is gone.
const afterCut = (state) => {
  if (state.sn.contains(state.en)) return [state.sn, state.so];
  let ref = state.sn;
  while (ref.parentNode && !ref.parentNode.contains(state.en)) ref = ref.parentNode;
  return [ref.parentNode, indexIn(ref) + 1];
};
const fragmentFor = (node) => (node.ownerDocument || node).createDocumentFragment();
const textCopy = (node, at, count) => {
  const copy = node.cloneNode(false);
  copy.data = node.substringData(at, count);
  return copy;
};

// taken gives what a range holds as a fragment: copies when copying,
// else the nodes themselves, taken from the tree.
const rangeTaken = (state, copying) => {
  const fragment = fragmentFor(state.sn);
  if (collapsedAt(state)) return fragment;
  const { sn, so, en, eo } = state;
  const cut = (node, at, count) => {
    fragment.appendChild(textCopy(node, at, count));
    if (!copying) node.replaceData(at, count, "");
  };
  if (sn === en && sn instanceof CharacterData) {
    cut(sn, so, eo - so);
    return fragment;
  }
  const common = commonOf(state);
  const kids = kidsOf(common);
  const first = sn.contains(en) ? null : kids.find((kid) => partlyIn(state, kid));
  const last = en.contains(sn) ? null : kids.findLast((kid) => partlyIn(state, kid));
  const whole = kids.filter((kid) => containedIn(state, kid));
  if (whole.some((kid) => kid instanceof DocumentType)) refuse("A doctype is not taken out of its document.");
  const [node, offset] = copying ? [] : afterCut(state);
  const part = (kid, inner) => {
    const copy = kid.cloneNode(false);
    fragment.appendChild(copy);
    copy.appendChild(rangeTaken(inner, copying));
  };
  if (first instanceof CharacterData) cut(sn, so, sized(sn) - so);
  else if (first) part(first, { sn, so, en: first, eo: sized(first) });
  for (const kid of whole) fragment.appendChild(copying ? kid.cloneNode(true) : kid);
  if (last instanceof CharacterData) cut(en, 0, eo);
  else if (last) part(last, { sn: last, so: 0, en, eo });
  if (!copying) Object.assign(state, { sn: node, so: offset, en: node, eo: offset });
  return fragment;
};
const deleted = (state) => {
  if (collapsedAt(state)) return;
  const { sn, so, en, eo } = state;
  if (sn === en && sn instanceof CharacterData) {
    sn.replaceData(so, eo - so, "");
    return;
  }
  const gone = [];
  const gather = (node) => {
    for (const kid of kidsOf(node)) {
      if (containedIn(state, kid)) gone.push(kid);
      else if (kid.contains(sn) || kid.contains(en)) gather(kid);
    }
  };
  gather(commonOf(state));
  const [node, offset] = afterCut(state);
  if (sn instanceof CharacterData) sn.replaceData(so, sized(sn) - so, "");
  for (const kid of gone) detach(kid);
  if (en instanceof CharacterData) en.replaceData(0, eo, "");
  Object.assign(state, { sn: node, so: offset, en: node, eo: offset });
};
const inserted = (state, node) => {
  const { sn, so } = state;
  if (sn instanceof ProcessingInstruction || sn instanceof Comment || (sn instanceof Text && !sn.parentNode) ||
    sn === node) {
    refuse("A node is not put there.");
  }
  let before = sn instanceof Text ? sn : kidAt(sn, so);
  const parent = before ? before.parentNode : sn;
  insertable(parent, node, before);
  if (sn instanceof Text) before = sn.splitText(so);
  if (node === before) before = before.nextSibling;
  if (node.parentNode) detach(node);
  let offset = before ? indexIn(before) : sized(parent);
  offset += node instanceof DocumentFragment ? sized(node) : 1;
  insert(parent, node, before);
  if (collapsedAt(state)) {
    state.en = parent;
    state.eo = offset;
  }
};
const textsIn = (state) => {
  const { sn, so, en, eo } = state;
  if (sn === en && sn instanceof Text) return sn.data.substring(so, eo);
  let text = sn instanceof Text ? sn.data.substring(so) : "";
  const stop = kidAt(en, eo) || followingIn(en, null, true);
  let node = kidAt(sn, so) || followingIn(sn, null, true);
  for (; node && node !== stop; node = followingIn(node, null)) {
    if (node instanceof Text && node !== en) text += node.data;
  }
  return en instanceof Text ? text + en.data.substring(0, eo) : text;
};

class AbstractRange {
  constructor() { illegal(); }
  get startOffset() { return rangeState(this).so; }
  get endOffset() { return rangeState(this).eo; }
  get collapsed() { return collapsedAt(rangeState(this)); }
}
// NodeRange is a range whose ends are in nodes: every range light has.
class NodeRange extends AbstractRange {
  get startContainer() { return rangeState(this).sn; }
  get endContainer() { return rangeState(this).en; }
}
const rangeMade = (Class, state) => {
  const range = Object.create(Class.prototype);
  ranges.set(range, state);
  return range;
};
const liveRange = (state, target) => {
  ranges.set(target, state);
  held(liveRanges, target, state);
  return target;
};
const HOW = { START_TO_START: 0, START_TO_END: 1, END_TO_END: 2, END_TO_START: 3 };
class Range extends NodeRange {
  constructor() {
    return liveRange({ sn: document, so: 0, en: document, eo: 0 }, Object.create(new.target.prototype));
  }
  get commonAncestorContainer() { return commonOf(rangeState(this)); }
  setStart(node, offset) { startAt(rangeState(this), node, offset); }
  setEnd(node, offset) { endAt(rangeState(this), node, offset); }
  setStartBefore(node) { startAt(rangeState(this), parentGiven(node), indexIn(node)); }
  setStartAfter(node) { startAt(rangeState(this), parentGiven(node), indexIn(node) + 1); }
  setEndBefore(node) { endAt(rangeState(this), parentGiven(node), indexIn(node)); }
  setEndAfter(node) { endAt(rangeState(this), parentGiven(node), indexIn(node) + 1); }
  collapse(toStart = false) {
    const state = rangeState(this);
    if (toStart) Object.assign(state, { en: state.sn, eo: state.so });
    else Object.assign(state, { sn: state.en, so: state.eo });
  }
  selectNode(node) {
    const state = rangeState(this), parent = parentGiven(node), at = indexIn(node);
    Object.assign(state, { sn: parent, so: at, en: parent, eo: at + 1 });
  }
  selectNodeContents(node) {
    const state = rangeState(this);
    if (aNode(node) instanceof DocumentType) throw new DOMException("A doctype holds nothing.", "InvalidNodeTypeError");
    Object.assign(state, { sn: node, so: 0, en: node, eo: sized(node) });
  }
  compareBoundaryPoints(how, other) {
    const state = rangeState(this);
    if (arguments.length < 2) throw new TypeError("compareBoundaryPoints takes which ends and a range.");
    if (!(other instanceof Range)) throw new TypeError("The range to compare with is not a Range.");
    how = Number(how) & 0xFFFF;
    if (how > 3) throw new DOMException("There is no such pair of ends.", "NotSupportedError");
    const source = rangeState(other);
    if (rooted(state.sn) !== rooted(source.sn)) {
      throw new DOMException("The ranges are in different trees.", "WrongDocumentError");
    }
    const mine = how === HOW.START_TO_START || how === HOW.END_TO_START ? [state.sn, state.so] : [state.en, state.eo];
    const theirs = how === HOW.START_TO_START || how === HOW.START_TO_END ? [source.sn, source.so]
      : [source.en, source.eo];
    return pointsCompared(...mine, ...theirs);
  }
  deleteContents() { deleted(rangeState(this)); }
  extractContents() { return rangeTaken(rangeState(this), false); }
  cloneContents() { return rangeTaken(rangeState(this), true); }
  insertNode(node) { inserted(rangeState(this), aNode(node)); }
  surroundContents(parent) {
    const state = rangeState(this);
    aNode(parent);
    const common = commonOf(state);
    for (const end of [state.sn, state.en]) {
      for (let node = end; node !== common; node = node.parentNode) {
        if (!(node instanceof Text)) throw new DOMException("The range holds part of a node.", "InvalidStateError");
      }
    }
    if (parent instanceof Document || parent instanceof DocumentType || parent instanceof DocumentFragment) {
      throw new DOMException("Such a node holds no content of a range.", "InvalidNodeTypeError");
    }
    const fragment = rangeTaken(state, false);
    for (const kid of kidsOf(parent)) detach(kid);
    inserted(state, parent);
    parent.appendChild(fragment);
    this.selectNode(parent);
  }
  cloneRange() {
    const { sn, so, en, eo } = rangeState(this);
    return liveRange({ sn, so, en, eo }, Object.create(Range.prototype));
  }
  detach() { rangeState(this); }
  isPointInRange(node, offset) {
    const state = rangeState(this);
    if (rooted(aNode(node)) !== rooted(state.sn)) return false;
    offset = pointGiven(node, offset);
    return pointsCompared(node, offset, state.sn, state.so) >= 0 && pointsCompared(node, offset, state.en, state.eo) <= 0;
  }
  comparePoint(node, offset) {
    const state = rangeState(this);
    if (rooted(aNode(node)) !== rooted(state.sn)) {
      throw new DOMException("The node is in another tree.", "WrongDocumentError");
    }
    offset = pointGiven(node, offset);
    if (pointsCompared(node, offset, state.sn, state.so) < 0) return -1;
    return pointsCompared(node, offset, state.en, state.eo) > 0 ? 1 : 0;
  }
  intersectsNode(node) {
    const state = rangeState(this);
    if (rooted(aNode(node)) !== rooted(state.sn)) return false;
    const parent = node.parentNode;
    if (!parent) return true;
    const at = indexIn(node);
    return pointsCompared(parent, at, state.en, state.eo) < 0 && pointsCompared(parent, at + 1, state.sn, state.so) > 0;
  }
  createContextualFragment(markup) {
    const { sn } = rangeState(this);
    const within = sn instanceof Element ? sn : sn.parentNode instanceof Element ? sn.parentNode : null;
    const fragment = fragmentFor(sn);
    const name = within && within.localName !== "html" ? within.localName : "body";
    for (const node of wrapAll(host("fragment", String(markup), name))) fragment.appendChild(node);
    return fragment;
  }
  getBoundingClientRect() {
    const { sn } = rangeState(this);
    return sn instanceof Element ? sn.getBoundingClientRect() : new DOMRect();
  }
  getClientRects() { rangeState(this); return []; }
  toString() { return textsIn(rangeState(this)); }
}
for (const [name, value] of Object.entries(HOW)) {
  Object.defineProperty(Range, name, { value, enumerable: true });
  Object.defineProperty(Range.prototype, name, { value, enumerable: true });
}

class StaticRange extends NodeRange {
  constructor(init) {
    if (init === null || typeof init !== "object") throw new TypeError("A StaticRange is made from its two ends.");
    const { startContainer: sn, startOffset: so, endContainer: en, endOffset: eo } = init;
    for (const node of [sn, en]) {
      if (!(node instanceof Node)) throw new TypeError("An end of a range is in a Node.");
    }
    if ([so, eo].includes(undefined)) throw new TypeError("An end of a range has an offset.");
    for (const node of [sn, en]) {
      if (node instanceof DocumentType || node instanceof Attr) {
        throw new DOMException("Such a node holds no end of a range.", "InvalidNodeTypeError");
      }
    }
    return rangeMade(new.target, { sn, so: so >>> 0, en, eo: eo >>> 0 });
  }
}
for (const Class of [AbstractRange, NodeRange, Range, StaticRange]) {
  Object.defineProperty(Class.prototype, Symbol.toStringTag, { value: Class.name, configurable: true });
}
define(Document.prototype, {
  createRange() { return liveRange({ sn: this, so: 0, en: this, eo: 0 }, Object.create(Range.prototype)); },
});

// The ranges that are held follow the tree: a node that leaves takes
// the ends in it to where it was, and the ends after it one back.
treeHooks.leaving.push((node, holds) => {
  let parent, at = -1;
  for (const state of holding(liveRanges)) {
    for (const [n, o] of rangeEnds) {
      const inside = holds(state[n]);
      if (parent === undefined) parent = node.parentNode;
      if (!inside && state[n] !== parent) continue;
      if (at < 0) at = indexIn(node);
      if (inside) {
        state[n] = parent;
        state[o] = at;
      } else if (state[o] > at) {
        state[o]--;
      }
    }
  }
});
treeHooks.entered.push((parent, where) => {
  for (const state of holding(liveRanges)) {
    if (state.sn === parent && state.so > where()) state.so++;
    if (state.en === parent && state.eo > where()) state.eo++;
  }
});
treeHooks.retexted.push((node, at, count, added) => {
  for (const state of holding(liveRanges)) {
    for (const [n, o] of rangeEnds) {
      if (state[n] !== node || state[o] <= at) continue;
      state[o] = state[o] <= at + count ? at : state[o] + added - count;
    }
  }
});
// textSplit moves the ends in the text cut off a node to the node that
// took it, and the end just after the node past the new one.
const textSplit = (node, at, rest, parent) => {
  if (!treeHooked()) return;
  const after = indexIn(node) + 1;
  for (const state of holding(liveRanges)) {
    for (const [n, o] of rangeEnds) {
      if (state[n] === node && state[o] > at) {
        state[n] = rest;
        state[o] -= at;
      } else if (state[n] === parent && state[o] === after) {
        state[o]++;
      }
    }
  }
};
// textJoined moves the ends in a text node about to be joined to the
// one before it, and those just before it, into that one.
const textJoined = (into, length, node) => {
  if (!treeHooked()) return;
  const parent = node.parentNode, at = indexIn(node);
  for (const state of holding(liveRanges)) {
    for (const [n, o] of rangeEnds) {
      if (state[n] === node) {
        state[n] = into;
        state[o] += length;
      } else if (state[n] === parent && state[o] === at) {
        state[n] = into;
        state[o] = length;
      }
    }
  }
};
