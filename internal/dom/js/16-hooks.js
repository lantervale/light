// ---- what follows the tree as it changes

// Ranges and node iterators stand in the tree and move with it: each
// keeps its live objects in a held set, and while one has any, every
// change of the tree a script makes is told to treeHooks. Nothing is
// asked of light for it when no set holds anything.
const heldSets = [];
const heldSet = () => {
  const set = { states: new Map(), swept: 64 };
  heldSets.push(set);
  return set;
};
// held keeps the state of a live object for as long as the object is:
// the states are what the hooks walk, and those of objects that are
// gone are dropped each time the set has doubled.
const held = (set, object, state) => {
  set.states.set(state, new WeakRef(object));
  if (set.states.size < set.swept) return;
  for (const [one, ref] of set.states) if (!ref.deref()) set.states.delete(one);
  set.swept = Math.max(64, set.states.size * 2);
};
const holding = (set) => set.states.keys();
const treeHooked = () => heldSets.some((set) => set.states.size > 0);

const treeHooks = { leaving: [], entered: [], retexted: [] };
// left tells the hooks of a node about to be taken from its parent,
// with what says whether another node is the node or under it: light
// is asked once a node, however many ranges stand in it.
const left = (node) => {
  const leaf = node instanceof CharacterData, known = new Map();
  const holds = (other) => {
    if (other === node) return true;
    if (leaf) return false;
    let inside = known.get(other);
    if (inside === undefined) known.set(other, inside = node.contains(other));
    return inside;
  };
  for (const hook of treeHooks.leaving) hook(node, holds);
};
// nodeLeaving tells of a node about to be taken from its parent.
const nodeLeaving = (node) => {
  if (treeHooked()) left(node);
};
// nodeEntered tells of a node just put under a parent, with what gives
// its index there, asked of light only by a hook that needs it.
const nodeEntered = (parent, node) => {
  if (!treeHooked()) return;
  let at = -1;
  const where = () => (at < 0 ? (at = kidsOf(parent).indexOf(node)) : at);
  for (const hook of treeHooks.entered) hook(parent, where);
};
// textReplaced tells of count units of a node's text from at replaced
// by added ones.
const textReplaced = (node, at, count, added) => {
  if (treeHooked()) for (const hook of treeHooks.retexted) hook(node, at, count, added);
};
// emptied takes every child from a node about to be given new content,
// one by one, when something follows the tree.
const emptied = (node) => {
  if (!treeHooked()) return;
  for (const child of kidsOf(node)) {
    left(child);
    host("remove", child[REF]);
  }
};
