// ---- what the documents of a kin share
//
// light runs this script as a function of what it was handed: nothing
// for a document that is alone or the first of its kin, the bag of the
// kin for one that joins them.

// bag is what the documents of a kin share, and no script of a page
// ever sees: the first of them makes it, and light hands it to each
// that joins. It is their registry of windows (57-windows.js), and
// holds what makes a node one object for all of them: the symbol its
// number is under, the objects by number, the listeners and handlers
// of every target, version, which counts the changes to their trees,
// and mixed, whether one has reached into another yet. Until then
// each is as alone.
const bag = handed || (() => {
  const windows = new Map();
  const all = (what, id, of) => {
    if (what === "get") return windows.get(id);
    if (what === "all") return Array.from(windows.values());
    if (what === "set") windows.set(id, of);
    else if (windows.get(id) === of) windows.delete(id);
    return undefined;
  };
  return Object.assign(all, {
    REF: Symbol("ref"), wrappers: new Map(), listeners: new WeakMap(), handlers: new WeakMap(), version: 0,
    mixed: false,
  });
})();
