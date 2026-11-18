// ---- mutation observers

const observers = new Set();
let delivering = false;
class MutationObserver {
  constructor(callback) {
    if (typeof callback !== "function") throw new TypeError("MutationObserver needs a function");
    this._callback = callback;
    this._watched = new Map();
    this._records = [];
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
    observers.add(this);
    watching = true;
  }
  disconnect() {
    this._watched.clear();
    this._records = [];
    observers.delete(this);
    watching = observers.size > 0;
  }
  takeRecords() {
    const records = this._records;
    this._records = [];
    return records;
  }
  // _wants reports whether a record is of what the observer watches.
  _wants(record) {
    for (const [target, options] of this._watched) {
      if (target !== record.target && !(options.subtree && target.contains(record.target))) continue;
      if (record.type === "childList" && options.childList) return options;
      if (record.type === "characterData" && options.characterData) return options;
      if (record.type === "attributes" && options.attributes &&
        (!options.attributeFilter || options.attributeFilter.includes(record.attributeName))) return options;
    }
    return null;
  }
}
observed = (change) => {
  let queued = false;
  for (const observer of observers) {
    const options = observer._wants(change);
    if (!options) continue;
    const keepsOld = change.type === "attributes" ? options.attributeOldValue : options.characterDataOldValue;
    observer._records.push({
      type: change.type, target: change.target,
      addedNodes: nodeList(Array.from(change.addedNodes || [])),
      removedNodes: nodeList(Array.from(change.removedNodes || [])),
      previousSibling: change.previousSibling || null, nextSibling: change.nextSibling || null,
      attributeName: change.attributeName || null, attributeNamespace: change.attributeNamespace || null,
      oldValue: keepsOld && change.oldValue !== undefined ? change.oldValue : null,
    });
    queued = true;
  }
  if (!queued || delivering) return;
  delivering = true;
  Promise.resolve().then(() => {
    delivering = false;
    for (const observer of Array.from(observers)) {
      const records = observer.takeRecords();
      if (records.length) guarded(observer._callback, observer, [records, observer]);
    }
  });
};
