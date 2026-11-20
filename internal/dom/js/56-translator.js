// ---- Translator and LanguageDetector: the model on this device, for a page

// A page in a secure context finds both, as the Web Machine Learning
// group's Translator and Language Detector APIs say. light never
// downloads a model for a page: availability() answers "available" or
// "unavailable", never "downloadable", and "unavailable" says nothing
// of why. What the page asks goes to the model one ask at a time,
// behind what light asks of it itself.

// trustworthy reports whether an address is of a potentially
// trustworthy origin: https, a file, or this machine.
const trustworthy = (at) => at.protocol === "https:" || at.protocol === "wss:" || at.protocol === "file:" ||
  (at.protocol === "http:" || at.protocol === "ws:") &&
    /^(127(\.\d{1,3}){3}|\[::1\]|localhost|.+\.localhost)$/i.test(at.hostname);
const secureContext = trustworthy(here);

// languageTag is a BCP 47 tag in its canonical form; one that is none
// is a RangeError.
const languageTag = (given) => {
  const tag = String(given);
  if (typeof Intl === "object" && Intl.getCanonicalLocales) return Intl.getCanonicalLocales(tag)[0];
  if (/^[a-z]{2,8}(-[a-z0-9]{1,8})*$/i.test(tag)) return tag;
  throw new RangeError(`Incorrect locale information provided: ${tag}`);
};
const primary = (tag) => tag.split("-")[0].toLowerCase();

// untranslatable is a text with nothing to translate: blanks and
// control characters only.
const untranslatable = (text) => /^[\s\0-\x1f\x7f]*$/.test(text);
const usageOf = (text) => [...text].length;

class QuotaExceededError extends DOMException {
  constructor(message = "", options = {}) {
    super(message, "QuotaExceededError");
    this.requested = options.requested === undefined ? null : options.requested;
    this.quota = options.quota === undefined ? null : options.quota;
  }
}

// failure is the DOMException a failure light names is thrown as.
const failure = (why) => {
  const named = /^(NotSupportedError|QuotaExceededError|NotAllowedError|InvalidStateError): (.*)$/s.exec(why);
  return named ? new DOMException(named[2], named[1]) : new DOMException(why, "UnknownError");
};

// modelAsks are the asks of the page the model answers or that wait,
// by the number light knows each by.
let lastAsk = 0;
const modelAsks = new Map();
const askModel = (kind, from, into, text, piece, settle) => {
  const id = ++lastAsk;
  modelAsks.set(id, { piece, settle });
  try {
    host("assistant", "ask", id, kind, from, into, text);
  } catch (error) {
    modelAsks.delete(id);
    throw error;
  }
  return () => {
    if (modelAsks.delete(id)) host("assistant", "stop", id);
  };
};
const assisted = (id, said, why) => {
  const ask = modelAsks.get(id);
  if (!ask) return;
  if (typeof said === "string") {
    ask.piece(said);
    return;
  }
  modelAsks.delete(id);
  ask.settle(why ? failure(why) : null, said);
};

class CreateMonitor extends EventTarget {
  constructor() { illegal(); }
}
withHandlers(CreateMonitor.prototype, ["downloadprogress"]);
const newMonitor = () => Reflect.construct(EventTarget, [], CreateMonitor);

// options is a dictionary of create(), availability() or a call, its
// signal checked to be one.
const optionsOf = (given) => {
  const options = dictionary(given);
  if (options.signal !== undefined && !signals.has(options.signal)) {
    throw new TypeError("Failed to read the 'signal' property: it is not of type 'AbortSignal'.");
  }
  if (options.monitor !== undefined && typeof options.monitor !== "function") {
    throw new TypeError("Failed to read the 'monitor' property: it is not a function.");
  }
  return options;
};

// models are the state of each Translator and LanguageDetector: the
// controller its destroy() aborts.
const models = new WeakMap();
const modelOf = (object, Class) => {
  const state = models.get(object);
  if (!state || !(object instanceof Class)) throw new TypeError("Illegal invocation");
  return state;
};

// created is how create() makes a model once its options are read: the
// page may use the feature, the monitor hears the download it does not
// need, and the model is there for what was asked.
const created = (feature, options, available, make) => new Promise((resolve, reject) => {
  const signal = options.signal;
  if (signal && signal.aborted) {
    reject(signal.reason);
    return;
  }
  if (!host("assistant", "allowed", feature)) {
    const why = `Access to the feature "${feature}" is disallowed by permissions policy.`;
    reject(new DOMException(why, "NotAllowedError"));
    return;
  }
  const monitor = newMonitor();
  if (options.monitor) {
    try {
      options.monitor.call(undefined, monitor);
    } catch (error) {
      reject(error);
      return;
    }
  }
  if (!available()) {
    reject(new DOMException("The model on this device cannot do what was asked.", "NotSupportedError"));
    return;
  }
  let settled = false;
  const stopped = () => {
    if (!settled) {
      settled = true;
      reject(signal.reason);
    }
  };
  if (signal) onAbort(signal, stopped);
  const progress = (loaded) => {
    if (settled) return;
    const event = new ProgressEvent("downloadprogress", { loaded, total: 1, lengthComputable: true });
    dispatch(monitor, trusted(event));
  };
  setTimeout(() => {
    progress(0);
    progress(1);
    if (settled) return;
    settled = true;
    const made = make();
    if (signal) onAbort(signal, () => destroyModel(made, signal.reason));
    resolve(made);
  }, 0);
});

const destroyModel = (object, reason) => {
  const state = models.get(object);
  if (state) abort(state.destroyed.signal, reason);
};

// compositeOf is the signal of a call of a model: its destruction's,
// and the call's own.
const compositeOf = (state, options) => AbortSignal.any(options.signal
  ? [state.destroyed.signal, options.signal] : [state.destroyed.signal]);

// modelCall runs one call of a model: refused at once when it was
// destroyed or its signal aborted, called off when either aborts later.
const modelCall = (state, options, start) => new Promise((resolve, reject) => {
  const composite = compositeOf(state, options);
  if (composite.aborted) {
    reject(composite.reason);
    return;
  }
  let stop = null;
  const end = (error, value) => {
    if (error) reject(error);
    else resolve(value);
  };
  onAbort(composite, () => {
    if (stop) stop();
    reject(composite.reason);
  });
  try {
    stop = start(end);
  } catch (error) {
    reject(error);
  }
});

// quotaChecked refuses a text over a model's quota.
const quotaChecked = (state, text) => {
  const requested = usageOf(text);
  if (requested > state.quota) {
    throw new QuotaExceededError("The input is too large.", { requested, quota: state.quota });
  }
};

class Translator {
  constructor() { illegal(); }
  static availability(given) {
    return new Promise((resolve) => {
      const [from, into] = translatorLanguages(given);
      resolve(host("assistant", "available", "translate", from, into) ? "available" : "unavailable");
    });
  }
  static create(given) {
    return new Promise((resolve) => {
      const [from, into] = translatorLanguages(given);
      const options = optionsOf(given);
      resolve(created("translator", options, () => host("assistant", "available", "translate", from, into), () => {
        const made = Object.create(Translator.prototype);
        models.set(made, {
          from, into, destroyed: new AbortController(), quota: host("assistant", "quota"),
        });
        return made;
      }));
    });
  }
  get sourceLanguage() { return modelOf(this, Translator).from; }
  get targetLanguage() { return modelOf(this, Translator).into; }
  get inputQuota() { return modelOf(this, Translator).quota; }
  measureInputUsage(input, given) {
    const state = modelOf(this, Translator);
    const text = String(input);
    return modelCall(state, optionsOf(given), (end) => {
      end(null, usageOf(text));
      return null;
    });
  }
  translate(input, given) {
    const state = modelOf(this, Translator);
    const text = String(input);
    return modelCall(state, optionsOf(given), (end) => {
      if (untranslatable(text) || primary(state.from) === primary(state.into)) {
        end(null, text);
        return null;
      }
      quotaChecked(state, text);
      let made = "";
      const add = (piece) => { made += piece; };
      return askModel("translate", state.from, state.into, text, add, (error) => end(error, made));
    });
  }
  translateStreaming(input, given) {
    const state = modelOf(this, Translator);
    const text = String(input);
    const options = optionsOf(given);
    const composite = compositeOf(state, options);
    if (composite.aborted) throw composite.reason;
    let stop = null;
    return newStream((controller) => {
      onAbort(composite, () => {
        if (stop) stop();
        controller.error(composite.reason);
      });
      if (untranslatable(text) || primary(state.from) === primary(state.into)) {
        if (text) controller.enqueue(text);
        controller.close();
        return;
      }
      try {
        quotaChecked(state, text);
        stop = askModel("translate", state.from, state.into, text, (piece) => controller.enqueue(piece), (error) => {
          if (error) controller.error(error);
          else controller.close();
        });
      } catch (error) {
        controller.error(error);
      }
    }, () => {
      if (stop) stop();
    });
  }
  destroy() { destroyModel(this, new DOMException("The model was destroyed.", "AbortError")); }
}

// translatorLanguages are the languages a Translator's options name,
// in their canonical form.
const translatorLanguages = (given) => {
  const options = dictionary(given);
  for (const member of ["sourceLanguage", "targetLanguage"]) {
    if (options[member] === undefined) {
      throw new TypeError(`Failed to read the '${member}' property: Required member is undefined.`);
    }
  }
  return [languageTag(options.sourceLanguage), languageTag(options.targetLanguage)];
};

class LanguageDetector {
  constructor() { illegal(); }
  static availability(given) {
    return new Promise((resolve) => {
      resolve(detects(expectedLanguages(given)) ? "available" : "unavailable");
    });
  }
  static create(given) {
    return new Promise((resolve) => {
      const expected = expectedLanguages(given);
      const options = optionsOf(given);
      resolve(created("language-detector", options, () => detects(expected), () => {
        const made = Object.create(LanguageDetector.prototype);
        models.set(made, {
          expected: expected ? Object.freeze(expected) : null, destroyed: new AbortController(),
          quota: host("assistant", "quota"),
        });
        return made;
      }));
    });
  }
  get expectedInputLanguages() { return modelOf(this, LanguageDetector).expected; }
  get inputQuota() { return modelOf(this, LanguageDetector).quota; }
  measureInputUsage(input, given) {
    const state = modelOf(this, LanguageDetector);
    const text = String(input);
    return modelCall(state, optionsOf(given), (end) => {
      end(null, usageOf(text));
      return null;
    });
  }
  detect(input, given) {
    const state = modelOf(this, LanguageDetector);
    const text = String(input);
    return modelCall(state, optionsOf(given), (end) => {
      if (untranslatable(text)) {
        end(null, [{ detectedLanguage: "und", confidence: 1 }]);
        return null;
      }
      quotaChecked(state, text);
      return askModel("detect", "", "", text, () => {}, (error, named) => {
        if (error) {
          end(error);
          return;
        }
        // The model names one language, without a likelihood: it has
        // it all, and the unknown nothing; a text it names none for is
        // unknown.
        const found = named.map((detectedLanguage) => ({ detectedLanguage, confidence: 1 / named.length }));
        found.push({ detectedLanguage: "und", confidence: found.length ? 0 : 1 });
        end(null, found);
      });
    });
  }
  destroy() { destroyModel(this, new DOMException("The model was destroyed.", "AbortError")); }
}

const detects = (expected) => host("assistant", "available", "detect", "", "", ...(expected || []));
const expectedLanguages = (given) => {
  const options = dictionary(given);
  if (options.expectedInputLanguages === undefined) return null;
  const list = options.expectedInputLanguages;
  if (list === null || typeof list !== "object" || typeof list[Symbol.iterator] !== "function") {
    throw new TypeError("Failed to read the 'expectedInputLanguages' property: it is not a sequence.");
  }
  return [...new Set([...list].map(languageTag))];
};

// ---- ReadableStream: what translateStreaming() gives, as far as a page
// reads it: a reader, or for await.

const streams = new WeakMap();
const streamOf = (stream) => {
  const state = streams.get(stream);
  if (!state) throw new TypeError("Illegal invocation");
  return state;
};
// newStream is a stream start fills through its controller, and cancel
// is told when its reader gives up on it.
const newStream = (start, cancel) => {
  const stream = Object.create(ReadableStream.prototype);
  const state = {
    queue: [], reads: [], done: false, failed: false, reason: undefined, reader: null, cancel, closed: null,
  };
  state.closed = new Promise((resolve, reject) => { state.close = resolve; state.fail = reject; });
  state.closed.catch(() => {});
  streams.set(stream, state);
  const settle = () => {
    while (state.reads.length && state.queue.length) {
      state.reads.shift().resolve({ value: state.queue.shift(), done: false });
    }
    if (state.done && !state.queue.length) {
      for (const read of state.reads.splice(0)) read.resolve({ value: undefined, done: true });
    }
    if (state.failed) for (const read of state.reads.splice(0)) read.reject(state.reason);
  };
  state.settle = settle;
  const controller = {
    enqueue(chunk) {
      if (state.done || state.failed) return;
      state.queue.push(chunk);
      settle();
    },
    close() {
      if (state.done || state.failed) return;
      state.done = true;
      state.close();
      settle();
    },
    error(reason) {
      if (state.done || state.failed) return;
      state.failed = true;
      state.reason = reason;
      state.queue = [];
      state.fail(reason);
      settle();
    },
  };
  queueMicrotask(() => start(controller));
  return stream;
};
const cancelled = (state, reason) => {
  if (state.failed) return Promise.reject(state.reason);
  if (!state.done) {
    state.done = true;
    state.queue = [];
    state.close();
    state.settle();
    if (state.cancel) state.cancel(reason);
  }
  return Promise.resolve();
};

class ReadableStreamDefaultReader {
  constructor() { illegal(); }
  get closed() { return streamOf(this).closed; }
  read() {
    const state = streamOf(this);
    if (state.reader !== this) return Promise.reject(new TypeError("The reader was released."));
    return new Promise((resolve, reject) => {
      state.reads.push({ resolve, reject });
      state.settle();
    });
  }
  cancel(reason) {
    const state = streamOf(this);
    return state.reader === this ? cancelled(state, reason) : Promise.reject(new TypeError("The reader was released."));
  }
  releaseLock() {
    const state = streamOf(this);
    if (state.reader !== this) return;
    state.reader = null;
    for (const read of state.reads.splice(0)) read.reject(new TypeError("The reader was released."));
  }
}

class ReadableStream {
  constructor() { illegal(); }
  get locked() { return streamOf(this).reader !== null; }
  getReader(options) {
    const state = streamOf(this);
    if (dictionary(options).mode !== undefined) throw new TypeError("light's streams have no BYOB reader.");
    if (state.reader) throw new TypeError("The stream is locked to a reader.");
    const reader = Object.create(ReadableStreamDefaultReader.prototype);
    streams.set(reader, state);
    state.reader = reader;
    return reader;
  }
  cancel(reason) {
    const state = streamOf(this);
    return state.reader ? Promise.reject(new TypeError("The stream is locked to a reader.")) : cancelled(state, reason);
  }
  values(options) {
    const reader = this.getReader();
    const keep = Boolean(dictionary(options).preventCancel);
    return {
      next: () => reader.read().then((step) => {
        if (step.done) reader.releaseLock();
        return step;
      }),
      return: (value) => {
        const ended = keep ? Promise.resolve() : reader.cancel(value);
        return ended.then(() => {
          reader.releaseLock();
          return { value, done: true };
        });
      },
      [Symbol.asyncIterator]() { return this; },
    };
  }
  [Symbol.asyncIterator](options) { return this.values(options); }
}

for (const Class of [Translator, LanguageDetector, CreateMonitor, ReadableStream, ReadableStreamDefaultReader]) {
  Object.defineProperty(Class.prototype, Symbol.toStringTag, { value: Class.name, configurable: true });
}
