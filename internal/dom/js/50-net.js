// ---- requests

class Headers {
  constructor(init) {
    this._map = new Map();
    if (init instanceof Headers) init.forEach((value, name) => this.append(name, value));
    else if (Array.isArray(init)) for (const [name, value] of init) this.append(name, value);
    else if (init && typeof init === "object") for (const name of Object.keys(init)) this.append(name, init[name]);
  }
  append(name, value) {
    name = String(name).toLowerCase();
    this._map.set(name, this._map.has(name) ? `${this._map.get(name)}, ${value}` : String(value));
  }
  set(name, value) { this._map.set(String(name).toLowerCase(), String(value)); }
  get(name) { const value = this._map.get(String(name).toLowerCase()); return value === undefined ? null : value; }
  has(name) { return this._map.has(String(name).toLowerCase()); }
  delete(name) { this._map.delete(String(name).toLowerCase()); }
  forEach(fn, self) { for (const [name, value] of this._map) fn.call(self, value, name, this); }
  keys() { return this._map.keys(); }
  values() { return this._map.values(); }
  entries() { return this._map.entries(); }
  [Symbol.iterator]() { return this._map.entries(); }
}

// bytesOf is what a part of a body is made of: a blob's bytes, a
// buffer's, a view's, and a text's in UTF-8.
const bytesOf = (part) => (part instanceof Blob ? part._bytes
  : part instanceof ArrayBuffer ? new Uint8Array(part)
    : ArrayBuffer.isView(part) ? new Uint8Array(part.buffer, part.byteOffset, part.byteLength)
      : utf8.encode(String(part)));
const joined = (chunks) => {
  const all = new Uint8Array(chunks.reduce((size, chunk) => size + chunk.length, 0));
  let at = 0;
  for (const chunk of chunks) {
    all.set(chunk, at);
    at += chunk.length;
  }
  return all;
};

class Blob {
  constructor(parts = [], options = {}) {
    define(this, { _bytes: joined(Array.from(parts, bytesOf)) });
    this.type = String((options && options.type) || "").toLowerCase();
  }
  get size() { return this._bytes.length; }
  text() { return Promise.resolve(utf8.decode(this._bytes)); }
  arrayBuffer() { return Promise.resolve(this._bytes.slice().buffer); }
  bytes() { return Promise.resolve(this._bytes.slice()); }
  slice(from = 0, to = this.size, type = "") { return new Blob([this._bytes.slice(from, to)], { type }); }
}
class File extends Blob {
  constructor(parts, name, options = {}) {
    super(parts, options);
    this.name = String(name);
    this.lastModified = options.lastModified || Date.now();
  }
}
class FormData {
  constructor(form) {
    this._list = [];
    if (form instanceof HTMLFormElement) {
      for (const field of form.elements) {
        if (!field.name || field.disabled || field instanceof HTMLButtonElement) continue;
        if (field instanceof HTMLInputElement && /^(checkbox|radio)$/.test(field.type) && !field.checked) continue;
        this._list.push([field.name, field.value]);
      }
    }
  }
  append(name, value, filename) {
    if (!(value instanceof Blob)) value = String(value);
    else if (filename !== undefined || !(value instanceof File)) {
      value = new File([value], filename === undefined ? "blob" : filename, { type: value.type });
    }
    this._list.push([String(name), value]);
  }
  set(name, value, filename) { this.delete(name); this.append(name, value, filename); }
  delete(name) { this._list = this._list.filter(([n]) => n !== String(name)); }
  get(name) { const found = this._list.find(([n]) => n === String(name)); return found ? found[1] : null; }
  getAll(name) { return this._list.filter(([n]) => n === String(name)).map(([, v]) => v); }
  has(name) { return this._list.some(([n]) => n === String(name)); }
  forEach(fn, self) { for (const [name, value] of this._list) fn.call(self, value, name, this); }
  keys() { return this._list.map(([n]) => n)[Symbol.iterator](); }
  values() { return this._list.map(([, v]) => v)[Symbol.iterator](); }
  entries() { return this._list.map((pair) => pair.slice())[Symbol.iterator](); }
  [Symbol.iterator]() { return this.entries(); }
}

// multipart is a FormData as multipart/form-data writes it, between
// lines of a boundary made for it.
const multipart = (data) => {
  let boundary = "----lightFormBoundary";
  for (let i = 0; i < 16; i++) boundary += "0123456789abcdefghijklmnopqrstuvwxyz"[Math.floor(Math.random() * 36)];
  const lines = (text) => String(text).replace(/\r\n|\r|\n/g, "\r\n");
  const named = (text) => lines(text).replace(/\r/g, "%0D").replace(/\n/g, "%0A").replace(/"/g, "%22");
  const chunks = [];
  for (const [name, value] of data._list) {
    const head = `--${boundary}\r\nContent-Disposition: form-data; name="${named(name)}"`;
    if (value instanceof Blob) {
      chunks.push(utf8.encode(`${head}; filename="${named(value.name)}"\r\nContent-Type: ${value.type || "application/octet-stream"}\r\n\r\n`),
        value._bytes, utf8.encode("\r\n"));
    } else chunks.push(utf8.encode(`${head}\r\n\r\n${lines(value)}\r\n`));
  }
  chunks.push(utf8.encode(`--${boundary}--\r\n`));
  return { bytes: joined(chunks), type: `multipart/form-data; boundary=${boundary}` };
};

// carried is what a request sends of the body a script gave it: its
// bytes, and the Content-Type it has unless the script names one.
const carried = (payload) => {
  if (payload === undefined || payload === null) return { bytes: null, type: null };
  if (typeof payload === "string") return { bytes: utf8.encode(payload), type: "text/plain;charset=UTF-8" };
  if (payload instanceof URLSearchParams) {
    return { bytes: utf8.encode(payload.toString()), type: "application/x-www-form-urlencoded;charset=UTF-8" };
  }
  if (payload instanceof FormData) return multipart(payload);
  if (payload instanceof Blob) return { bytes: payload._bytes, type: payload.type || null };
  if (payload instanceof ArrayBuffer || ArrayBuffer.isView(payload)) return { bytes: bytesOf(payload).slice(), type: null };
  if (typeof payload.getReader === "function") throw new TypeError("light sends no stream as the body of a request.");
  return { bytes: utf8.encode(String(payload)), type: "text/plain;charset=UTF-8" };
};

// sentAs is a method as a request is sent with it: the common ones in
// capitals. One that is no method, and one no script may send, are
// thrown as fail makes them.
const sentAs = (method, fail) => {
  method = String(method);
  if (!/^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/.test(method)) throw fail(`'${method}' is not a valid HTTP method.`, "SyntaxError");
  if (/^(connect|trace|track)$/i.test(method)) throw fail(`'${method}' HTTP method is unsupported.`, "SecurityError");
  return /^(delete|get|head|options|post|put)$/i.test(method) ? method.toUpperCase() : method;
};
const oneOf = (value, known, what) => {
  if (!known.includes(value)) throw new TypeError(`'${value}' is not a request's ${what}.`);
  return value;
};
// keptAlive is the most a request that outlives its page carries.
const keptAlive = 64 * 1024;

// body gives what reads an answer's or a request's body: its bytes as
// they came, and the text they are in UTF-8 whatever the answer says of
// its encoding. A body is read once.
const body = (prototype) => define(prototype, {
  _read() {
    if (this.bodyUsed) return Promise.reject(new TypeError("The body was already read."));
    if (this._bytes) this.bodyUsed = true;
    return Promise.resolve(this._bytes || new Uint8Array(0));
  },
  arrayBuffer() { return this._read().then((bytes) => bytes.slice().buffer); },
  bytes() { return this._read().then((bytes) => bytes.slice()); },
  blob() { return this._read().then((bytes) => new Blob([bytes], { type: this.headers.get("content-type") || "" })); },
  text() { return this._read().then((bytes) => utf8.decode(bytes)); },
  json() { return this.text().then(JSON.parse); },
  formData() {
    return this.text().then((text) => {
      if (!/^application\/x-www-form-urlencoded\b/i.test(this.headers.get("content-type") || "")) {
        throw new TypeError("light reads a form from an application/x-www-form-urlencoded body only.");
      }
      const data = new FormData();
      new URLSearchParams(text).forEach((value, name) => data.append(name, value));
      return data;
    });
  },
});

class Request {
  constructor(input, init = {}) {
    const from = input instanceof Request ? input : null;
    init = init || {};
    if (from) this.url = from.url;
    else {
      const to = parseURL(input, document.baseURI);
      if (to.username || to.password) throw new TypeError(`Request cannot be constructed from a URL that includes credentials: ${input}`);
      this.url = to.href;
    }
    this.method = init.method === undefined ? (from ? from.method : "GET") : sentAs(init.method, (why) => new TypeError(why));
    this.headers = new Headers(init.headers || (from ? from.headers : undefined));
    this.signal = init.signal || (from ? from.signal : null);
    this.mode = oneOf(init.mode || (from ? from.mode : "cors"), ["cors", "no-cors", "same-origin"], "mode");
    this.credentials = oneOf(init.credentials || (from ? from.credentials : "same-origin"), ["omit", "same-origin", "include"], "credentials");
    this.cache = init.cache || (from ? from.cache : "default");
    this.redirect = oneOf(init.redirect || (from ? from.redirect : "follow"), ["follow", "error", "manual"], "redirect");
    this.keepalive = Boolean(init.keepalive === undefined ? from && from.keepalive : init.keepalive);
    this.referrer = "about:client";
    this.referrerPolicy = "";
    this.bodyUsed = false;
    if (this.mode === "no-cors" && !/^(GET|HEAD|POST)$/.test(this.method)) throw new TypeError(`'${this.method}' is unsupported in no-cors mode.`);
    const sent = init.body === undefined || init.body === null ? { bytes: from ? from._bytes : null, type: null } : carried(init.body);
    if (sent.bytes && /^(GET|HEAD)$/.test(this.method)) throw new TypeError("Request with GET/HEAD method cannot have body.");
    define(this, { _bytes: sent.bytes });
    if (sent.type && !this.headers.has("content-type")) this.headers.set("content-type", sent.type);
  }
  clone() {
    if (this.bodyUsed) throw new TypeError("The body was already read.");
    return new Request(this);
  }
}
body(Request.prototype);

class Response {
  constructor(content = null, init = {}) {
    init = init || {};
    const held = carried(content);
    define(this, { _bytes: held.bytes });
    this.status = init.status === undefined ? 200 : init.status;
    this.statusText = init.statusText || "";
    this.headers = new Headers(init.headers);
    if (held.type && !this.headers.has("content-type")) this.headers.set("content-type", held.type);
    this.url = init.url || "";
    this.redirected = Boolean(init.redirected);
    this.type = init.type || "default";
    this.bodyUsed = false;
  }
  get ok() { return this.status >= 200 && this.status < 300; }
  get body() { return null; }
  clone() {
    if (this.bodyUsed) throw new TypeError("The body was already read.");
    return new Response(this._bytes, { status: this.status, statusText: this.statusText, headers: this.headers, url: this.url, redirected: this.redirected, type: this.type });
  }
  static error() { return new Response(null, { status: 0, type: "error" }); }
  static json(data, init = {}) {
    return new Response(JSON.stringify(data), { ...init, headers: { "content-type": "application/json", ...(init.headers || {}) } });
  }
  static redirect(url, status = 302) { return new Response(null, { status, headers: { location: url } }); }
}
body(Response.prototype);

// asked are the requests on their way, by the number light knows each
// by: answered tells one how it ended.
let lastRequest = 0;
const asked = new Map();
const ask = (request, api, settle) => {
  const id = ++lastRequest;
  asked.set(id, settle);
  const headers = {};
  request.headers.forEach((value, name) => { headers[name] = value; });
  host("fetch", id, {
    api, method: request.method, url: request.url, headers,
    mode: request.mode, credentials: request.credentials, redirect: request.redirect, keepalive: request.keepalive,
  }, new Error().stack || "", request._bytes ? shared(request._bytes) : null);
  return id;
};
const answered = (id, failure, status, statusText, url, headers, size, type, redirected) => {
  const settle = asked.get(id);
  asked.delete(id);
  if (!settle) return;
  const landed = new SharedBuffer(size);
  if (size) host("body", landed);
  settle(failure, { status, statusText, url, headers, bytes: new Uint8Array(landed).slice(), type, redirected });
};

const fetch = (input, init) => new Promise((resolve, reject) => {
  const request = new Request(input, init);
  const signal = request.signal;
  if (signal && signal.aborted) {
    reject(signal.reason);
    return;
  }
  if (request.keepalive && request._bytes && request._bytes.length > keptAlive) {
    reject(new TypeError("Failed to fetch"));
    return;
  }
  const id = ask(request, "fetch", (failure, answer) => {
    if (failure) {
      reject(new TypeError("Failed to fetch"));
      return;
    }
    resolve(new Response(answer.bytes, {
      status: answer.status, statusText: answer.statusText, headers: answer.headers, url: answer.url,
      redirected: answer.redirected, type: answer.type,
    }));
  });
  if (signal) {
    signal.addEventListener("abort", () => {
      if (!asked.has(id)) return;
      asked.delete(id);
      host("abortFetch", id);
      reject(signal.reason);
    });
  }
});

// beacon is navigator.sendBeacon: a POST that outlives its page, whose
// answer nobody reads. It says whether the request was taken.
const beacon = (url, data) => {
  const to = parseURL(url, document.baseURI);
  if (!/^https?:$/.test(to.protocol)) throw new TypeError("Failed to execute 'sendBeacon' on 'Navigator': Beacons are only supported over HTTP(S).");
  const request = new Request(to.href, { method: "POST", body: data, credentials: "include", keepalive: true });
  if (request._bytes && request._bytes.length > keptAlive) return false;
  ask(request, "fetch", () => {});
  return true;
};

class XMLHttpRequestEventTarget extends EventTarget {}
withHandlers(XMLHttpRequestEventTarget.prototype, ["loadstart", "progress", "abort", "error", "load", "timeout", "loadend", "readystatechange"]);
class XMLHttpRequestUpload extends XMLHttpRequestEventTarget {}
class XMLHttpRequest extends XMLHttpRequestEventTarget {
  constructor() {
    super();
    this.readyState = 0;
    this.status = 0;
    this.statusText = "";
    this.responseURL = "";
    this.timeout = 0;
    this.withCredentials = false;
    this.upload = new XMLHttpRequestUpload();
    this._headers = new Headers();
    this._answer = new Headers();
    this._id = 0;
    this._sent = false;
    this._timer = 0;
    this._type = "";
    this._mime = null;
    this._bytes = null;
    this._read = {};
  }
  _state(state) {
    this.readyState = state;
    dispatch(this, new Event("readystatechange"));
  }
  _blank() {
    this.status = 0;
    this.statusText = "";
    this.responseURL = "";
    this._answer = new Headers();
    this._bytes = null;
    this._read = {};
  }
  open(method, url, asynchronous = true) {
    method = sentAs(method, (why, name) => new DOMException(why, name));
    if (asynchronous === false) throw new DOMException("light makes no synchronous request.", "NotSupportedError");
    const to = parseURL(url, document.baseURI).href;
    this._drop();
    this._method = method;
    this._url = to;
    this._headers = new Headers();
    this._blank();
    this._state(1);
  }
  setRequestHeader(name, value) {
    if (this.readyState !== 1 || this._sent) throw new DOMException("The request is not opened.", "InvalidStateError");
    this._headers.append(name, value);
  }
  overrideMimeType(mime) {
    if (this.readyState >= 3) throw new DOMException("The answer is already being read.", "InvalidStateError");
    this._mime = String(mime);
  }
  get responseType() { return this._type; }
  set responseType(type) {
    if (this.readyState >= 3) throw new DOMException("The answer is already being read.", "InvalidStateError");
    if (["", "text", "arraybuffer", "blob", "json", "document"].includes(type)) this._type = type;
  }
  // _kind is the type the answer is read as: the one the script gave
  // in place of the answer's own, else that one.
  get _kind() { return this._mime === null ? this._answer.get("content-type") || "" : this._mime; }
  // _text is the answer decoded: in the encoding its byte order mark
  // tells of, else the one its type names, else UTF-8.
  get _text() {
    if (!this._bytes) return "";
    if (this._read.text === undefined) {
      const named = /;\s*charset\s*=\s*"?([^\s;"]+)/i.exec(this._kind);
      this._read.text = utf8.decode(this._bytes, named ? named[1] : "utf-8");
    }
    return this._read.text;
  }
  get responseText() {
    if (this._type !== "" && this._type !== "text") {
      throw new DOMException(`The answer is read as '${this._type}', not as text.`, "InvalidStateError");
    }
    return this._text;
  }
  send(payload) {
    if (this.readyState !== 1 || this._sent) throw new DOMException("The request is not opened.", "InvalidStateError");
    const request = new Request(this._url, {
      method: this._method, headers: this._headers, body: /^(GET|HEAD)$/.test(this._method) ? null : payload,
      credentials: this.withCredentials ? "include" : "same-origin",
    });
    this._sent = true;
    dispatch(this, new ProgressEvent("loadstart"));
    if (!this._sent) return;
    if (this.timeout > 0) this._timer = setTimeout(() => this._end("timeout"), this.timeout);
    this._id = ask(request, "XMLHttpRequest", (failure, answer) => {
      this._id = 0;
      if (failure) {
        this._end("error");
        return;
      }
      clearTimeout(this._timer);
      this._sent = false;
      this.status = answer.status;
      this.statusText = answer.statusText;
      this.responseURL = answer.url;
      this._answer = new Headers(answer.headers);
      this._state(2);
      this._state(3);
      this._bytes = answer.bytes;
      this._state(4);
      const progress = { lengthComputable: true, loaded: answer.bytes.length, total: answer.bytes.length };
      dispatch(this, new ProgressEvent("load", progress));
      dispatch(this, new ProgressEvent("loadend", progress));
    });
  }
  // _drop forgets the request on its way, which light then stops.
  _drop() {
    clearTimeout(this._timer);
    this._sent = false;
    if (!this._id) return;
    asked.delete(this._id);
    host("abortFetch", this._id);
    this._id = 0;
  }
  // _end ends a request that got no answer: how is error, timeout or
  // abort.
  _end(how) {
    this._drop();
    this._blank();
    this._state(4);
    dispatch(this, new ProgressEvent(how));
    dispatch(this, new ProgressEvent("loadend"));
  }
  abort() {
    if (this._sent) this._end("abort");
    if (this.readyState === 4) {
      this._blank();
      this.readyState = 0;
    }
  }
  // response is the answer as the type asked for: the same value each
  // time, null until it is whole for all but text, and for what is no
  // JSON.
  get response() {
    if (this._type === "" || this._type === "text") return this._text;
    if (this.readyState !== 4 || !this._bytes) return null;
    if (!("response" in this._read)) {
      const made = {
        json: () => { try { return JSON.parse(utf8.decode(this._bytes)); } catch (_) { return null; } },
        arraybuffer: () => this._bytes.slice().buffer,
        blob: () => new Blob([this._bytes], { type: this._kind }),
        document: () => new DOMParser().parseFromString(this._text),
      };
      this._read.response = made[this._type]();
    }
    return this._read.response;
  }
  get responseXML() { return null; }
  getResponseHeader(name) { return this._answer.get(name); }
  getAllResponseHeaders() {
    let all = "";
    this._answer.forEach((value, name) => { all += `${name}: ${value}\r\n`; });
    return all;
  }
}
const requestStates = { UNSENT: 0, OPENED: 1, HEADERS_RECEIVED: 2, LOADING: 3, DONE: 4 };
define(XMLHttpRequest, requestStates);
define(XMLHttpRequest.prototype, requestStates);
