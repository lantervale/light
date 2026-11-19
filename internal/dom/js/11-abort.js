// ---- AbortController and AbortSignal: a way to call something off

const signals = new WeakMap();
const signalState = (signal) => {
  const state = signals.get(signal);
  if (!state) throw new TypeError("Illegal invocation");
  return state;
};
const newSignal = () => {
  const signal = Reflect.construct(EventTarget, [], AbortSignal);
  signals.set(signal, { aborted: false, reason: undefined, then: [], sources: null, followers: new Set() });
  return signal;
};
const abortReason = (reason) => (reason === undefined
  ? new DOMException("signal is aborted without reason", "AbortError") : reason);
// onAbort has a signal run something of light's own when it aborts,
// before the page hears of it.
const onAbort = (signal, run) => {
  const state = signalState(signal);
  if (!state.aborted) state.then.push(run);
};
// signalIn is the signal among the options a listener was added with:
// anything else given as one is refused.
const signalIn = (options) => {
  if (options === null || typeof options !== "object" || options.signal === undefined) return null;
  if (!signals.has(options.signal)) throw new TypeError("The signal of a listener is an AbortSignal.");
  return options.signal;
};
const aborted = (signal) => {
  const state = signals.get(signal);
  for (const run of state.then.splice(0)) run();
  dispatch(signal, trusted(new Event("abort")));
};
const abort = (signal, reason) => {
  const state = signalState(signal);
  if (state.aborted) return;
  state.aborted = true;
  state.reason = abortReason(reason);
  const following = [];
  for (const follower of state.followers) {
    const theirs = signals.get(follower);
    if (theirs.aborted) continue;
    theirs.aborted = true;
    theirs.reason = state.reason;
    following.push(follower);
  }
  aborted(signal);
  following.forEach(aborted);
};

class AbortSignal extends EventTarget {
  constructor() { illegal(); }
  get aborted() { return signalState(this).aborted; }
  get reason() { return signalState(this).reason; }
  throwIfAborted() {
    const state = signalState(this);
    if (state.aborted) throw state.reason;
  }
  static abort(reason) {
    const signal = newSignal();
    Object.assign(signals.get(signal), { aborted: true, reason: abortReason(reason) });
    return signal;
  }
  static timeout(ms) {
    if (!arguments.length) throw new TypeError("timeout takes how long to wait.");
    const wait = Number(ms);
    if (!Number.isFinite(wait) || Math.trunc(wait) < 0 || wait > Number.MAX_SAFE_INTEGER) {
      throw new TypeError("The time to wait is a count of milliseconds.");
    }
    const signal = newSignal();
    setTimeout(() => abort(signal, new DOMException("signal timed out", "TimeoutError")), Math.trunc(wait));
    return signal;
  }
  // any follows the signals that are followed themselves, never one
  // that follows.
  static any(given) {
    if (given === null || typeof given !== "object" || typeof given[Symbol.iterator] !== "function") {
      throw new TypeError("any takes a list of signals.");
    }
    const all = [...given];
    for (const signal of all) signalState(signal);
    const result = newSignal();
    const state = signals.get(result);
    state.sources = new Set();
    const first = all.find((signal) => signals.get(signal).aborted);
    if (first) {
      Object.assign(state, { aborted: true, reason: signals.get(first).reason });
      return result;
    }
    for (const signal of all) {
      for (const source of signals.get(signal).sources || [signal]) {
        state.sources.add(source);
        signals.get(source).followers.add(result);
      }
    }
    return result;
  }
}
withHandlers(AbortSignal.prototype, ["abort"]);
const controllers = new WeakMap();
class AbortController {
  constructor() { controllers.set(this, newSignal()); }
  get signal() {
    if (!controllers.has(this)) throw new TypeError("Illegal invocation");
    return controllers.get(this);
  }
  abort(reason) { abort(this.signal, reason); }
}
for (const Class of [AbortSignal, AbortController]) {
  Object.defineProperty(Class.prototype, Symbol.toStringTag, { value: Class.name, configurable: true });
}
