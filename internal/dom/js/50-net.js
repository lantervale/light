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

class Blob {
  constructor(parts = [], options = {}) {
    this._text = parts.map((part) => (part instanceof Blob ? part._text : typeof part === "string" ? part
      : ArrayBuffer.isView(part) || part instanceof ArrayBuffer ? new TextDecoder().decode(part) : String(part))).join("");
    this.type = options.type || "";
  }
  get size() { return utf8.encode(this._text).length; }
  text() { return Promise.resolve(this._text); }
  arrayBuffer() { return Promise.resolve(utf8.encode(this._text).buffer); }
  slice(from, to, type) { return new Blob([this._text.slice(from, to)], { type }); }
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
  append(name, value) { this._list.push([String(name), value instanceof Blob ? value : String(value)]); }
  set(name, value) { this.delete(name); this.append(name, value); }
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

// body gives what reads an answer's or a request's text.
const body = (prototype) => define(prototype, {
  text() {
    if (this.bodyUsed) return Promise.reject(new TypeError("The body was already read."));
    this.bodyUsed = true;
    return Promise.resolve(this._body);
  },
  json() { return this.text().then(JSON.parse); },
  arrayBuffer() { return this.text().then((text) => utf8.encode(text).buffer); },
  blob() { return this.text().then((text) => new Blob([text], { type: this.headers.get("content-type") || "" })); },
  formData() { return this.text().then((text) => { const data = new FormData(); new URLSearchParams(text).forEach((v, n) => data.append(n, v)); return data; }); },
});

class Request {
  constructor(input, init = {}) {
    const from = input instanceof Request ? input : null;
    this.url = from ? from.url : parseURL(input, document.baseURI).href;
    this.method = String(init.method || (from ? from.method : "GET")).toUpperCase();
    this.headers = new Headers(init.headers || (from ? from.headers : undefined));
    this._body = init.body === undefined || init.body === null ? (from ? from._body : "") : String(init.body);
    this.signal = init.signal || (from ? from.signal : null);
    this.mode = init.mode || "cors";
    this.credentials = init.credentials || "same-origin";
    this.cache = init.cache || "default";
    this.redirect = init.redirect || "follow";
    this.referrer = "";
    this.bodyUsed = false;
  }
  clone() { return new Request(this); }
}
body(Request.prototype);

class Response {
  constructor(text = "", init = {}) {
    this._body = text === null ? "" : String(text);
    this.status = init.status === undefined ? 200 : init.status;
    this.statusText = init.statusText || "";
    this.headers = new Headers(init.headers);
    this.url = init.url || "";
    this.redirected = Boolean(init.redirected);
    this.type = init.type || "default";
    this.bodyUsed = false;
  }
  get ok() { return this.status >= 200 && this.status < 300; }
  get body() { return null; }
  clone() {
    return new Response(this._body, { status: this.status, statusText: this.statusText, headers: this.headers, url: this.url, redirected: this.redirected, type: this.type });
  }
  static error() { return new Response("", { status: 0, type: "error" }); }
  static json(data, init = {}) {
    return new Response(JSON.stringify(data), { ...init, headers: { "content-type": "application/json", ...(init.headers || {}) } });
  }
  static redirect(url, status = 302) { return new Response("", { status, headers: { location: url } }); }
}
body(Response.prototype);

// asked are the requests on their way, by the number light knows each
// by: answered tells one how it ended.
let lastRequest = 0;
const asked = new Map();
const ask = (request, settle) => {
  const id = ++lastRequest;
  asked.set(id, settle);
  const headers = {};
  request.headers.forEach((value, name) => { headers[name] = value; });
  host("fetch", id, request.method, request.url, headers, new Error().stack || "");
  return id;
};
const answered = (id, failure, status, statusText, url, headers, text) => {
  const settle = asked.get(id);
  asked.delete(id);
  if (settle) settle(failure, { status, statusText, url, headers, text });
};

const fetch = (input, init) => new Promise((resolve, reject) => {
  const request = new Request(input, init);
  const signal = request.signal;
  if (signal && signal.aborted) {
    reject(signal.reason);
    return;
  }
  const id = ask(request, (failure, answer) => {
    if (failure) {
      reject(new TypeError("Failed to fetch: " + failure));
      return;
    }
    resolve(new Response(answer.text, {
      status: answer.status, statusText: answer.statusText, headers: answer.headers, url: answer.url,
      redirected: answer.url !== request.url, type: "basic",
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

class XMLHttpRequestEventTarget extends EventTarget {}
withHandlers(XMLHttpRequestEventTarget.prototype, ["loadstart", "progress", "abort", "error", "load", "timeout", "loadend", "readystatechange"]);
class XMLHttpRequestUpload extends XMLHttpRequestEventTarget {}
class XMLHttpRequest extends XMLHttpRequestEventTarget {
  constructor() {
    super();
    this.readyState = 0;
    this.status = 0;
    this.statusText = "";
    this.responseText = "";
    this.responseURL = "";
    this.responseType = "";
    this.timeout = 0;
    this.withCredentials = false;
    this.upload = new XMLHttpRequestUpload();
    this._headers = new Headers();
    this._answer = new Headers();
    this._id = 0;
  }
  _state(state) {
    this.readyState = state;
    dispatch(this, new Event("readystatechange"));
  }
  open(method, url, asynchronous = true) {
    if (asynchronous === false) throw new DOMException("light makes no synchronous request.", "NotSupportedError");
    this._method = String(method).toUpperCase();
    this._url = parseURL(url, document.baseURI).href;
    this._headers = new Headers();
    this._state(1);
  }
  setRequestHeader(name, value) { this._headers.append(name, value); }
  overrideMimeType() {}
  send(payload) {
    if (this.readyState !== 1) throw new DOMException("The request is not opened.", "InvalidStateError");
    const request = new Request(this._url, { method: this._method, headers: this._headers, body: payload });
    dispatch(this, new ProgressEvent("loadstart"));
    this._id = ask(request, (failure, answer) => {
      this._id = 0;
      if (failure) {
        this._state(4);
        dispatch(this, new ProgressEvent("error"));
        dispatch(this, new ProgressEvent("loadend"));
        return;
      }
      this.status = answer.status;
      this.statusText = answer.statusText;
      this.responseURL = answer.url;
      this._answer = new Headers(answer.headers);
      this._state(2);
      this._state(3);
      this.responseText = answer.text;
      this._state(4);
      const progress = { lengthComputable: true, loaded: answer.text.length, total: answer.text.length };
      dispatch(this, new ProgressEvent("load", progress));
      dispatch(this, new ProgressEvent("loadend", progress));
    });
  }
  abort() {
    if (!this._id) return;
    asked.delete(this._id);
    host("abortFetch", this._id);
    this._id = 0;
    this.readyState = 0;
    dispatch(this, new ProgressEvent("abort"));
    dispatch(this, new ProgressEvent("loadend"));
  }
  get response() {
    if (this.readyState !== 4) return this.responseType === "" || this.responseType === "text" ? this.responseText : null;
    switch (this.responseType) {
      case "json":
        try { return JSON.parse(this.responseText); } catch (_) { return null; }
      case "arraybuffer": return utf8.encode(this.responseText).buffer;
      case "blob": return new Blob([this.responseText]);
      case "document": return new DOMParser().parseFromString(this.responseText);
      default: return this.responseText;
    }
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
