// ---- document.fonts: the fonts the page's text waits for

// FontFaceSet is document.fonts as far as light has it: whether the
// fonts of the page's sheets have come, asking for those a text needs,
// and the events of their loading. It lists no FontFace.
class FontFaceSet extends EventTarget {
  get status() { return host("fontsLoading") > 0 ? "loading" : "loaded"; }
  get ready() {
    if (!fontsReady) {
      let done;
      const promise = new Promise((resolve) => { done = resolve; });
      fontsReady = { promise, done, settled: false };
      if (host("fontsLoading") === 0) settleFonts();
    }
    return fontsReady.promise;
  }
  get size() { return 0; }
  check(font, text = " ") { return host("fontsCheck", String(font), String(text)); }
  load(font, text = " ") {
    font = String(font);
    text = String(text);
    try {
      if (host("fontsLoad", font, text)) return Promise.resolve([]);
    } catch (error) {
      return Promise.reject(error);
    }
    return new Promise((resolve) => { fontLoads.push({ font, text, resolve }); });
  }
  forEach() {}
  has() { return false; }
  values() { return [][Symbol.iterator](); }
  keys() { return this.values(); }
  entries() { return this.values(); }
  [Symbol.iterator]() { return this.values(); }
}
withHandlers(FontFaceSet.prototype, ["loading", "loadingdone", "loadingerror"]);

const fontSet = new FontFaceSet();
let fontsReady = null, fontLoads = [];
const settleFonts = () => {
  if (fontsReady && !fontsReady.settled) {
    fontsReady.settled = true;
    fontsReady.done(fontSet);
  }
};

// fontsMoved is told how many fonts are on their way, each frame that
// one is or that the last has just landed.
const fontsMoved = (loading, was) => {
  if (loading > 0 && was === 0) {
    if (fontsReady && fontsReady.settled) fontsReady = null;
    dispatch(fontSet, new Event("loading"));
  }
  fontLoads = fontLoads.filter((asked) => {
    if (!host("fontsCheck", asked.font, asked.text)) return true;
    asked.resolve([]);
    return false;
  });
  if (loading === 0) {
    settleFonts();
    if (was > 0) dispatch(fontSet, new Event("loadingdone"));
  }
  return fontLoads.length;
};

Object.defineProperty(Document.prototype, "fonts", { get() { return fontSet; }, configurable: true });
define(window, { FontFaceSet });
