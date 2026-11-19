// ---- the interfaces as their standards' Web IDL has them

// A page can see more of an interface than what it does: that its
// members are enumerable, that an operation given too few arguments
// refuses before it does anything, that an operation or an attribute
// asked of what is no object of the interface refuses, that a constant
// is never written. All of it follows from the IDL (88-idl.js), so it
// is done once, over the interfaces the window has when it is whole
// (90-window.js).
const idlFinished = (name, Class, row, heirs) => {
  const proto = Class.prototype;
  const fixed = (target, key, value) => Object.defineProperty(target, key,
    { value, writable: false, enumerable: true, configurable: false });
  for (const [key, value] of Object.entries(row.consts || {})) {
    fixed(Class, key, value);
    if (proto) fixed(proto, key, value);
  }
  if (!proto) return;
  // counted is an operation that refuses fewer arguments than it
  // needs.
  // An EventTarget's operations called bare are the window's, and a
  // window of another document is one too: they take any this.
  const open = name === "EventTarget";
  const counted = (key, fn, needs, Brand) => {
    const checked = {
      [key]() {
        if (Brand && !(this instanceof Brand)) throw new TypeError("Illegal invocation");
        if (arguments.length < needs) {
          throw new TypeError(`${name}.${key} takes ${needs} argument${needs > 1 ? "s" : ""}, not ${arguments.length}.`);
        }
        return fn.apply(this, arguments);
      },
    }[key];
    Object.defineProperty(checked, "length", { value: needs, configurable: true });
    return checked;
  };
  const operations = (target, ops, Brand) => {
    for (const [key, needs] of Object.entries(ops || {})) {
      const descriptor = Object.getOwnPropertyDescriptor(target, key);
      if (!descriptor || typeof descriptor.value !== "function") continue;
      descriptor.value = counted(key, descriptor.value, needs, Brand);
      Object.defineProperty(target, key, { ...descriptor, enumerable: true });
    }
  };
  // What is iterable and answers by index is walked by the functions
  // an array is walked by, themselves.
  const walked = row.ops && "entries" in row.ops ? ["entries", "keys", "values", "forEach"] : [];
  for (const key of walked) {
    Object.defineProperty(proto, key,
      { value: Array.prototype[key], writable: true, enumerable: true, configurable: true });
  }
  if (walked.length) {
    Object.defineProperty(proto, Symbol.iterator,
      { value: Array.prototype.values, writable: true, configurable: true });
  }
  const others = Object.fromEntries(Object.entries(row.ops || {}).filter(([key]) => !walked.includes(key)));
  operations(proto, others, open ? null : Class);
  operations(Class, row.statics, null);
  // An interface that has an operation of its own in place of the one
  // it inherits counts its arguments as well.
  for (const [key, needs] of Object.entries(row.ops || {})) {
    if (!needs) continue;
    for (const heir of heirs) {
      const descriptor = Object.getOwnPropertyDescriptor(heir, key);
      if (!descriptor || typeof descriptor.value !== "function" || !proto.isPrototypeOf(heir)) continue;
      Object.defineProperty(heir, key, { ...descriptor, value: counted(key, descriptor.value, needs, null) });
    }
  }
  for (const key of Object.keys(row.attrs || {})) {
    const descriptor = Object.getOwnPropertyDescriptor(proto, key);
    if (!descriptor || !descriptor.get) continue;
    const { get, set } = descriptor;
    const branded = Object.getOwnPropertyDescriptor({
      get [key]() {
        if (!(this instanceof Class)) throw new TypeError("Illegal invocation");
        return get.call(this);
      },
      set [key](value) {
        if (!(this instanceof Class)) throw new TypeError("Illegal invocation");
        set.call(this, value);
      },
    }, key);
    Object.defineProperty(proto, key,
      { get: branded.get, set: set && branded.set, enumerable: true, configurable: true });
  }
  if (!Object.hasOwn(proto, Symbol.toStringTag)) {
    Object.defineProperty(proto, Symbol.toStringTag, { value: name, configurable: true });
  }
  if (row.unscopables) {
    const hidden = Object.create(null);
    for (const key of row.unscopables) hidden[key] = true;
    Object.defineProperty(proto, Symbol.unscopables, { value: hidden, configurable: true });
  }
  Object.defineProperty(Class, "length", { value: row.length || 0, configurable: true });
};
const finishInterfaces = () => {
  const all = [];
  for (const key of Object.getOwnPropertyNames(window)) {
    const value = Object.getOwnPropertyDescriptor(window, key).value;
    if (typeof value === "function" && value.prototype) all.push(value.prototype);
  }
  for (const [name, row] of Object.entries(IDL)) {
    const Class = Object.getOwnPropertyDescriptor(window, name)?.value;
    if (typeof Class === "function") idlFinished(name, Class, row, all);
  }
};
