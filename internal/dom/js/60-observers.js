// ---- mutation observers

// A MutationRecord is what an observer is told of one change.
const recorded = new WeakMap();
class MutationRecord {
  constructor() { illegal(); }
}
for (const key of ["type", "target", "addedNodes", "removedNodes", "previousSibling", "nextSibling", "attributeName",
  "attributeNamespace", "oldValue"]) {
  Object.defineProperty(MutationRecord.prototype, key, {
    get() {
      const record = recorded.get(this);
      if (!record) throw new TypeError("Illegal invocation");
      return record[key];
    },
    enumerable: true, configurable: true,
  });
}
const mutationRecord = (fields) => {
  const record = Object.create(MutationRecord.prototype);
  recorded.set(record, fields);
  return record;
};
const observers = new Set();
let delivering = false;
class MutationObserver {
  constructor(callback) {
    if (typeof callback !== "function") throw new TypeError("MutationObserver needs a function");
    this._callback = callback;
    this._watched = new Map();
    this._records = [];
    this._gone = [];
  }
  observe(target, options = {}) {
    if (!(target instanceof Node)) throw new TypeError("MutationObserver observes a Node");
    const attributes = options.attributes === undefined
      ? options.attributeFilter !== undefined || options.attributeOldValue !== undefined : Boolean(options.attributes);
    const characterData = options.characterData === undefined
      ? options.characterDataOldValue !== undefined : Boolean(options.characterData);
    if (!options.childList && !attributes && !characterData) {
      throw new TypeError("The options object must set at least one of 'attributes', 'characterData', or 'childList' to true.");
    }
    if (!attributes && (options.attributeOldValue || options.attributeFilter !== undefined)) {
      throw new TypeError("The options object asks for attributes it does not watch.");
    }
    if (!characterData && options.characterDataOldValue) {
      throw new TypeError("The options object asks for the old text of text it does not watch.");
    }
    this._watched.set(target, { ...options, attributes, characterData });
    this._gone = this._gone.filter((gone) => gone.source !== target);
    observers.add(this);
    kinWatch();
  }
  disconnect() {
    this._watched.clear();
    this._records = [];
    this._gone = [];
    observers.delete(this);
    kinWatch();
  }
  takeRecords() {
    const records = this._records;
    this._records = [];
    return records;
  }
  // _wants reports whether a record is of what the observer watches.
  _wants(record) {
    const asked = (options) => (record.type === "childList" && options.childList) ||
      (record.type === "characterData" && options.characterData) || (record.type === "attributes" &&
        options.attributes && (!options.attributeFilter || options.attributeFilter.includes(record.attributeName)));
    for (const [target, options] of this._watched) {
      if (target !== record.target && !(options.subtree && target.contains(record.target))) continue;
      if (asked(options)) return options;
    }
    for (const { node, options } of this._gone) {
      if ((node === record.target || node.contains(record.target)) && asked(options)) return options;
    }
    return null;
  }
  // _keeps has the observer follow what has just left a subtree it
  // watches, until it is next told of its records.
  _keeps(change) {
    if (change.type !== "childList" || !change.removedNodes || !change.removedNodes.length) return;
    const follow = (source, options) => {
      for (const node of change.removedNodes) this._gone.push({ node, options, source });
    };
    for (const [target, options] of this._watched) {
      if (options.subtree && (target === change.target || target.contains(change.target))) follow(target, options);
    }
    for (const { node, options, source } of this._gone.slice()) {
      if (node === change.target || node.contains(change.target)) follow(source, options);
    }
  }
}
observing = () => observers.size > 0;
observed = (change) => {
  if (!bag.mixed) return observedHere(change);
  for (const one of bag("all")) one.heard(change);
  return undefined;
};
const observedHere = (change) => {
  let queued = false;
  for (const observer of observers) {
    const options = observer._wants(change);
    observer._keeps(change);
    if (!options) continue;
    const keepsOld = change.type === "attributes" ? options.attributeOldValue : options.characterDataOldValue;
    observer._records.push(mutationRecord({
      type: change.type, target: change.target,
      addedNodes: nodeList(Array.from(change.addedNodes || [])),
      removedNodes: nodeList(Array.from(change.removedNodes || [])),
      previousSibling: change.previousSibling || null, nextSibling: change.nextSibling || null,
      attributeName: change.attributeName || null, attributeNamespace: change.attributeNamespace || null,
      oldValue: keepsOld && change.oldValue !== undefined ? change.oldValue : null,
    }));
    queued = true;
  }
  if (!queued || delivering) return;
  delivering = true;
  Promise.resolve().then(() => {
    delivering = false;
    for (const observer of Array.from(observers)) {
      observer._gone = [];
      const records = observer.takeRecords();
      if (records.length) guarded(observer._callback, observer, [records, observer]);
    }
  });
};
