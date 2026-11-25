# light: reference

How each part behaves, and where it differs from other browsers. The short version is the [README](../README.md).

## Using it

Type in the address field, or in the field of the start page:

| Typed | Opened |
|-------|--------|
| An address with its scheme (`https://…`, `light://…`), `about:blank` | As it is |
| A host name with a dot and no space (`example.com`, `example.com:8080/path`) | `https://` and what was typed |
| `localhost`, a name ending in `.localhost`, an IP address | `http://` and what was typed |
| `view-source:` and an address | The source of that page |
| Anything else | A search, with DuckDuckGo unless the settings name light's own pages |

Around the page: tabs in the title bar (macOS), several windows, back, forward, reload and stop, bookmarks in
folders (the star of the address field), a history by day, downloads, a menu (new tab, new window, bookmarks,
history, downloads, zoom, full screen, find in page, save page as, developer tools, settings), a menu of the
right click, zoom from 80 % to 200 %, text you can select and copy, and developer tools (Inspector, Network with
HAR export, Console, Warnings, Storage). light's own pages are light or dark as the system is, and change with it;
a click on the logo of the start page sets the other one than shown, whatever the system's, and the settings
bring the system's back. Pages of the web stay as their sites made them.

### Shortcuts

Cmd is Command on macOS and Control elsewhere.

| Keys | What they do |
|------|--------------|
| Cmd+T, Cmd+W | A new tab, close the tab (a window's last tab closes the window) |
| Cmd+N | A new window, on the start page |
| Cmd+Q | Quit, whatever the windows open |
| Cmd+L | The address field |
| Cmd+R | Reload, or stop a page that is loading |
| Cmd+[ and Cmd+] (Alt+Left and Alt+Right too, off macOS) | Back, forward |
| Cmd+D | Bookmark this page |
| Cmd+F | Find in page |
| Cmd+Y on macOS, Ctrl+H elsewhere | The history |
| Cmd+Shift+J on macOS, Ctrl+J elsewhere | The downloads |
| Cmd+, | The settings |
| Cmd++, Cmd+-, Cmd+0 | Zoom in, zoom out, the actual size |
| Cmd+Alt+U on macOS, Ctrl+U elsewhere | The page's source, in a new tab |
| F12, or Cmd+Alt+I on macOS, Ctrl+Shift+I elsewhere | The developer tools |
| Alt+Up and Alt+Down (Ctrl+P and Ctrl+N too, on macOS), in the console's line | What was typed before, and after |
| Right click | The menu of what is under the pointer; arrows, Enter and Escape work in it |
| Cmd+click or a middle click on a link | The link in a new tab, behind the shown one |
| Cmd+Shift+click or Shift with a middle click on a link | The link in a new tab, shown |
| Shift+click on a link | The link in a new window |
| Middle click on a tab | Close the tab |

### light's own pages

| Address | What it is |
|---|---|
| `light://home` | The start page: a search field and the pages light knows |
| `light://bookmarks` | The bookmarks, by folder |
| `light://history` | Every page visited, by day, filtered as you type |
| `light://downloads` | The files saved, in progress and done |
| `light://config` | The settings; a change applies at once ([Settings](#settings)) |
| `light://about` | What light is, where its files are, and what it changes in a page before showing it |
| `light://scripts` | A page that shows what light runs of JavaScript |
| `light://categories` | The page categories, edited; there only while the model helps with them ([A model on this device](model.md)) |
| `light://guide`, `light://examples` | Pages about ember's ui and its examples |

### Command line

`light [address]` (or `light browse [address]`) is the browser; `light drive` is the one a test runner drives
([Driving it](development.md#driving-it)). `light --help` lists the options: `--dir` (or `LIGHT_DIR`) names the folder light
keeps its files in, `--downloads` (or `LIGHT_DOWNLOADS`) the folder files are saved to.

## What works and what does not

What works, in short: pages over `http` and `https` with their stylesheets, pictures (PNG, JPEG, GIF, WebP, SVG)
and web fonts (TrueType, OpenType, WOFF, WOFF2); forms sent by GET and POST; cookies by the rules of RFC 6265bis;
frames, another site's included, each origin in its own V8 isolate; classic scripts and modules with import
maps; `fetch` and `XMLHttpRequest` under the CORS and mixed content rules; `localStorage`, `sessionStorage` and
IndexedDB; `postMessage`, `MessageChannel` and `BroadcastChannel`; XPath 1.0; XHTML; `alert`, `confirm` and
`prompt`; a prompt for a script that runs too long; downloads; view source.

What does not, in short:

- **No JavaScript in a build without the `v8` tag**, nor on a system V8 is not built for (it is for macOS and Linux,
  x86-64 and arm64, and Windows x86-64).
- **No canvas**: `getContext()` answers `null`, for 2D and WebGL alike. No audio or video: media files are
  downloaded.
- **No workers, no service workers, no WebSockets, no EventSource.**
- **No shadow trees and no custom elements** (`attachShadow` throws; `customElements` keeps names and upgrades
  nothing).
- **No HTTP cache**: every load asks for everything again.
- **No sign-in by HTTP authentication** (401, 407), no file upload, no `multipart/form-data` form.
- **No Content Security Policy**: a page's scripts run whatever its policy says. Only `frame-ancestors` and
  `X-Frame-Options` are honoured.
- **No `window.open`**, no `beforeunload`, no `document.write`.
- **No PDF viewer, no AVIF**: they are downloaded. An animated WebP is not drawn.
- **No extensions, no sync, no private window, no password manager, no autofill, no printing.**
- **CSS is a subset.** ember's guide lists what is missing:
  [What is missing](https://github.com/lantervale/ember/blob/main/docs/05-ui/10-missing.md).
- **One process.** Every tab and frame runs in one process, and V8 isolates are the only wall between origins.
  Downloads are not scanned. Do not keep in light what a hostile page must never reach
  ([the boundary and its limits](#scripts-in-frames)).
- **Tabs in the title bar are macOS only.**

light runs a subset of the [web-platform-tests](https://web-platform-tests.org) (`task wpt`): `dom/`,
`domxpath/`, `url/`, `domparsing/` and `IndexedDB/` whole, and parts of `html`, `css`, `encoding` and `fetch`, but the
tests that need what light does not have. On 20 November 2026, 2,876 of those 6,791 tests passed whole, and 147,240
of their 204,204 subtests ([the result](#the-result-of-20-november-2026)): a figure of this subset, not of the
web-platform-tests.

The full lists are below: [What it does not do](#what-it-does-not-do) and
[Limits of its JavaScript](#limits-of-its-javascript).

### The address field and search

What is typed in the address field, in the start page's field or after `light` on the command line is read the
same way (`pkg/tabs`):

| Typed | Opened |
|-------|--------|
| An address with its scheme (`https://…`, `light://…`), `about:blank` | As it is |
| `view-source:` and any of the rest | The source of what the rest opens |
| `localhost`, a name ending in `.localhost`, an IP address (`127.0.0.1`, `[::1]`), with a port and a path or not | `http://` and what was typed |
| A host name with a dot and no space (`example.com`, `example.com:8080/path`, `example.com.`) | `https://` and what was typed |
| Anything else: a word, several, a name with a space after it (`foo.bar baz`) | A search for it |

A search goes to the engine the [settings](#settings) name. **DuckDuckGo** is the default; the other choice is
light's own pages (`light://search?q=…`, which stays an address whatever the engine). The table of engines is
`pkg/search`, a row each.

- Nothing leaves light while you type: no suggestions asked of anybody, no page fetched ahead. The words are sent
  on Enter.
- While you type in the address field, a list under it offers what Enter will do ("Search DuckDuckGo: …" or
  "Go to …"), then the pages of your history and your bookmarks whose title or address holds every word typed, the
  most visited first. It is made of what this device holds alone (`pkg/complete`).
- Alt with Down or Up moves through the list (the arrows alone are the field's own), Enter takes the highlighted
  row (what was typed when none is), a click takes a row, Escape closes the list and keeps the text.
- The request is a navigation like any address typed, with the headers of [any other](#what-light-tells-a-site):
  `https://duckduckgo.com/?q=` and the words, with no other parameter and no identifier of light's.
- Where no script runs (a build without the `v8` tag, or JavaScript off in the settings), DuckDuckGo is asked for
  the page it has for such browsers, `https://html.duckduckgo.com/html/?q=`, which light shows with its results.
- The engine's mark stands in the address field while words are typed, in the start page's field and beside the
  choice in the settings: the icon of the engine's site, kept from a page of it shown in this run, and a magnifier
  until then. Nothing is fetched for it, and light ships no logo of DuckDuckGo's. light's own pages show light's.
- With text selected, the right click's menu has "Search DuckDuckGo for “…”", which opens the results in a tab of
  their own.
- A word alone is a search, so a host of your network without a dot wants its scheme: `http://nas/`.

### Loading a page

A tab that shows a page keeps showing it while the next one loads: no empty page comes between the two.

- **What is held.** A link, the address field, a form, a script's move, Back and Forward to a page light
  fetches again, and Reload leave the page shown as it is, alive: it scrolls, its links and its scripts work, its
  timers run. The next page takes its place in one frame, with its own background, once light has it ready:
  fetched, parsed, its sheets and scripts fetched and its document built. That moment is when the tab *moves*:
  its history, its title, its icon, the visit in light's history and what `history.length` says change then, not
  before. A page of light's own and a page the tab already has show at once, as before.
- **What a browser does that light does not.** A browser moves at the answer's first bytes and draws the page
  as it comes. light moves when the whole document is built, so the page held is held for the whole load.
- **What you see.** At once: a line across the top of the page that fills as the load goes (asked, sent,
  answered, received, parsed, sheets and scripts fetched, built), a spinner in the tab in place of its icon, Stop
  in place of Reload, and "Waiting for *host*…" in the status bubble. After 4 seconds the page held is dimmed,
  and is still the page: it scrolls and its links work. When the load fails, the page that says why takes the
  tab, at the address that failed. Stop, or Escape, gives the move up: the tab stays on the page it shows, at
  its address, and nothing is added to its history. A second move while the first is waited for takes its place,
  and the first's requests are cancelled. An answer with nothing to show (204) and a download leave the tab
  where it is.
- **The address field while a page is waited for.** A move *you* asked for (the address field, a page of
  light's, Back, Forward, Reload) is named there at once, in italics, with the neutral mark in place of the
  lock: the address is where the tab is going, and the page under it is not that address's page. A move *the
  page* asked for (a link, a form, a script) is not named until its page shows: a page cannot have its own
  content read under an address it sent the tab to. Once a page shows, the address and the lock are its own,
  always.
- **A place in the page.** An address that differs from the shown page's only by what follows its `#` (a link
  to `#notes`, the address typed, `location.hash`) is not fetched: the tab takes a step of its history, the
  address field shows it, and the page scrolls so the element it names is at the top of the view, or as near it
  as the page scrolls. That element is the first whose `id` is the fragment, else the first `<a name>`, by the
  fragment as written, then with its `%` escapes read; `#` and `#top` are the top of the page, and a fragment
  that names nothing scrolls nowhere. A page opened at such an address (a link of another page, a tab opened
  behind) scrolls there once it is laid out. The page's scripts hear `popstate` and `hashchange`; their own
  `history.pushState` scrolls nothing. In a frame the step is the tab's, and that frame's document alone
  scrolls. What follows the `#` is never sent to a server, in a request or in a `Referer`.
- **Where a page was left.** Each step of a tab's history keeps how far its page was scrolled when the tab left
  it. Back and Forward put the page back there, between two places of one page (nothing is fetched) as between
  two pages, one light fetches again included; Reload keeps where the page is. A step that kept nothing goes to
  the place its address names, else the top. What is kept is the page's own scroll: not that of a box that
  scrolls inside it, nor of a frame's document, which Back takes to the place its address names; and it is kept
  for the run, not across a restart.
- **What a browser does with places that light does not.** `:target` styles the element the address names, by
  its `id` or an `a`'s `name`, but for scripts (`querySelector`) matches an element by its `id` only;
  `location.hash` set by a script tells `hashchange` without `popstate`; a script's `scrollIntoView()` scrolls by
  the least that shows the element whatever its arguments, where a browser brings it to the top; and a place of
  the shown page is no visit of `light://history`.
- **Where there is no page to hold** (a new tab opened on an address, a tab's first load): the page area is
  light's own surface, light or dark as the settings say, with the same line and spinner; Stop then leaves the
  page that says the load was stopped.
- **Frames** hold too: an `iframe` shows its document until the next one is ready.
- **WebDriver** waits for the move as before; while it is waited for, the session's address and title are
  those of the page shown.
- **Timing.** light keeps when each step of a page's load ended, since the page was asked for: `asked` (the
  request given to the client), `connected`, `sent`, `answered` (the first byte), `received`, `parsed`,
  `gathered` (its sheets and scripts fetched), `built` (its document built), `shown` (the tab took it), `drawn`
  (the start of the frame after the one that first drew it) and `loaded` (its scripts told of `load`). The
  developer tools list them under the request of the page, in the Network tab's details, and
  `LIGHT_TIMING=1` prints them on the standard error once the page is drawn. Scripts are not told:
  `performance.getEntriesByType("navigation")` is empty.
- **Each request** is timed too. The details of a request, in the Network tab, say under "Connection" whether
  it went through a new connection or one used before, by which protocol (HTTP/1.1, HTTP/2.0), how long after
  the page's own request it was asked, and how long it waited for a connection, looked the name up, connected
  (and of that, the TLS handshake), sent, waited for the first byte and received. `LIGHT_TIMING=1` prints a
  `light: request:` line for each as it ends, with when it was asked against the end of the page's parse.
- **Connections.** A page's sheets, scripts, pictures and fonts go through the connection its document came
  through when the server keeps it, and HTTP/2 is spoken wherever the server offers it, every request of a
  host in one connection. Over HTTP/1.1 light opens six connections to a host at most and keeps them for the
  next requests, as browsers do. The sheets and scripts of a page are asked for together, 32 at once at most,
  as soon as the page is parsed; sheets apply and scripts run in the page's order whatever order they arrive
  in. Connections are not kept apart by the site of the tab: a host two sites both ask is asked through one
  connection (a third party still gets no cookie unless the settings allow it). light has no HTTP cache: every load asks
  for everything again.

### Windows

A tab opened by a Cmd+click or a middle click stands after the shown one and those it opened already, and a page
of the web loads in it at once. The links of light's own pages (the start page's tiles, the rows of the history)
open the same way. A page's scripts hear a Cmd+click as a click with its keys (`metaKey`, `ctrlKey`, `shiftKey`),
and may prevent it; they hear nothing of a middle click.

A link or a form that asks for a window of its own (`target="_blank"`, or a name no frame of the page has) opens in
a new tab, shown, beside its own; the keys held at the click win over it. "Open image in new tab" in the menu of a
right click, and the links of the developer tools, open their tab behind the shown one, as "Open link in new tab"
does.

Shift+click on a link, "Open link in new window" in the menu of a right click and Cmd+N open a window: the size of
the one it comes from, a little down and right of it, with tabs of its own. The windows share one profile: the
settings, the history, the cookies, what sites store and the downloads, so a setting changed in one window holds in
the others at once, and the history and the downloads pages list the same in each. The tabs, the developer tools,
the Network log and the Console are each window's own.

Closing a window stops what its tabs were loading and running, and leaves the other windows; light ends with the
last one, or at Cmd+Q, and writes its files then, once. Closing a window's last tab (Cmd+W, its close button, a
middle click on it, "Close tab" in its menu) closes the window.

### The developer tools

F12 opens them under the page, each tab its own. The tool bar has a tab for each tool, the count of the console's
errors in red (a click shows them), Preserve log and the close button. The top edge of the panel is a handle: drag it
to size the panel, between 120 points and the window less 80; a double click on it gives the panel its own height
back. The height is kept between runs (`tools_height` in `config.json`).

#### Inspector

The Inspector tab shows the page as its markup, in three panes, read-only:

- **The tree**: elements with their attributes, text cut short, the doctype of a page of the web. The first two levels
  are open, the rest opens at a click on its triangle, and an element of more than 200 children lists them 200 at a
  time. A click selects a row; Up and Down move by a row, Left closes or goes to the parent, Right opens or goes to the
  first child. `flex`, `grid` and `scroll` badges tell what an element is. The tree follows the page as scripts change
  it and keeps the selection while its node lives. Search HTML finds by tag, `#id`, `.class` or text, Enter going to the
  next match.
- **On the page**: the element under the pointer in the tree is tinted, content, padding, border and margin each in
  its colour, with a label of its tag, id, classes and size; the selected one keeps a dashed outline while the
  Inspector shows. The picker, the button left of the search, selects the element clicked in the page without the
  page getting the click, Escape leaving it; "Inspect element" in the menu of a right click does the same. In the
  console, `$0` is the selected element.
- **Rules and computed**: the rules that match the selected element, the strongest first, the style attribute among
  them, each with its sheet and line (a click opens the source), its `@media` and `@layer`, what another declaration
  overrides struck through. Beside them its box, margin to content, and its computed style: what was declared or
  inherited, or every property with Browser styles ticked.

What it does not show: comments and the page's own doctype, which the Document does not keep, pseudo-element rules,
and declarations ember's ui refused (those are in Warnings). A page of light's own is inspected too.

#### Network

- A row for each request, each hop of a redirect its own: its status on the colour of its kind, its method, its
  domain (a padlock for `https`), its file, who asked (the navigation, the document, a picture, an `iframe` or its frame, or the script and
  the line of a `fetch` or an `XMLHttpRequest`), its type in a word (`html`, `css`, `js`, `json`, `png`…), the bytes
  the server sent, those read once a gzip encoding is undone, and how long it took.
- A script's request light does not send (an `http` address asked by an `https` page, another origin asked in
  `same-origin` mode) and one whose answer is kept from the script (CORS) are blocked rows, with their sign and the
  reason in their details; a failed one is tinted. The preflight of a script's request is a row of its own, an
  `OPTIONS` of type `preflight`, before the request it asks leave for.
- The chips keep one kind: All, HTML, CSS, JS, XHR (what scripts asked for through `fetch`, `XMLHttpRequest` or
  `sendBeacon`, whatever was answered, and their preflights), Fonts, Images, Media, Other. There is none for WebSockets: light opens none. The field
  filters by address.
- A click on a column's name sorts by it, a second the other way, a third no more.
- A click on a row shows its headers and its response beside the table, the `Cookie` header it was sent with and
  the `Set-Cookie` headers it was answered among them; the pointer over a row names its address
  in the status bubble. The bin empties the list; a new page does too, unless Preserve log is checked.
- The arrow beside the bin saves the requests as a HAR 1.2 file, the one Chrome's and Firefox's Network panels
  import, to hand a whole exchange to someone else: those the filters list, as Firefox does, in the order they were
  asked. It goes to the downloads folder as `<host>-<date>-<time>.har`, never over another file, is listed among the
  downloads, and the status bubble says where it is. A right click on a row copies or saves that request alone
  (Copy as HAR, Save as HAR), or saves them all; a right click on the arrow or the table saves them all.
- A HAR file holds a page for each navigation and an entry for each request, each hop of a redirect, the requests
  light refused to send and those that failed (status 0, the reason in `_blocked` or `_error`) included: the
  headers as they were sent and answered, the `Cookie` and `Set-Cookie` among them, the query and the cookies
  parsed, what a POST carried with a form's fields, the body of the answer as text by its charset or in base64,
  its size read and its size sent, the server's address, and how long it waited for a connection, looked the host
  up, connected, did the TLS handshake, sent, waited and received. `_kind` is the request's kind, `_initiator` who
  asked.
- A tab keeps the bodies for it from its first request, tools open or not, so the request that went wrong is in
  the file: up to 4 MB of a body and 32 MB a tab, the oldest going first down to the 256 kB the Response view
  shows. A body that is not whole says so (`_truncated` and a `comment`), and its sizes are still the real ones.
  The bodies go when the list does.
- **A HAR file holds secrets.** Every entry is there twice in the menus: *sanitized*, which the arrow saves, and
  *with sensitive data*. Sanitized, as Chrome's export is, the values of the `Cookie`, `Set-Cookie`,
  `Authorization` and `Proxy-Authorization` headers and of the cookies read `[redacted]`. Nothing else is taken
  out: the addresses with their queries and the bodies are as they were sent, so a password typed in a form, a
  token in an address and whatever a page answered are in the file. Read it before you hand it on.

#### Console

The Console tab of the developer tools lists what the shown tab's page said, the newest line last, and follows it
unless you scrolled up:

- `console.log`, `info`, `warn`, `error` and `debug`, warnings and errors on their tint with their sign, several values on a
  line printed as a console prints them (`{a: [1, "two"]}`, `<p#id.class>`, `ƒ name()`, an error with its stack),
  `%s %d %i %f %o %O` filled in and `%c` left out. `assert`, `count`, `time`/`timeLog`/`timeEnd`, `dir`, `trace` (with
  the calls that led there), `group`/`groupEnd` (indented), `table` (as plain text) and `clear` work too.
- The exceptions nothing caught and the promises rejected with no handler, as errors whose calls unfold at a click
  on their triangle; the scripts
  that did not load or were skipped; the requests of scripts that failed or were blocked, in a browser's words
  (`Access to fetch at '…' from origin '…' has been blocked by CORS policy: …`, `Mixed Content: …`).
- On the right of a line, the script and the line it is from: a click shows that script's source in a new tab, behind
  the shown one (at its top: the source view does not scroll to a line). An inline script's lines count from its own first. An
  address in a line is a link that opens it in a new tab, behind too.
- A line said again in a row is listed once, with how many times.
- The bar filters by text and by level: Errors, Warnings, Info, Logs and Debug are each on or off. XHR lists the
  requests of scripts among the lines (method, address, status and time), Requests every request, and CSS what
  ember's ui refused in the page's sheets; the three are off at first. The bin empties the console; a new page does
  too, unless Preserve log is checked.
- The line at the bottom evaluates in the page: Enter runs it, its value comes back under it (a string in its
  quotes) or the error it threw, and `$_` is the last value. A `let` or a `const` a line begins with stays for the
  next lines. Alt+Up and Alt+Down (and Ctrl+P and Ctrl+N on macOS) walk what was typed before in the tab. A page without a
  script gets its scripts' context the first time a line is run.
- The text selects and copies as a page's does.
- In a build without the engine, or with JavaScript off in the settings, the tab says so and its line takes nothing.

Not there: the arrows alone for the history and Shift+Enter for a line of several (ember's text field keeps its
keys to itself), completion, and values you can unfold.

### Settings

`light://config` (Cmd+,) shows them; a change applies at once and is written to `config.json`. A value out of its
bounds, or a file that does not read, falls back to the default, setting by setting.

| Setting | Default | Overridden by |
|---------|---------|---------------|
| Start page (a new tab, the home button) | `light://home` | The address argument, at launch only |
| Search engine (where words typed in the address field go: DuckDuckGo, or light's own pages) | DuckDuckGo | |
| Appearance of light's own pages (`appearance`): as the system, light or dark. A click on the start page's logo sets the other one than shown | As the system, and as it changes; light where the system says none | |
| Downloads folder | The system's (`~/Downloads`) | `--downloads`, `LIGHT_DOWNLOADS` |
| Keep the pages visited | On | |
| Visits kept (100 to 100,000) | 2,000 | |
| Zoom of web pages (80 to 200 %) | 100 % | |
| Reverse the way the wheel and the trackpad scroll (`scroll_reversed`; light's own pages too) | Off: as the system scrolls | |
| Scroll speed (`scroll_speed`, 25 to 400 % of what the system says; light's own pages too) | 100 % | |
| Load pictures | On | |
| Load frames (the pages other pages show inside them) | On | |
| Run JavaScript (in a build with the `v8` tag; none runs otherwise) | On | |
| Follow a page that refreshes elsewhere at once | On | |
| Keep the cookies sites set, and send them back | On | |
| Those of other sites than the page's too (third-party cookies) | Off | |
| Let pages reach this machine and the local network | Off | |
| Request timeout (1 to 300 s) | 30 s | |
| Redirects followed (0 to 20) | 10 | |
| User-Agent | light's own (see [What light tells a site](#what-light-tells-a-site)) | |
| Languages (as `fr-FR, en`) | The system's, and as they change | |
| Preserve the Network log in new tabs | Off | |
| Requests kept in each tab's Network panel (100 to 10,000) | 1,000 | |

Pictures, JavaScript and refreshes apply to pages loaded afterwards; frames are loaded, or emptied, at once. The page also clears the history, clears the cookies and resets every setting.
The appearance and the languages follow the system while light runs: a Mac that turns dark at night turns light's own pages dark, unless the appearance is set (a `config.json` of an earlier light with `"dark_mode": true` is read as `"appearance": "dark"`, and the old key goes the next time the file is written). A light nobody sits at (a hidden run, a capture, ember's automation, `light drive`) does not look at the system: its own pages are light unless set dark, and its languages are those read at launch. The scroll settings are laid over the system's own: the direction and the distance the system gives the wheel and the trackpad stay what they are at 100 %, not reversed, and a page's scripts are told nothing of them. Pages of the web are not told the appearance either: `prefers-color-scheme` answers `light` to every page.
`--dir` (or `LIGHT_DIR`) names the folder the settings are kept in (see [below](#where-things-are-kept)); `light --help` lists the command line: `light [address]` (or `light browse [address]`) is the browser, `light drive` the one a test runner drives over [WebDriver](#webdriver).

The page also has the model's section, how long a page keeps its category (`category_hours`, with the way to `light://categories`), and the sites never remembered (`recall_excluded`); `hide_summaries` keeps the start page's summaries hidden.

### What light tells a site

Every value light sends a server or shows a page's scripts is true of light, of the machine or of the request. None
names another browser's engine or brand (no `AppleWebKit`, `Chrome`, `Safari` or `Gecko` token in the User-Agent),
and nothing is said to exist that does not work. light sends no client hints (`Sec-CH-UA`) and has no
`navigator.userAgentData`. One function decides the headers of every request (`pkg/identity`).

| Header | What it says |
|--------|--------------|
| `User-Agent` | `Mozilla/5.0 (<platform>) light/<version>`: the platform as browsers froze it (`Macintosh; Intel Mac OS X 10_15_7`, `Windows NT 10.0; Win64; x64`, `X11; Linux x86_64`), the version of the build, `0.1.0-alpha` for one made by hand. The settings can replace it. |
| `Accept` | By what is asked for: a page `text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8`, a picture `image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5` (no AVIF, which light does not read), a sheet `text/css,*/*;q=0.1`, the rest `*/*`. A script's `fetch` says its own. |
| `Accept-Language` | The system's languages (macOS: the user's preferred languages; elsewhere `LANGUAGE`, `LC_ALL`, `LC_MESSAGES`, `LANG`), as browsers list them: `fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7`. `en-US,en;q=0.9` when the system names none. Where the system tells light of a change (macOS and Windows, with a person at the window), the requests made from then on say the new ones. The settings can replace them; a script's `fetch` says its own. |
| `Accept-Encoding` | `gzip, deflate`, and `br` over https: the encodings light undoes. No `zstd`. |
| `Sec-Fetch-Dest`, `-Mode`, `-Site`, `-User` | What the request is for (`document`, `style`, `script`, `image`, `font`, `empty`), how it is made (`navigate`, `no-cors`, `cors`, `same-origin`), and from where: `none` for an address typed, a bookmark, a reload or a link of light's own pages, else `same-origin`, `same-site` or `cross-site` to the page that asks, worked out again at each redirect. `Sec-Fetch-User: ?1` only on a navigation an act of the user began: an address typed, a link clicked, a form submitted, a script's move while it hears a click. Sent to https addresses and to this machine only. |
| `Upgrade-Insecure-Requests` | `1` on navigations. |
| `Referer` | On every request a page makes, by the policy browsers have by default (`strict-origin-when-cross-origin`): the page's whole address within its origin, its origin alone to another, nothing from https to http. A font names its sheet. An address typed and a link of light's own pages send none. `<meta name="referrer">`, the `Referrer-Policy` header and `referrerpolicy` attributes are not read. |
| `Origin` | On a form's POST, and on a script's request where the standard asks for it. |

A link opened in a tab of its own (the command key, the middle button) is the player's own navigation: it says
`Sec-Fetch-Site: none` and no `Referer`. A font is asked for as a picture is (`no-cors`, no `Origin`).

Scripts read the same: `navigator.userAgent`, `appVersion` (the User-Agent after `Mozilla/`), `language` and
`languages` (those of `Accept-Language`), `platform` (`MacIntel`, `Win32`, `Linux x86_64`), `hardwareConcurrency`
(the machine's logical cores), `cookieEnabled` (the setting), and the constants every browser answers (`appName`
`Netscape`, `appCodeName` `Mozilla`, `product` `Gecko`, `vendor` empty). `plugins` and `mimeTypes` are empty,
`pdfViewerEnabled` and `javaEnabled()` false, `maxTouchPoints` 0, `doNotTrack` null, `onLine` always true. There is no
`deviceMemory`. `navigator.webdriver` is true when light is driven by something else than a person at its window:
ember's automation server (`EMBER_AUTOMATION`), a capture with its scripted input (`EMBER_CAPTURE…`), a hidden
window (`EMBER_HIDDEN`) or WebDriver (`light drive`).

light is not told its monitor's size, so `screen.width`, `height`, `availWidth` and `availHeight` are the window's,
as `outerWidth` and `outerHeight`; `screenX` and `screenY` are where the window is, `innerWidth` and `innerHeight`
the page's view, and `devicePixelRatio` the pixels a pixel of the page is drawn with (the display's scale times the
zoom). `Date` and `Intl` use the system's time zone.

### Frames

A page's `iframe` shows a document of its own, in ember's `iframe` element: light gives each one its own Document,
loaded as a page is, so its sheets style nothing around it. A frame's documents are part of its tab: their requests
are rows of the tab's Network panel, asked by `iframe` for the document and by `frame` for what it needs, and they
are in its HAR file.

- **What it shows**: its `srcdoc` when it has one, else the address of its `src`, read from the page that holds
  it; nothing for no `src`, `about:blank`, and any address that is not `http` or `https` (a `javascript:` or
  `data:` one runs and shows nothing, and the Console says so). A page is shown with its sheets, pictures and
  fonts, a text as it was sent; a picture alone, a file to download and the other kinds are not, and nothing is
  saved from a frame's address. A script that sets `src` or `srcdoc`, or adds or removes the element, is followed.
- **Scripts** of a frame's document run, in a JavaScript engine it shares only with the documents of its
  own origin ([below](#scripts-in-frames)): its script elements and modules, its timers, its `fetch`, its events. A frame
  whose `sandbox` has no `allow-scripts`, like any frame with JavaScript off, is shown as a page without scripts,
  what its `noscript` holds included, and a document with no script costs no engine.
- **Links and forms** of a frame open in it, with no entry in the tab's history: Back leaves the page, not the
  frame's last document, and a reload starts every frame again from its attributes. `target="_top"` opens in the
  tab, `_parent` in the frame around, a name in the frame of the tab that has it; `_blank` and a name no frame
  has open in a new tab of the window, shown. Cmd+click, a middle click and the menu of a right click open a frame's link in a
  tab as they do the page's. A form is sent, with a GET or a POST, where its target says.
- **Limits**: a tab's page holds 64 frames, 8 deep; one more stays empty, and so does a frame asked for the
  address of a page it is itself in. The Console says which.
- **Refusals**: a document answered with `X-Frame-Options: DENY`, or `SAMEORIGIN` when any page above it is of
  another origin, is not shown; nor is one whose `Content-Security-Policy` has a `frame-ancestors` that does not
  name every page above it (`'none'`, `'self'`, hosts with `*.` and ports, schemes). `frame-ancestors` decides
  alone when it is there, `ALLOW-FROM` is read as nothing, two `X-Frame-Options` that disagree refuse. It is
  checked on the answer's headers, after its redirects, before anything of it is read: none of its sheets or
  pictures is asked for. The frame shows a small notice, the Console says it as Chrome does, and the request is a
  blocked row of the Network panel.
- **Requests**: a frame's document is asked for as browsers ask (`Sec-Fetch-Dest: iframe`, `Sec-Fetch-Site`
  against the page that holds it, that page as `Referer`, `Sec-Fetch-User` only for a link you clicked in it). It
  is a page's request, not yours: a public page frames neither your router nor this machine
  ([Local network](#what-it-does-not-do)), at every hop of a redirect. Under a page that came over `https`, a frame
  at an `http` address is not asked for, nor a redirect to one (this machine's own addresses aside).
- **Cookies**: a frame of the page's own site, in pages of that site only, has the site's cookies. Any other is a
  third party: it gets none unless the settings let third-party cookies in, and then only those it was set as a
  frame under the same top-level site, kept apart (and written to `cookies.json` with that site). The same
  widget in two sites' pages has two jars, and neither is the one of a visit to the widget's own site.
- **Sandbox**: a frame with a `sandbox` attribute that does not say `allow-same-origin` has no origin: it and what
  it asks for carry no cookie and keep none, and its forms say they come from `null`. Its forms are not sent
  without `allow-forms`, whether a click or a script sends them, its links and its scripts do not leave the tab
  without `allow-top-navigation` (or `allow-top-navigation-by-user-activation`, on a click), a `_blank` link or a
  target no frame it reaches has goes nowhere without `allow-popups`, and nowhere either without
  `allow-popups-to-escape-sandbox`: light has no sandboxed tab to open in its place, so it opens none and the
  Console names the missing token, where a browser opens a window under the same sandbox. A `download` link
  saves nothing without `allow-downloads`, and a named target reaches only frames inside it. Without `allow-scripts` nothing runs;
  without `allow-modals` `alert`, `confirm` and `prompt` are refused in the Console. A frame in a sandboxed one is
  left what both allow, and a document keeps the sandbox it was loaded under until the frame shows another.
- **Not honoured**: the `frame-src` and `child-src` of the page's own Content-Security-Policy, `allow`
  (Permissions Policy), `loading="lazy"`, `referrerpolicy`, `csp`, `credentialless` and `scrolling`. A
  `frame-ancestors` in a `<meta>` is not read, as in browsers. The Inspector shows the `iframe`
  element, not the document in it, and "Inspect element" in a frame selects that element. The Warnings tab lists
  the page's, not its frames'. A menu or a dialog of a frame's document is clipped to the frame.

#### Scripts in frames

The documents of a page that are of one origin and keep their storage in one place (their kin, below) run in
one V8 engine (an isolate), each with its own globals; every other document has another engine. Two documents
of different origins share no object: what one knows of the other goes through light, in Go, as text.

- **What runs**: classic scripts and modules (a module map a document), timers and `requestAnimationFrame`,
  `fetch` and `XMLHttpRequest`, the events of clicks and keys (a click goes to the document under the pointer, the
  keys to the document last clicked in, and no other hears of them), `load` and `error` on an `iframe` told to the
  document that holds it, at any depth. An `iframe`'s `load` is told once its document's own `load` was, so a
  frame loads before the document that holds it, and `DOMContentLoaded` waits for no frame.
- **A document's identity** is set by light when it is shown and never read from its scripts: its origin (that
  of the address its answer came from, the parent's for a `srcdoc`, `null` in a sandbox without
  `allow-same-origin`), `window.origin`, the `Origin`, `Referer` and `Sec-Fetch-Site` of its requests, the origin
  its CORS checks are made against, the network space its requests may reach, and the https page above it that
  forbids requests in the clear.
- **Other windows**: `parent`, `top`, `frames`, `window[0]`, `window.name-of-a-frame`, `length` and an
  `iframe`'s `contentWindow` are windows of other frames. Such a window answers exactly what the standard lets a
  page of another origin have, from one table (`pkg/frames/window.go`) that also writes the tests: `window`, `self`,
  `frames`, `parent`, `top`, `opener` (`null`), `length`, `closed`, `location` (only `href` to set and
  `replace()`), `postMessage()`, `close()`, `focus()`, `blur()`, a frame by its index or its name, and `then`
  read as `undefined`. Anything else, read, written, defined or deleted, throws a `SecurityError`.
- **Frames of one origin reach each other**: `iframe.contentDocument`, `contentWindow.document`,
  `frameElement`, `parent.document`, `top.document`, `frames[i].document`, a frame by its name, and whatever
  else a window holds (`frame.contentWindow.someFunction()`, its globals, read, written, defined and deleted),
  for a frame that never left its first document, one sent elsewhere within the origin, a `srcdoc`, an
  `about:blank` and an `iframe` a script made. A frame holds an empty `about:blank` document of its parent's
  origin until what it asks for has come, as in a browser; light makes it when a script first reaches for it. A
  document with no script of its own gets an engine when its kin first reach for it. Each document keeps its own
  globals and prototypes: `iframe.contentWindow.Element !== Element`.
- **Who is kin** is decided in Go, from the origins light holds, by one function (`frames.Kin`), and asked again
  at every use of a window: two documents of the same tab's page, of the same origin (scheme, host and port),
  that keep their storage in the same place. A frame of the page's origin under a frame of another site keeps its
  storage apart and is kin to its like only, not to the page; a sandboxed frame is kin to none without
  `allow-same-origin`, nor one whose sandbox runs no script (its `contentDocument` is `null`). There is no
  `document.domain`. A window is the same object whatever its frame shows: it reaches the document while that is
  kin, and is the window above from the moment it is not.
- **What differs from a browser between kin** (deviations):
  - A node goes from one kin's document to another's as in a browser: `appendChild`, `insertBefore`,
    `replaceChild`, `append` and their like and `adoptNode` move it, `importNode` and `cloneNode` copy it. The
    node is the same object in both, of the document that made it (its prototype, listeners and own properties
    stay), `ownerDocument` is the new document, and the new document shows it, styles it, runs a script in it
    that never ran, loads an `iframe` in it again and finds a control's form. Mutation observers of both
    documents hear of it. What does not follow a node across:
    - what ui held of it and no attribute says (a value typed, a scroll position, focus, the selection);
    - a `Range`, a `NodeIterator` or a live selection, which stay with the document they were made in;
    - custom element reactions of the document it left; an element parsed into a node that crossed is not
      upgraded by the registry of the document that now has it;
    - an `iframe` element that crossed gives a `contentWindow` that is not `===` the `frames[n]` of its new
      document, though both reach the same window;
    - an event sent on a node that crossed bubbles to the window of the document that has it, but
      `event.view`, `baseURI` and what else a node reads from "its" window are of the document that made it.
  - `instanceof` is true for a node or an event of a kin against the interface of the same name (`node
    instanceof Node` for a node of a frame), once one document has reached into another: a browser answers
    false across windows. `Object.getPrototypeOf` tells them apart as in a browser.
  - What a script still holds of a document its frame left (the document, its nodes, its functions) throws when
    it is used, or answers what it last knew; a browser keeps such a document alive. A node that document made
    and another keeps is a new object the next time the keeper reaches it (its listeners are gone); a node
    another document made and put in it is still that document's to read and change, out of any page.
  - `frame.contentWindow.eval(…)` answers `undefined` and `new frame.contentWindow.Function(…)` makes nothing:
    V8 lets no document compile code in another's globals. A function the other document defined is called as
    any.
  - A window is reached through the windows above (`parent`, `frames`, `contentWindow`, `defaultView`,
    `event.source`). A script that stores its own `window` inside an object of another document hands over
    something the other cannot use (a `TypeError`); setting it as a property of the other's window works.
    `document.defaultView`, read by a listener of one document that another document's script called into, is
    the window as that other script sees it.
  - `postMessage` and `structuredClone` copy a plain object or an array of another document, and refuse its
    `Date`, `Map`, typed arrays and the like (`DataCloneError`): only their own document knows them for what
    they are.
- **A sandbox with `allow-scripts` and `allow-same-origin` is not a security boundary**, in light as in every
  browser, and the standard says so: such a frame is kin to its page, reaches its document, and can have it run
  anything (or take the `sandbox` attribute off its own `iframe`). light prints the warning browsers print
  ("An iframe which has both allow-scripts and allow-same-origin for its sandbox attribute can escape its
  sandboxing."). What light still holds such a frame to is what the standard checks against the script that
  asks, not a wall:
  - **Navigation**: the standard checks a navigation against the document of the script that asked for it (the
    incumbent), whatever window, location, link or form it went through. `w.location = …`, `href`, `assign()`
    and `replace()` on a kin's window are asked of light as for any window. A navigation made through a kin's
    own objects (`w.document.location`, its `reload()`, `w.history.back()`, a click on its link, its form's
    `submit()`, a function of its own that navigates) is checked against the document whose script was entered
    as well as the one that navigates, and refused if either may not: a sibling is never sent anywhere from a
    sandbox, the tab only with `allow-top-navigation` (or `-by-user-activation` on a click), a form only with
    `allow-forms`. V8 does not tell light the incumbent, so this is the entered document. What a sandboxed frame
    has its kin run later is therefore run as the kin's: a function it hands `w.setTimeout` is still refused
    what it sends through its own view of the windows (`w.location = …`), but what it sends through the kin's
    document (`w.document.location.href = …`) goes where the kin may, and so does source it hands the kin to
    compile (`w.setTimeout("location = …")`). A browser refuses those. A kin's own function that navigates
    itself when a sandboxed frame calls it is refused, which a browser would allow.
  - **`w.eval(source)` and `new w.Function(source)` between kin run nothing**: V8 compiles no source of one
    context for a script of another unless they share a security token, which the binding does not set.
    `eval` gives `undefined`, `Function` throws. A timer's source (`w.setTimeout("…")`) runs.
  - **Dialogs**: `w.alert()` called by a script of a sandboxed frame without `allow-modals` on its kin's window
    is refused (stricter than the standard, which asks the window's own document only).
  - Downloads, popups and pointer lock are asked of the document that holds the link or the form, as the
    standard has it. `window.open` opens nothing in light, for anyone.
  - Kin share their engine's memory limit and its clock for a script that runs long: see the failures below.
- **`postMessage`** between frames copies the message in the sender (light's structured clone), carries it as
  text through Go and builds it again from the receiver's own objects. `event.origin` and `event.source` are set
  by light. `targetOrigin` (`*`, `/`, or an origin) is checked when the message comes, against the origin of the
  document then in the window, and a message goes only to the document that was there when it was posted: a
  frame that went elsewhere, or was removed, gets nothing. Messages come in the order they were posted. A `Blob`, a `File`,
  an `ArrayBuffer` and typed arrays go as bytes, never as text; an `ArrayBuffer` transferred is copied and
  detached. A message is 32 MB at most, and 4096 may wait. Mail is delivered twice a tick of the tab, each time in
  rounds until none is left (256 rounds or 4 ms at most): 100 messages of a ping-pong between two frames come
  within a few ticks, not 100.
- **`MessagePort`** crosses frames: in the transfer list of `postMessage` (`event.ports`), inside another port's
  message, and onward to a third frame. A channel is two ends light keeps in Go, each held by one document: a
  script never has more of a port than its own `MessagePort`, which gives messages and nothing else (no
  reference to the other side, nothing up its prototypes), whatever the two documents are to each other.
  Messages wait until `start()` or `onmessage`, and travel with a port sent on unread. `close()` closes both ends
  (`close` is told to the other), and so does the end of the document that held one: a frame removed or sent
  elsewhere. A page holds 1024 ends at most.
- **`BroadcastChannel`** reaches the documents of the same origin under the same top-level site, in every
  tab and window of the profile (the partition storage has, below). A sandboxed document with no origin reaches
  none but itself. A tab that is not shown gets its messages when it is (1024 wait at most).
- **Session history** is the tab's: a frame sent elsewhere by a link, a form or a script (a new `src` on its
  `iframe` too) adds a step, as a visit of the tab does; a frame's first document, a `srcdoc`, and
  `location.replace()` add none. Back and Forward (the buttons, the keys, `history.back()`, `forward()` and
  `go(n)` from any document) go through these steps and put the right document back in the right frame, nested
  ones too; a step of a frame that is no longer there is passed over. A sandboxed frame moves only itself and the
  frames in it, and the tab only as it may navigate it. `history.length` is the tab's; `pushState`,
  `replaceState` and `popstate` are their document's. Reload shows the frames where they are, not where their
  attributes say (light has one reload, no hard one). What a frame's form posted is asked again by `GET` on Back.
  `light://history` lists the tab's visits only.
- **`focus()`** on a frame's window gives it the keys, when asked on a click or a key of yours, or by the
  document that has them; `blur()` does nothing.
- **Sending a window elsewhere** (`w.location = …`, `w.location.replace(…)`, a form a script sends): a document
  may send itself and the frames in it; its tab when it is of the tab's origin, or on a click of yours, and, in a
  sandbox, only as `allow-top-navigation` and `allow-top-navigation-by-user-activation` say; any other frame
  only when it is of its own origin and not sandboxed. Only to `http` and `https` addresses: a `javascript:`
  address runs nothing. What is refused is a line of the Console.
- **Storage and cookies**: `localStorage` and `sessionStorage` are kept by top-level site and origin. A frame
  of the tab's own site, with no other site between, has the site's own; any other frame has an area kept apart
  under the tab's site (in `storage.json`, under `framed`), which neither a visit to its own site nor the same
  frame under another site reads. `document.cookie` is the jar its requests use (above): nothing for a third
  party unless the settings let them in. A sandboxed frame with no origin has neither: `document.cookie` is empty
  and takes nothing, `localStorage` throws a `SecurityError`. IndexedDB databases are kept apart the same way,
  by the same keys: `indexedDB.open` throws a `SecurityError` in a sandboxed frame with no origin.
- **Failures stay in their frame**: an error a frame's script does not catch is a line of the Console, and the
  other documents go on. A script that never ends is a [slow script](#a-page-that-is-slow) as a page's is: a
  person's light asks whether to wait for it or stop it, a hidden or a driven one stops it at its budget. The
  other documents of its tab wait meanwhile, since they run one after the other, and then go on. Between kin
  the time counted is that of the script that was started, whichever document's function spins: the bar and the
  Console line ("ran too long") name the document whose script it was, and the timers of them all go on after. Kin
  share their engine's memory limit, one document's worth for them all. A frame whose drawing breaks light is
  emptied and its scripts closed.
- **Dialogs** (`alert`, `confirm`, `prompt`): the script that asks holds its engine, so nothing of its kin runs
  until it is answered, as nothing of its tab does. A kin removed or sent elsewhere meanwhile is gone for the
  page at once, and its scripts are closed when the answer came; one that would start waits for it too.
- **What it costs** (measured, Apple silicon): about 3.3 MB and 7 ms for each frame that runs scripts, 10 µs of
  each frame drawn after; a page of 64 such frames takes 175 MB and 0.6 s more than one without scripts (measured
  with an engine a frame; kin now share one). A document's scripts are closed when its frame is removed, shows
  something else, its tab leaves the page or its window closes, and an engine with the last document in it.
- **The developer tools** are yours, so they read every frame, and nothing of it reaches a page's scripts.
  The Inspector opens an `iframe` on its document, picks and outlines what is in one, and shows its rules and
  its box; `$0` is the selected element in the document it is of. The Warnings list each frame's as its own. The
  Console tags a frame's lines with its host, and a button beside its line chooses the document the line is
  evaluated in (the page's by default, each frame by its host and origin; the page's again once the frame
  went away). The Network tab does not filter by frame.
- **Not done**: `close()` and `blur()` on a frame do nothing, `window.open` opens nothing and a page's script
  opens no tab. An `iframe` gets its window when it is in its document; one beyond the 64 has none.

**The boundary and its limits.** Documents of different origins are in different isolates; those of one origin
and one storage place share one, where V8 keeps each document's globals from the others and light hands a
window across only after Go said the two are kin. The frames of a tab, and every tab, run in one process: an
isolate is a wall inside V8, not the operating system's. A bug in V8 that escapes an isolate reaches everything light holds, and
light does nothing against Spectre: the documents of a process share an address space, so `SharedArrayBuffer`
is not given to pages, `performance.now()` is told in steps of 100 µs and `crossOriginIsolated` is `false`
(`WebAssembly.Memory` can still be shared within one document, which has no worker to share it with). Neither
`Cross-Origin-Opener-Policy`, `Cross-Origin-Embedder-Policy`, `Cross-Origin-Resource-Policy` nor a Permissions
Policy is read. Do not keep in light what a hostile page must never reach.

### What a page asks

Built with the engine, `alert`, `confirm` and `prompt` show a dialog named after who asks ("site.test says", "An
embedded page at site.test says" for a frame), with the message as plain text (its first 2,000 characters, scrolled
past a few lines), a field holding the default for a `prompt`, OK and, but for an `alert`, Cancel. Enter is OK,
Escape is Cancel, and the script that asked waits for the answer: `confirm` gives true or false, `prompt` the text
or null. The time a dialog stays open is not counted as the script's ([a page that is slow](#a-page-that-is-slow)).

- The dialog is its tab's. A tab behind that asks waits, its name in italics, and shows its dialog when it is
  selected; its dialog never shows over another tab. While a tab's dialog shows, the whole window waits for it,
  light's header too, which is stricter than other browsers, whose header stays live.
- While a document waits for an answer nothing else of its tab runs: timers, events and messages of the page and of
  its frames wait and run after the answer, in their order; a click on the page is dropped.
- From its second question on, a page's dialog has a box "Stop this page from asking again". Once ticked, the
  page's calls return at once as if cancelled, each with a line in the Console, until another page is shown in the
  tab; a script that loops on `alert` is then a slow script as any other (below).
- A frame of the page's own origin asks as the page does. A frame of another origin, or sandboxed, asks nothing:
  the call returns as cancelled, with a line in the Console (a sandboxed one needs `allow-modals` and the page's
  origin, which a sandbox without `allow-same-origin` takes away).
- A page left, a tab closed or a window shut while it asks ends the script that asked: nothing after the call
  runs.
- light fires no `beforeunload`, `unload` or `pagehide`, so there is no "Leave this page?" prompt and no call to
  ignore during them.
- Without the engine no script runs, so nothing asks.

#### A page that is slow

A script that runs long is not stopped behind your back. After one second the window goes on without it: the
header, the other tabs and the other windows answer, the page is drawn as the script leaves it, and the Console
says so once ("A script of this page has run for 1s and still runs"). After five seconds a bar under the header
says which document is slow ("site.test is slow", "An embedded page at site.test is slow" for a frame's script)
with two buttons, and the tab's name goes italic:

- **Wait** hides the bar and leaves the script alone for ten more seconds, then asks again, as often as it takes.
  Wait holds the focus when the bar shows, so Enter waits and Tab then Enter stops, unless a text is being typed
  somewhere in the window (the address, a field of the page, a field of the developer tools): the bar then takes
  no focus, and Tab reaches it from there.
- **Stop it** ends that script, with the Console's "ran too long" line. The page stays, and its next timers,
  events and messages run. There is no choice to stop every script of the page.

While the script runs:

- Nothing else of its tab runs, as while a page asks: timers, events, messages, fetches and database requests that
  complete wait, and run after it in their order. A click or a key for the page waits too, and is heard once the
  script ended or was stopped. An expression typed in the Console is refused ("a script of the page is still
  running").
- The script's own calls of the browser and of the page are served for 8 ms of each frame. A script that only
  computes runs at full speed; one that writes to the page all the while runs at about half speed once the window
  goes on (measured: 104,000 operations in 4 s against 170,000 with the window waiting).
- A question it asks (`alert`…) shows as any other, and the time the dialog is open is not counted.
- The bar is its tab's. In a tab behind, the script goes on computing but waits at its next call of the page until
  the tab is shown again; the tab's name is italic meanwhile, and the bar shows when the tab is selected.
- A script light has to run while it answers another script's call cannot be left to run: it is stopped at five
  seconds.
- Leaving the page, reloading it, closing its tab or its window ends the script, with no line in the Console.

Other browsers hold the page's last picture and drop nothing of what you click meanwhile; light shows the page
as the script writes it. The times (1 s, 5 s, 10 s) are not settings.

A light nobody sits at asks nobody: hidden, capturing, driven by ember's automation or by `light drive`, it stops
a script at its budget, 2 seconds, or what `--script-budget` gives a `light drive`, with the Console's "ran too
long" line.

### Downloads

- An answer that is no page is shown by its kind: a picture alone on a dark page (PNG, JPEG, GIF, WebP, SVG, an
  ICO holding a PNG; a click switches between fitted and its own size), text kinds (CSS, JavaScript, JSON, XML,
  plain text…) as they were sent. The rest (PDF, archives, audio, video, fonts, AVIF…) is downloaded.
- What light cannot show is downloaded, and so is a link with a `download` attribute, an answer sent as an
  attachment, and what "Save link as…", "Save image as…" and "Save page as…" name.
- A file goes to the downloads folder under a free name: light never writes over a file. It is written as
  `<name>.lightdownload` and takes its name once whole; a cancelled or failed download leaves nothing behind.
- **Downloaded files are not scanned**, and light never opens or runs one by itself. "Open" and "Show in folder"
  act only on your click, only on a download of light's that is complete, through the system's own opener.
- There is no size cap, no pause and no resume: a download that failed is fetched again from its start by "Retry".
- A navigation that turns out to be a download is fetched twice: once to learn its kind, once to save it.
- The list is kept between runs, 500 entries at most. A download that a quit interrupted shows as failed.

### The model in detail

[A model on this device](model.md) says what the model is and what it helps with. It is **off** unless
you turn it on, and until then nothing of it is downloaded, started or written, and nothing of it shows but its
section under Experimental in `light://config`. Its settings are also in
`config.json`; which features it helps with is in `assist.json`.

| Setting (`config.json`) | Default | |
|---------|---------|---|
| `model` | `false` | Whether the model is on |
| `model_id` | `""` | One of the models light knows, by its id; none is the one light proposes for this machine's memory |
| `model_file` | `""` | A GGUF file of your own in its place, by its whole path (ignored when `model_id` is set) |
| `model_threads` | `0` | The threads it computes on (up to 64); 0 is half the fast cores, at least 2, at most 8 |
| `model_cpu` | `false` | Leave the graphics card out of it |
| `preview_seen` | `""` | The release whose warning, that light is an early preview, you agreed to; light shows it again at the first start of each release |
| `hide_summaries` | `false` | Keep what the model wrote of the pages visited off the start page |
| `translate_always` | `""` | The languages whose pages are translated as they show, as `fr, de` |
| `translate_never` | `""` | The languages whose pages are offered no translation |
| `recall_excluded` | `""` | The sites whose pages are never remembered, by their hosts, as `bank.example, mail.example` |
| `category_hours` | `24` | The hours a page keeps the category the model gave it before the model reads it again (0 to 720); 0 keeps none |

The page categories are not in `config.json`: they are in `categories.json` in light's folder, with the sites and
pages you corrected and what the model made of the pages it judged (dropped when the categories change, when the
history is cleared, and never kept while the history is not), and are edited in `light://categories`, a page
that is there only while the model helps with them.

- **What runs where.** The model runs in a program of its own, `light-model`, which light starts beside itself the
  first time something asks, and stops after five minutes without a request, when the model is turned off or changed,
  and when light quits. It is [llama.cpp](https://github.com/ggml-org/llama.cpp) loaded through
  [yzma](https://github.com/hybridgroup/yzma). light speaks to it over its standard input and output only: it listens
  on no port, opens no connection, reads the library and the model light names and nothing else, and is given none
  of light's environment. A page's text is only ever the model's input. Nothing you ask it leaves the machine.
- **What is downloaded, from where and when.** Only once the model is on and you accept the model light proposes
  (or choose another of its list): the llama.cpp libraries for your system from the project's own release on
  `github.com/ggml-org/llama.cpp` (13 to 25 MB), and the model from its publisher's repository on `huggingface.co`
  (the one proposed: Qwen3 4B, 2.5 GB, on Apple Silicon with 16 GB of memory or more, and Qwen2.5 1.5B Instruct,
  1.1 GB, elsewhere; both Apache-2.0). Both are asked over https with no cookie, a
  download goes on from where it stopped, and a file is used only once its size and SHA-256 are the ones this
  version of light was built with (`pkg/model/manifest.json`). An archive is unpacked keeping only its libraries
  and licence. light downloads no model from an address of your own.
- **A model of your own.** `model_file` names a GGUF file already on your machine. light reads what it is, refuses
  one that does not load or has no chat template, saying why, notes its SHA-256 so that a changed file is read
  again, and never copies or deletes it.
- **What is written.** Everything is in `model/` in [light's folder](#where-things-are-kept): `runtime/` (the
  libraries), `models/` (the models, each with a note of its digest), `downloads/` (what is still downloading),
  `own.json` (what your own file is), `cache/` (the helper's home: one small library, libffi, that yzma unpacks
  there to call llama.cpp) and `helper.log` (what the helper wrote on its error output, never shown to a page). A
  hidden run not given a folder has no model.
- **Turning it off and deleting it.** `model: false` stops the helper at once. Deleting the model removes the whole
  `model/` folder; a model of the list can be deleted alone. A file of your own stays where it is.
- **For developers.** light looks for `light-model` beside its own executable. `task model:build` builds it into
  `dist/`, and `LIGHT_MODEL_HELPER=$PWD/dist/light-model` names it to a light run from the sources. `LIGHT_MODEL_DEV` has a development build use a
  llama.cpp and a model already on the machine, downloading nothing:
  `LIGHT_MODEL_DEV="library=/opt/homebrew/lib;backends=/opt/homebrew/opt/ggml/libexec;model=/path/to/model.gguf"`
  (after `brew install llama.cpp`; `embedding=` names an embedding model). A released light ignores both, and a
  released `light-model` refuses them, as it refuses a llama.cpp from a system folder or of another version than
  the one light installs.

### Where things are kept

`history.json`, `bookmarks.json`, `downloads.json`, `config.json`, `assist.json` (the features the model helps
with), `categories.json` (the page categories), `recall.bin` (what pages said, when that is remembered), `model/`
(the model's files), `storage.json` (what pages kept in `localStorage`, 5 MB an
origin), `indexeddb/` (the IndexedDB databases, one file a site, see [What it does not do](#what-it-does-not-do))
and `cookies.json` (the cookies that outlive a run, readable by you alone: they are what signs you in) are
in light's folder of your config folder
(`~/Library/Application Support/light` on macOS). `--dir` or `LIGHT_DIR` names another folder. A hidden run (`EMBER_HIDDEN`)
not given one keeps them in memory.

### What it does not do

- Built without the `v8` tag it runs no script: a page that builds itself in JavaScript shows little, and what a
  `noscript` holds is shown, as with JavaScript off. A font a script loads (Typekit…) is then replaced by the next
  family of its list.
- **Fonts**: a page's `@font-face` fonts are fetched when its text first needs them, TrueType, OpenType, WOFF and
  WOFF2, and each is a row of the Network panel asked by its stylesheet, sent with the sheet's address as its
  `Referer` (the site alone to another site) and the cookies of its address. Until one lands the text is laid out
  in the next family and hidden as its `font-display` says, as in Chrome (up to 3 s for `auto` and `block`, never
  for `swap`), then the page is laid out again; an icon shows as nothing meanwhile. A font is 16 MB at most and has
  30 s to come; one that fails is a warning of the page. A variable font shows the weight and width its text asks
  for, a bold or italic the family lacks is made up from the face it has, the font's OpenType features and
  ligatures apply (an icon font that writes its icons as words, as Material Icons, shows the icons), and colour
  fonts and collections are not read. A character none of the page's fonts has is drawn by Go Mono, or as `?`:
  light does not yet look among the machine's fonts for one that has it, so Japanese, Chinese, Korean and Arabic
  show only where the page names a font that has them. A font
  of another site is used whatever its `Access-Control-Allow-Origin`, where a browser refuses it without one.
  A sheet an `@import` names is fetched as the page shows, asked by the sheet that imports it, and taken when it
  is served as `text/css` or as no type; the page shows without it until it lands. light keeps its own
  `User-Agent`, which names no other browser: Google Fonts answers it with TrueType files of whole families, where it gives browsers WOFF2 cut
  by `unicode-range`.
- It sends a form with a GET or a POST, as `application/x-www-form-urlencoded` or `text/plain`, in the page's
  encoding: the controls of the form in the page's order (those its `form` attribute gives it included), the
  button that was pressed and no other, no disabled control and no unchecked box, with the `formaction`,
  `formmethod` and `formenctype` of that button. A POST says its `Origin` and its `Referer` (the whole address
  within an origin, the origin alone to another, nothing from `https` to `http`), carries no `SameSite=Lax` cookie
  to another site, and is a row of the Network panel. A redirect that answers it is followed with a GET after a
  301, a 302 or a 303 and with the form again after a 307 or a 308, of the origin `null` to another origin. A form
  is sent once: Reload, Back and Forward fetch the page a POST gave with a GET, and never send the form again. A
  form with `target="_blank"` is sent in a new tab. What is not sent is a warning in the Console: a `multipart/form-data`
  form, and so a file (a file field sends an empty name otherwise), and a form of a dialog (`method="dialog"`). An
  `accept-charset`, a `_charset_` field and the place an `<input type="image">` was clicked at are not read, and
  the Network panel does not show what a POST carried: a HAR file saved from it does.
- It sends no credentials: a page that asks to sign in (401, 407) says so and stops.
- Its cookies follow the rules of RFC 6265bis, with these limits: no partitioned cookies (CHIPS), no Cookie Store
  API, no `Set-Cookie` from a `<meta http-equiv>`, no cookie without a name. A cookie holds 4,096 bytes, a site
  180 cookies, the jar 3,000, and none lasts over 400 days; the least lately sent go first. A `Secure` cookie
  goes over `https` only, and so not to `http://localhost`. `http` and `https` are two sites for `SameSite`. A link
  opened in a new tab and a download count as asked by you, not by the page. A cookie of the run alone ends when
  light quits, not when its tab closes. With cookies off, those kept stay in their file, unsent, until cleared.
- It has no cache: every page shown for the first time in a tab is fetched, and nothing is revalidated (no 304).
- It follows redirects (301, 302, 303, 307, 308) and a refresh of 0 seconds, as many in a row as the settings say,
  to `http` and `https` only. A refresh that waits is left alone.
- It reads gzip, deflate and, over https, Brotli. No zstd, and it does not ask for it.
- A page's `Content-Language` header, when it names one language, is the page's language: what `:lang()` reads
  where no `lang` attribute says one. A `<meta http-equiv="content-language">` is not read.
- It holds a page's answer in memory, 64 MB at most, and lays out the first 512 kB of a text. A download is
  streamed to its file instead.
- It decodes PNG, JPEG, GIF, WebP (lossy and lossless, with alpha; an animated one is not drawn) and SVG. AVIF and
  BMP are downloaded.
- An `img` shows the picture ember's ui chooses as Chrome does, of its `srcset` or of its `<picture>`'s first
  `<source>` whose `media` holds and whose `type` light draws, for the slot `sizes` gives and the screen's pixels
  a point (`img.currentSrc` reads it); it is chosen again when the view, the attributes or the sources change.
  Lazy loading is not done.
- **Local network**: a page asks only addresses as public as the one it came from. One that came from the
  Internet reaches neither the network around this machine (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`,
  `169.254.0.0/16`, `100.64.0.0/10`, `fc00::/7`, `fe80::/10` and the other addresses the Internet does not route)
  nor this machine (`127.0.0.0/8`, `::1`, `0.0.0.0/8`); one that came from the local network does not reach this
  machine; one of this machine, a file and a page of light's reach everything. It holds for all a page asks for by
  itself: its pictures, sheets, fonts and scripts, what its scripts fetch, a redirect of any of these, a refresh,
  and a move or a form its scripts begin with no click of yours. It is checked on the address a name resolves
  to, as the connection opens, so a public name that turns to `127.0.0.1` is refused too, and a page's requests
  never share a connection opened for you. What is refused is a blocked row of the Network panel, and for a
  script's request a line of the Console. Where you go yourself is not held back: an address typed, a bookmark,
  a link or a button you click, and the redirects of these (a public page you open may so redirect you to your
  router). There is no prompt and no `Access-Control-Allow-Private-Network`: the setting "Let pages reach this
  machine and the local network" turns the whole protection off. Through a proxy (`HTTP_PROXY`, `HTTPS_PROXY`)
  the address that answers is the proxy's, so only an address written in numbers, or `localhost`, is refused, a
  name that resolves to a private address is not, and every page counts as one of the Internet.
- It does not scan what it downloads, and neither pauses nor resumes a download.
- It zooms web pages only, not its own pages. The keys zoom one tab; the setting is the zoom of the others.
- It colours no syntax in a source, and completes no address from the history.
- A Control+click is not a right click on macOS.
- A link or a form with `target="_blank"` opens a new tab, never a window, and a named target that no frame has
  opens a new tab each time: a tab is not found again by the name it was opened under. A script's `window.open`
  opens nothing.
- With no window left light ends, on macOS too; a tab is not dragged from a window to another.
- It draws what ember's ui draws. What is missing is listed in ember's guide:
  [What is missing](https://github.com/lantervale/ember/blob/main/docs/05-ui/10-missing.md).

### Limits of its JavaScript

What runs: classic scripts, inline and `src`, in document order, with `defer` and `async`, of the types and
`language`s the HTML Standard names JavaScript's; a script element a script puts in the page, fills with text or
gives a `src` (its text runs at once, a `src` once fetched: in the order they were put in the page when `async` is
set false, as each comes otherwise, and the page's `load` waits for them); modules (below);
`on…` attributes; the
DOM (the tree as the parser made it: comments, the doctype, text, `svg`, `noscript`, `template` and its content,
`script` and `style` where the page has them, every attribute as written; classes, `style`, `dataset`, `innerHTML`
and `outerHTML` written as browsers write them, selectors, forms' values), which a framework hydrates (tested with
React 18); events with
capture, bubbling and `preventDefault`; `DOMContentLoaded` and `load`; timers, microtasks,
`requestAnimationFrame`; `fetch`, `XMLHttpRequest` and `navigator.sendBeacon` (below); `getBoundingClientRect` and the `offset…` and `client…`
sizes; stylesheets (a `<style>` or a `<link rel="stylesheet">` added, edited, moved or removed by a script, its
`sheet` with `cssRules`, `insertRule` and `deleteRule`, a rule's `selectorText` and `style`, `document.styleSheets`,
`new CSSStyleSheet()` with `adoptedStyleSheets`, `CSS.supports`), applied once a frame; `document.fonts` (`ready`, `status`, `check()`, `load()`, `loading` and `loadingdone`; it lists no `FontFace` and takes none); `getComputedStyle`; `localStorage` and `sessionStorage`; `indexedDB` (below); XPath 1.0 (`document.evaluate`, below); `history.pushState` and `popstate`; `MutationObserver`,
`CustomEvent`, `URL`, `TextEncoder`, `atob`; messages (`MessageChannel` and its ports, a port sent through a port,
`window.postMessage` with its `targetOrigin`, `BroadcastChannel`, `structuredClone` with `transfer`). One context a
page, closed when the tab leaves it.

What scripts send, by the Fetch Standard: `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS` and methods
of their own (`CONNECT`, `TRACE` and `TRACK` throw), with a body that is a string, a `URLSearchParams`, a `FormData`
(as `multipart/form-data`, its `Blob` and `File` values as files), a `Blob`, an `ArrayBuffer` or a typed array,
each with the `Content-Type` it implies unless the script names one, and the headers the script sets but those
that are the browser's (`Cookie`, `Host`, `Origin`, `Referer`, `Content-Length`, `Sec-…`, `Proxy-…`…), which are
dropped. light says the `Origin` and the `Referer` itself. A request to the page's own origin goes straight. To
another origin (`mode: "cors"`, the default) a simple request is sent and its answer given to the script only if
its `Access-Control-Allow-Origin` allows the page, with `Access-Control-Allow-Credentials: true` and no wildcard
when the request has credentials; any other request is first asked for with a preflight `OPTIONS`, and sent only
if the answer allows its method and its headers (the answer is remembered for its `Access-Control-Max-Age`, 5
seconds unsaid, 10 minutes at most). Of another origin's answer a script reads the body and the headers every
answer may show, with those `Access-Control-Expose-Headers` names. `mode: "no-cors"` sends a simple request and
gives an opaque answer (status 0, nothing to read); `mode: "same-origin"` sends nothing to another origin.
`credentials` (`withCredentials` for an `XMLHttpRequest`) says whether the request carries cookies and keeps
those its answer sets: `same-origin` (the default) for the page's own origin only, `include` for any, `omit` for
none; the jar's own rules hold over it (no cookie to a third party unless the setting allows them, and then no
`SameSite=Lax` or `Strict` one). `redirect` is `follow`, `error` or `manual`; each hop of a redirect is checked
as a request of its own, and one that goes from another origin to a third is sent as of the origin `null`. An
`https` page sends nothing to an `http` address but this machine's (`localhost`, `127.0.0.1`). `sendBeacon` is a
`POST` with credentials that goes on once the page is left, as a `fetch` with `keepalive` does, up to 64 KiB,
`no-cors` unless its body is of a type a form could not send. An
`AbortSignal` stops a request, an `XMLHttpRequest`'s `timeout` too. What is blocked is a network error for the
script (`TypeError: Failed to fetch`, an `error` event), told in the Console and listed in the Network panel.

Modules, by the HTML Standard: `<script type="module">`, inline and `src`, run once the page is read, in the
order of the page among the `defer` scripts, an `async` one when it is ready; a `nomodule` script is not run.
`import` and `export` are the engine's own: the whole graph is fetched first, each address once a page whoever
imports it, then linked and run in order, with cycles, live bindings and an `await` at the top holding the
modules that import it. A specifier is an address (`./`, `../`, `/` or a whole one, read from the module's own
address after its redirects) or a name of the page's import maps (`<script type="importmap">`, `imports` and
`scopes`); any other is a `TypeError`. `import()` works in modules and in classic scripts, `import.meta.url` and
`import.meta.resolve()` too, and so does a JSON module (`with { type: "json" }`). A `<link rel="modulepreload">`
fetches its module ahead. A module is asked for as a `fetch` in `cors` mode is, with the page's cookies for its
own origin alone unless `crossorigin="use-credentials"`: another origin's is run only if it shares it with the
page, and only an answer of a JavaScript type is (a JSON one for a JSON module). What stops a module (it was not
found, is not shared, is of another type, does not read, imports what nothing exports, throws) is said once in
the Console with its address and its line, fires `error` on its script element when it was not fetched, and
stops no other script. Modules are rows of the Network panel, as scripts.

What does not:

- **Modules**: the engine's binding tells light neither where a module's syntax error is when the text is one
  only a module may be (the Console then names the address, no line), nor of an `import()`: light finds the
  `import(` of a script by asking the engine's own parser about them, all at once (three more readings of a
  script that has any, more only where an `import(` is in a text, a pattern or a method's name), so one written
  in an `on…` attribute, in `eval`, in `new Function` or at the Console's line is rejected, and the Console says
  why; so is one of a classic script that is not strict code, or of a script light would read again over 512 MB
  to tell them apart, which the Console says too. `import.meta` is filled by the module's first statement, so a
  function of a module in a cycle called before its module ran reads none, and the columns of a module's first
  line are off by that statement's length. No CSS module, no
  `integrity`, no `referrerpolicy` (a module says the page as its referrer), no import map with a `src`, and an
  import map counts whenever it comes, the first to name a specifier kept. A `modulepreload` fetches its module,
  not what it imports. The page's address, not its `<base>`, is what an inline module's specifiers are read from.
- An `iframe`'s document of another origin runs its scripts in another engine, so the page reaches its window
  and never its document: `contentDocument` is `null` there, and is the frame's document for a frame of the
  page's own origin (see [Scripts in frames](#scripts-in-frames)). Its `src`,
  `srcdoc`, `name`, `width`, `height` and `sandbox` reflect, a `src` or a `srcdoc` a script sets is followed, and
  the element hears `load` when a document, or light's notice in place of one, shows in it, and `error` when light
  left it empty (see [Frames](#frames)); a browser says `load` there too.
- Workers, service workers, WebSockets,
  shadow trees, custom elements, canvas contexts, WebGL, media: absent, or answering with
  nothing. An `svg` is in the tree and scripts change it:
  it is drawn as one picture, again the frame after a change, with `currentColor` the colour of its text, but the
  page's rules do not reach the shapes inside it (`svg path { fill: … }`), and `foreignObject` and animations
  are not drawn; scripts run once the whole page is read, so `document.write` writes nothing.
- **Requests**: an answer's bytes reach a script as they came (`arrayBuffer()`, `bytes()`, `blob()`, an
  `XMLHttpRequest` of type `arraybuffer` or `blob`), and are decoded where it asks for text: as UTF-8 by `text()`
  and `json()` whatever the answer says of its encoding, by the answer's charset (or `overrideMimeType`'s) for
  `responseText`. It comes whole, with no stream (`body` is `null`) and no progress events while it comes or
  while a body is sent; a request's body is no stream either. `formData()` reads an
  `application/x-www-form-urlencoded` body only, a `blob:` address of `URL.createObjectURL` is fetched by
  nothing, and a `TextDecoder` decodes UTF-8 only. No synchronous `XMLHttpRequest`, no `cache` modes and no HTTP cache, no `integrity`, no `referrerPolicy` (the default policy is
  the only one), no `priority`. A page of no origin (`light://`, a file) may read only what is shared with all
  (`Access-Control-Allow-Origin: *`), not what names the origin `null`. A `Blob` or a `FormData` is sent whole from
  memory, and a `FormData` made from a form has no file of a file field. A `no-cors` answer is not checked against `Cross-Origin-Resource-Policy`, and the
  redirect a `manual` request stopped at cannot be followed by a navigation. No Service Worker, WebSocket or
  EventSource.
- **Cookies**: `document.cookie` reads and sets the browser's cookies for the page, those marked `HttpOnly` kept
  from it, and `navigator.cookieEnabled` says whether the setting is on. A script reads no `Set-Cookie` header of
  what it fetched.
- **Styles**: a rule ember's ui does not read applies nothing, whoever inserted it. A stylesheet moved among the others is read again, and what scripts inserted in it kept.
  `getComputedStyle` answers what ui computed: lengths in px, colours as `rgb()` when they were written in
  hexadecimal and as written otherwise, a width and a height as laid out at the last frame; an element a script
  put in the page since then is styled and laid out at once when its style is read. A rule's `style` is its text read again: no `CSSKeyframeRule`,
  no `CSSStyleSheet` of an `@import`, no `ownerRule`.
- **Messages**: `window.postMessage` reaches the window itself and the windows of the tab's frames
  ([Scripts in frames](#scripts-in-frames)), and a `BroadcastChannel` the documents that section names. A message is
  copied by light's own structured clone, written in JavaScript (V8's is not within reach): primitives, plain
  objects and arrays, `Date`, `RegExp`, `Map`, `Set`, `ArrayBuffer` and its views, errors, `Blob`, with their cycles;
  an `ArrayBuffer` transferred is copied, then detached. Each message is a task of its own among the timers, so it
  waits for the next frame, and the promises its handler settles run once the frame's tasks have (as for a timer).
- **XML documents**: a page sent as `application/xhtml+xml` is read by XML's rules into the tree HTML is read
  into, its elements in XHTML's namespace: a CDATA section is text (a node of its own for scripts), an element
  closed in its start tag holds nothing, names keep their case, nothing is implied or closed for the page, and
  namespaces are resolved (`svg` and MathML as in HTML, an `xml:` or `xlink:` attribute under that name). The five
  entities of XML are read, and HTML's named ones under a doctype of XHTML or MathML. The encoding is the byte
  order mark's, else the header's, else the XML declaration's, else UTF-8. A document that is not well-formed is
  not shown: light says where it stops (line and column) in place of the page, and never reads it as HTML. To
  its scripts such a page is a document of XML: `contentType` says its type, `createElement` keeps the case,
  `tagName` and attribute names are as written, `createCDATASection` works, `innerHTML`, `outerHTML` and
  `insertAdjacentHTML` read and write XML (markup that is not well-formed is a `SyntaxError` and changes
  nothing), and `document.write` does nothing, as in every page. An element and an attribute keep the prefix they
  were written with (`prefix`, `tagName`, `name`), and what is written back (`XMLSerializer`, `outerHTML`) is
  written by the DOM Parsing standard's rules: the prefix kept where it still names its namespace, a declared one
  else, a generated `ns1`, `ns2`… where none does, a default namespace declared again where it changes, an
  element of HTML that holds nothing as `<p></p>` and a void one as `<br />`. What is missing: a doctype's
  internal subset is skipped, so the entities it declares are not known; `xml:space` and the `xml-stylesheet` instruction do nothing; a
  selector of a name matches no XHTML element whose name has capitals; an SVG document is shown as a picture, and is no document to scripts.
- **A document of XML that is no page** (`application/xml`, `text/xml`, a `…+xml` type other than XHTML's and
  SVG's: a feed, a sitemap) is still shown as the text it was sent, in a tab and in a frame. When it is
  well-formed and scripts run, it is also a document: in a frame, `contentDocument` is its `XMLDocument` for the
  documents it is kin to (`documentElement`, `contentType`, `createElement` in no namespace, the whole DOM), in a
  window of its own (`contentWindow`), and the frame's `load` fires as for any. In a tab, the Console's line
  reads it. What a script changes in it shows nowhere: the text stays as sent. Its own scripts do not run (an
  XHTML `script` element in it is a node like the others), it has no sheets, and one that is not well-formed is
  text and no document (`contentDocument` is `null`), where a browser shows the parser's error.
- **Documents a script makes** are whole: `document.implementation.createHTMLDocument`, `createDocument` and
  `createDocumentType`, `new Document()`, a document's `cloneNode`, `DOMParser` (`text/html` and the four XML
  types, with the `parsererror` document for XML that is not well-formed) and an `XMLHttpRequest`'s
  `responseXML` and `response` of type `document` each give a real `Document` (an `XMLDocument` from
  `createDocument`) with its own tree: `createElement` and `createElementNS`, text, comments, fragments,
  `createProcessingInstruction`, `createCDATASection` in a document of XML, `getElementById`,
  `getElementsBy…`, `querySelector`, `importNode`, `adoptNode`, `documentElement`, `head`, `body`, `title`,
  `doctype`, `contentType`, `URL`, `compatMode`, `implementation`. A node is of the document that made it
  (`ownerDocument`) and becomes another's when it is put there. Such a document has no window (`defaultView` is
  `null`): nothing of it is laid out, no picture, sheet or frame of it is fetched, and none of its scripts or
  `on…` attributes runs, wherever they were made or read; a script read into one never runs, even moved into the
  page. A document holds what the DOM lets it hold (one element, one doctype before it, no text), and
  `lookupNamespaceURI`, `lookupPrefix` and `isDefaultNamespace` answer from the elements' own namespaces and the
  attributes that declare one. What is missing: what a `template` holds stays the page's; an `iframe`'s
  `contentDocument` is `null` ([Scripts in frames](#scripts-in-frames)); `HTMLDocument` is `Document` itself.
  `characterSet` (`charset`, `inputEncoding`) is the encoding the page came in, named as the Encoding standard
  names it; a document a script made is `UTF-8`.
- **Lists of nodes** are the DOM's own objects and no arrays: a `NodeList` (`childNodes` live, `querySelectorAll`
  not) and an `HTMLCollection` (`children`, `getElementsBy…`, `forms`, `images`, `links`, `scripts`, `embeds`,
  `rows`, `cells`, `tBodies`) answer by index and, for a collection, by the `id` then the `name` of its elements
  (a `name` only on HTML's), with `item`, `namedItem`, iteration, no own `length` and names that do not
  enumerate. A select's `options` is an `HTMLOptionsCollection` (set by index, `length`, `add`, `remove`), a
  form's `elements` an `HTMLFormControlsCollection` whose controls of one name are a `RadioNodeList`. A live list
  is read again from the tree only once the tree has changed.
- **Ranges and walkers**: `Range` is whole (its ends set and compared, `selectNode`, `compareBoundaryPoints`,
  `deleteContents`, `extractContents`, `cloneContents`, `insertNode`, `surroundContents`, `isPointInRange`,
  `comparePoint`, `intersectsNode`, its text) and live: its ends follow every change a script makes to the tree
  and to text (`splitText`, `normalize`, `replaceData`…). `StaticRange`, `TreeWalker` and `NodeIterator` (moved
  off a node that leaves, as the DOM says) with any `NodeFilter`. What is missing: a range's rectangles
  (`getClientRects`, `getBoundingClientRect`) and the selection's ranges (`getRangeAt`, `addRange`): the
  selection is ember's and is not a script's to set.
- **Events** go down to their target and back as the DOM orders it (the capturing listeners of the target first),
  with `composedPath`, `cancelBubble`, `returnValue`, `srcElement`, `window.event`, `once`, `passive`, a `signal`
  that takes a listener away, `createEvent` with the names the DOM keeps, and `initEvent` and its kin. A click a
  script sends (`click()`, `dispatchEvent`) acts as the standard says: a checkbox or a radio is set before the
  listeners run and set back when one cancels it, then tells of its `input` and `change`; a submit button sends
  its form; a link is followed. The handlers a `body` or a `frameset` is given for the window's events (`onload`,
  `onresize`, `onblur`…) are the window's. `AbortSignal` has `abort()`, `timeout()`, `any()`, `reason` and
  `throwIfAborted()`. A listener of `touchstart`, `touchmove`, `wheel` or `mousewheel` on the window, the
  document, its element or its body is passive unless it says otherwise. What is missing: events of animations and transitions (their
  handlers are there, prefixed ones included, and hear what a script sends).
- **Changes told to observers**: a `MutationObserver` hears one record for a child replaced, for a fragment put
  in and for `replaceChildren`, and follows what left a subtree it watches until it is told; `moveBefore` moves
  a node within its tree. The root element of the page leaves its document as any node does, and another takes
  its place.
- **Selectors** from a script (`querySelector`, `matches`, `closest`) are read as CSS writes them: escapes,
  namespaces (`*|a`, `|a`), `:is`, `:where`, `:not`, `:has`, the `nth-` family, `:lang()`, `:target`, `:root`,
  `:empty`, `:scope`, `:link`, `:defined`, and what a control is: `:checked`, `:indeterminate`, `:default`,
  `:disabled` and `:enabled` (through a `fieldset` and its first `legend`), `:required`, `:optional`,
  `:read-only`, `:read-write`, `:placeholder-shown`, `:valid`, `:invalid`, `:in-range`, `:out-of-range`. The last
  four follow the constraint validation API, which is there too (`validity`, `willValidate`, `checkValidity`,
  `reportValidity`, `setCustomValidity`, `validationMessage`; `reportValidity` shows no bubble). A sheet is
  matched by ember's ui, which knows fewer of them.
- **Interfaces** are shaped as their IDL says: one table generated from the DOM standard's IDL
  (`tools/wpt/idl.py` writes `internal/dom/js/88-idl.js` from `.wpt/wpt/interfaces/dom.idl`) gives each
  interface its operations with their number of arguments (fewer is a `TypeError`), enumerable attributes that
  refuse another object, constants, `Symbol.toStringTag` and `Symbol.unscopables`.
- **XPath** is XPath 1.0, whole: `document.evaluate`, `createExpression` and `createNSResolver`, `XPathEvaluator`,
  `XPathExpression`, and `XPathResult` in its ten types, with the standard's errors (a `SyntaxError` for what is
  no expression, a `NamespaceError` for a prefix the resolver does not know, a `TypeError` for a result read as
  what it is not). The thirteen axes (`namespace::` is empty, as in browsers), every node test, predicates,
  unions, the operators and the 27 core functions, with numbers read, written and rounded as XPath says. An HTML
  document is matched as browsers match it: a name without a prefix finds HTML elements whatever its case and no
  `svg` or MathML element (`//svg:path` with a resolver, or `//*[local-name() = "path"]`, does), an attribute's
  name is lower-cased, and `lang()` reads the `xml:lang` of the XML namespace, which only an `svg` or a MathML
  element has. A `$variable` is a `SyntaxError`: nothing binds one. An attribute found is the `Attr` its element
  has (`attributes`, `getAttributeNode`), a document a script made or an `XMLHttpRequest` answered is walked from
  its own root, and a resolver is a function or anything with a `lookupNamespaceURI`, a node included. What
  differs from a browser: the doctype is no node (`/node()` is the `html` element alone); a string is measured in
  characters, not in UTF-16 units; a number is written without an exponent, however large; the `result` argument
  is never reused; an iterator goes invalid on a change made
  through the DOM to the tree it was evaluated in. Each node looked at is a call into light, so XPath is slower
  than in a browser: `//div[@class="x"]//a[position() < 3]` over a page of 10,000 nodes takes about 20 ms, and an
  expression that must walk every text node about 120 ms. The Web Platform Tests of `domxpath/` are in
  `task wpt`'s subset.
- **Attributes** are nodes: an `Attr` is the same object each time its element gives it (`attributes[i]`,
  `getAttributeNode`, `getAttributeNodeNS`) for as long as it is on it, with `ownerElement`, `ownerDocument`,
  `namespaceURI`, `prefix`, `localName`, `name`, `value` and `specified`; `createAttribute` and
  `createAttributeNS` make one, `setAttributeNode`, `setAttributeNodeNS` and `removeAttributeNode` move it (an
  `InUseAttributeError` for one another element has), and `attributes` is a whole `NamedNodeMap` (by index, by
  name, `getNamedItem`, `setNamedItem`, `removeNamedItem` and their `NS` forms). Attributes are told apart by
  namespace and local name, `getAttribute` and `setAttribute` reach the first of a qualified name, and a
  mutation record names the namespace. Only an attribute without a namespace is the element's own (`style`,
  `src`, `value`…).
- **Token lists**: `classList`, a link's, an anchor's and a form's `relList`, a link's `sizes`, an iframe's
  `sandbox` and an output's `htmlFor` are one `DOMTokenList` each, the same object at every read, live with its
  attribute both ways, to the DOM standard: the ordered set, `add`, `remove`, `toggle`, `replace`, `contains`,
  `item` and indexes, `length`, `value`, `supports` (a `TypeError` where the attribute has no set of known
  tokens), `forEach`, `keys`, `values`, `entries`, a `SyntaxError` for an empty token and an
  `InvalidCharacterError` for one with a space, and `el.classList = "a b"` sets its value.
- **Reflected attributes**: the properties of HTML's elements that stand for an attribute come from one table
  (`internal/dom/js/36-reflection.js`), by kind as HTML's "reflect" rules say: text, a URL resolved against the
  document, a flag, a signed or unsigned number with its range, default and fallback, a number clamped to a
  range, a real number, and a keyword of a known set with its missing and invalid defaults. The global ones are
  on `HTMLElement`, the others on each element's interface, obsolete ones included (`HTMLMarqueeElement`,
  `HTMLFrameSetElement`, `HTMLDirectoryElement`…); an unknown name is an `HTMLUnknownElement`.
- **IndexedDB** is the Indexed Database API 3.0: `indexedDB` (`open`, `deleteDatabase`, `databases()`, `cmp`),
  `IDBDatabase`, `IDBTransaction` (`commit()`, `abort()`, `durability`), `IDBObjectStore`, `IDBIndex` (unique,
  `multiEntry`), `IDBCursor` and `IDBCursorWithValue` in the four directions, `IDBKeyRange`, the requests and
  their events, key paths, key generators, and the standard's `DOMException` names. A transaction is active
  during its requests' callbacks and the promises they settle, commits by itself once none is left, and two
  that write the same stores run one after the other, in the order they were made, across tabs and windows;
  `versionchange` and `blocked` cross them too. The stores, their order, the indexes and the transactions are Go
  (`pkg/idb`); the script's side converts keys and copies values. What it is not:
  - Values are stored by light's own structured clone (Messages, above), so what it does not copy is a
    `DataCloneError`: `ImageData`, `ImageBitmap`, `FileList`, `CryptoKey`, the geometry types (`DOMRect`,
    `DOMPoint`, `DOMMatrix`…), `SharedArrayBuffer`, WebAssembly modules, and an object of a class comes back a
    plain object, as the standard says. A `Blob` or a `File` is stored with its bytes.
  - On disk, a site (the key its `localStorage` is kept under) is one file of `indexeddb/` in light's folder,
    named by a hash of that key: a header, then one checksummed record a committed transaction, appended off the
    frame, and the whole file written again (to a temporary file, then renamed) once it holds over twice what
    the site does. A file cut short is read up to the cut. `complete` is told once the transaction is committed
    in memory, before its record is on disk, whatever its `durability`, which is kept but changes nothing: a
    crash can lose the last transactions (light forces nothing to the disk), never half of one. A document of no
    site (a file, `about:blank`) has databases of its own that last as long as it.
  - A site's databases are all read into memory when one of its pages first opens one, on the frame, and stay
    there for the run. A site holds 128 MB at most, keys and indexes counted: past it a request fails with a
    `QuotaExceededError`, which aborts its transaction. `navigator.storage.estimate()` tells the bytes the
    site's databases hold (not its `localStorage` or cookies) and that quota; `persisted()` and `persist()`
    answer false, since "Clear databases" in the settings forgets every site's, and nothing else evicts them.
  - The events are tasks run after the frame's timers, each with its own microtasks, for 8 ms a frame at most.
    A tab that is not shown runs nothing, so it answers a `versionchange` only once shown or closed, and the
    upgrade it blocks waits (`blocked` is told as the standard says).
  - Not there: IndexedDB in workers (light has none), `IDBObjectStore.getAllRecords()` and the options form of
    `getAll()` (later drafts), `storage.getDirectory()`. Of the `IndexedDB/` directory of the web platform tests,
    172 of the 229 tests in a window pass ([the result](#the-result-of-20-november-2026)).
- **Scrolling** from a script (`scrollTo`, `scrollX`…), `IntersectionObserver` and `ResizeObserver`
  (each tells once, as if everything were in view), and the observers of performance.
- **Keys**: in a focused text field, scripts hear typed characters and Enter, not the other keys. A key event
  says the modifiers held with it (`shiftKey`, `altKey`, `ctrlKey`, `metaKey`, `getModifierState()`), so a page
  that sends on Enter lets Shift+Enter break a textarea's line. Cmd+Enter (Ctrl+Enter off macOS) in a textarea
  submits its form, as ember's field does, unless a script prevents its keydown.
- A script that runs long is not stopped in a person's light, which [asks](#a-page-that-is-slow); in a hidden or
  a driven run it is stopped after 2 seconds (`--script-budget` of `light drive`) and reported in the Console. A
  page's timers get 8 ms a frame; what is left waits for the next. A page holds 500,000 nodes at most.
- Content security policies are not read: a page's scripts run whatever its policy says.
- A page with scripts is not kept when the tab leaves it: Back fetches it again. The scripts of a background tab
  wait until it is shown.

### WebDriver

light can be driven by a test runner over [W3C WebDriver](https://www.w3.org/TR/webdriver2/), the protocol the
web-platform-tests runner speaks. That is a command of its own, `light drive`; a light started any other way
serves nothing:

```sh
task run -- drive --address 127.0.0.1:4444              # or LIGHT_WEBDRIVER=127.0.0.1:4444 task run -- drive
EMBER_HIDDEN=1 go run -tags v8 ./cmd/light drive --address 127.0.0.1:4444   # without a window on screen
```

The mode is the command: `LIGHT_WEBDRIVER` only names the address of a `light drive`, and a light started without
that command is a person's browser whatever the variable says. `light drive` takes the options of any light
(`--dir`, `--downloads`, an address to open) and those below, which no other light takes.

Whoever reaches that port drives the browser, so:

- It listens on this machine only: `127.0.0.1`, `::1` or `localhost` with a port. Any other address is refused at
  launch, and light does not start.
- A request whose `Host` is not a name or an address of this machine is answered 403, so a page that makes another
  name resolve here does not reach it. So is a request carrying an `Origin` header: a WebDriver client sends none, a
  page's script does.
- A driven light keeps its settings, history, cookies and what pages store in memory, and touches none of yours,
  unless `--dir` names a folder.
- `navigator.webdriver` is true for as long as light runs.
- `light drive` draws the text of pages at its exact coverage, as under `-webkit-font-smoothing: antialiased`,
  without the stem darkening a person's light draws it with: reference tests compare pictures pixel by pixel, and a
  darkened glyph is wider than the box it is compared with. A page that sets the property itself keeps its own, and
  light's own header and tools are drawn as ever. A hidden run or a capture of a light that browses draws its pages
  as a person's window does.
- A navigation asked over WebDriver is an address typed: `Sec-Fetch-Site: none`, `Sec-Fetch-User: ?1`, no `Referer`,
  and it may reach this machine and the local network as you may.

There is one session at a time. It begins on `about:blank`, an empty page scripts run in, in the shown tab of the
first window; a window handle is a tab. Deleting the session, or closing its last window, leaves light with one
window of one tab on its start page, ready for the next. Cookies and storage are not wiped between sessions: start
light again for a clean profile.

Elements and scripts need the JavaScript engine (`-tags v8`): without it those commands answer `unsupported
operation` and say so, while sessions, navigation, windows, cookies, the page's source and title, pointer and key
actions and screenshots work.

| Command | |
|---------|---|
| Status, New Session, Delete Session | Yes. Capabilities are matched as the standard says: `browserName` `light`, `browserVersion`, `platformName` (`mac`, `linux`, `windows`), `pageLoadStrategy`, `timeouts`, `unhandledPromptBehavior` (a word or one a kind of prompt, see the alert commands), `setWindowRect` true. `acceptInsecureCerts: true`, a `proxy` and `webSocketUrl: true` match nothing: light checks every certificate, takes no proxy this way and has no WebDriver BiDi. |
| Get and Set Timeouts | Yes: `script` (null for none), `pageLoad`, `implicit`. |
| Navigate To, Get Current URL, Back, Forward, Refresh, Get Title, Get Page Source | Yes. A navigation waits by the page load strategy: `normal` and `eager` for the page shown and its scripts run, `none` for nothing. Pictures and fonts may land after. A page that is not reached is `unknown error` with light's reason; a load longer than `pageLoad` is `timeout`. The source is the document as it stands with the engine, as it was sent without. |
| Get Window Handle and Handles, Switch To Window, Close Window, New Window | Yes. A new `tab` opens behind the session's; Switch To Window shows it. `window` opens a window. |
| Get and Set Window Rect, Maximize, Minimize, Fullscreen | Yes. The rect is the window's content area in points, header included: `outerWidth - innerWidth` and `outerHeight - innerHeight` tell a runner how much to add for a view of a given size. A window is no smaller than light allows. |
| Switch To Frame, Switch To Parent Frame | Yes, with the engine, for frames that run scripts: by index, by element, `null` for the top. |
| Find Element(s), from the document or an element | `css selector`, `tag name`, `link text`, `partial link text` and `xpath` (XPath 1.0, see [Limits of its JavaScript](#limits-of-its-javascript)), waited for as long as `implicit`. An `xpath` that is no expression, or that selects what is not an element, is `invalid selector`. |
| Get Active Element; Get Element Attribute, Property, CSS Value, Text, Tag Name, Rect; Is Element Enabled, Selected | Yes. Text is the element's text with its spaces collapsed, not the standard's rendered text. |
| Is Element Displayed | In part: false under `display: none`, `hidden`, or with `visibility: hidden`; no check of size, opacity or what covers it. |
| Element Click | The element is scrolled into view and the window's own pointer presses its middle; a navigation the click begins is waited for. No check that something else is over it. |
| Element Clear, Element Send Keys | Yes, through the window's keyboard, so the page hears its events. Clear selects the field's text and deletes it. No file upload. |
| Execute Script, Execute Async Script | With the engine. Arguments and results as the standard copies them (elements as references, collections as lists, `undefined` as null), a promise returned awaited, a throw or a rejection `javascript error` with its message and stack, `script timeout` after the session's. A run without a pause is also stopped at the script budget (2 seconds, or `--script-budget`). An async script goes on across frames of light, not across a navigation of its own document. |
| Get All Cookies, Get Named Cookie, Add Cookie, Delete Cookie, Delete All Cookies | Yes, over the profile's one jar, for the address of the session's page. A cookie the jar's rules refuse is `unable to set cookie`. |
| Perform Actions, Release Actions | `key`, `pointer` (mouse; move to the view, the pointer or an element, down, up) and `pause`, tick by tick through the window's input; `wheel` scrolls by its pixels at once, as a trackpad does. Touch and pen pointers act as the mouse. A move is one jump, whatever its duration. |
| Dismiss Alert, Accept Alert, Get and Send Alert Text | Yes, with the engine, over the dialog the session's window shows (see "What a page asks"); `no such alert` with none, `element not interactable` for a text sent to an `alert` or a `confirm`. A script run by Execute Script that opens one is answered null at once and goes on when the prompt is answered. Any other command that meets an open prompt handles it as `unhandledPromptBehavior` says (`dismiss and notify` by default, `dismiss`, `accept`, `accept and notify`, `ignore`), failing with `unexpected alert open` and the prompt's text in `data.text` where it notifies; Status, the session, timeouts and window commands leave it alone. |
| Take Screenshot | The page's view, without light's header, as a PNG of one pixel a CSS pixel: an 800 by 600 view gives 800 by 600 pixels on a display of any scale, each the mean of the device pixels it covers, which is what reference tests compare. Taken from the window's own next frame, hidden or not. Not on light's own pages. |
| Take Element Screenshot | The part of the element in the view, the same way. |
| Get Element Shadow Root, Find From Shadow Root | `no such shadow root`: light has no shadow tree. |
| Get Computed Role and Label, Print Page, anything else | `unknown command`. |

With a window on screen, your own mouse and keys reach the page too.

Three more options are a test runner's, so they are `light drive`'s alone and unknown to a light that browses:

- `--host-map "web-platform.test=127.0.0.1;not-web-platform.test=127.0.0.1"`: names, and every name under them,
  that light finds at an address of this machine, where it dials. The system's hosts file is not read for them and
  never written. Only `127.0.0.1` and `::1` are taken. The [local network](#what-it-does-not-do) rule is unchanged:
  it looks at the address a name was dialled at, so a page of the Internet still does not reach a mapped name.
- `--trust-ca FILE`: a certificate authority (PEM) trusted beside the system's, by this run's requests only. Every
  certificate is still checked, against its name and its dates; nothing is added to the system's trust store.
- `--script-budget DURATION` (`10s`): how long one script of a page may run before light stops it, in place of 2
  seconds. A test is one long task more often than a page is. A driven light always stops a script at this
  budget, without asking; a person's light [asks instead](#a-page-that-is-slow).

### Web platform tests

`task wpt` runs a subset of the [web-platform-tests](https://web-platform-tests.org) against light, with
their own runner (`wpt run`) driving light over WebDriver. The subset (`tools/wpt/subset.txt`) is the whole of
`dom/`, `domxpath/`, `url/`, `domparsing/`, `IndexedDB/`, `cors/`, `console/` and `hr-time/`, most of `encoding/`
and `html/syntax/`, parts of `html/semantics/`, `css/` and `fetch/api/`, and a few tests of `html/dom/` and
`css/CSS2/`. Within those, a test is left out only when it needs what light does not have (workers, shadow trees,
custom elements, `window.open`, audio and video, WebSockets, `document.write`, frames of `blob:` addresses,
`Observable`, `OpaqueRange`, scrolling by script, touch and wheel input over WebDriver) or stops light: the file
names each one under its reason, and what each directory takes and leaves out.

```sh
task wpt                                          # the tests of tools/wpt/subset.txt
task wpt -- /dom/nodes/Node-contains.html         # the tests given instead (addresses, or folders)
WPT_OUT=/tmp/report task wpt                      # the report elsewhere than .wpt/results
```

It builds light with its JavaScript engine into `.wpt/light` and runs it hidden (`EMBER_HIDDEN=1`), one test at a
time, in one light for the script tests and one for the reference tests. The report is `wptreport.json` (the format
wpt.fyi reads), `raw.log` (every event, the screenshots of a failed reference test included) and `summary.txt`: a
line a test, with its status and its subtests passed over all. No result fails the task yet: there is no list of
expected results to compare with.

`task wpt:triage` reads that report and says what to fix first (`tools/wpttriage`). Each failing test and subtest
gets a cause: a missing API (`x.foo is not a function`, `Foo is not defined`, a member idlharness finds missing),
another exception, a harness error, a timeout, a crash, a reference mismatch, a computed value or a value a CSS
parsing test expects, or an assertion with what it checks; quoted values and numbers are elided so alike failures
meet. A missing API is one cause wherever it fails; the others are counted per directory (`/css/css-flexbox`,
`/html/semantics/forms`). The causes are ranked by the tests that fail for that cause alone, which fixing it would
make pass, then by the subtests it fails; `triage.md` shows the first 60 (`-top`), with their directories, three
tests each and a guess of where the fix lives, and `triage.json` holds them all.

#### The result of 20 November 2026

`task wpt` on light `c02bb21` with this subset, ember `3f64950` and web-platform-tests `d6179d3` (the commit
`wpt.lock` pins), on macOS 26.6 (arm64), took 81 minutes, `.wpt/` already set up. A test passes when its harness is
OK and every subtest passes, or, for a reference test, when it matches its reference:

| Directory | Tests passed | Subtests passed |
|---|---|---|
| `dom` | 408 / 566 | 61,233 / 63,966 |
| `domxpath` | 30 / 35 | 1,158 / 1,171 |
| `html` | 560 / 1,686 | 67,946 / 74,690 |
| `css` | 1,621 / 3,876 | 7,115 / 34,893 |
| `url` | 8 / 50 | 3,341 / 10,185 |
| `encoding` | 12 / 68 | 3,605 / 12,843 |
| `domparsing` | 22 / 97 | 422 / 2,644 |
| `fetch` | 29 / 133 | 856 / 1,683 |
| `IndexedDB` | 172 / 229 | 1,245 / 1,601 |
| `cors` | 9 / 26 | 254 / 414 |
| `console` | 3 / 12 | 38 / 56 |
| `hr-time` | 2 / 13 | 27 / 58 |
| **Total** | **2,876 / 6,791 (42%)** | **147,240 / 204,204 (72.1%)** |

Of the 6,791 tests, 4,044 script tests ran to the end (OK), 1,446 reference tests passed and 960 failed, 149 tests
stopped on an error and 192 timed out; a test that times out counts the subtests it reached. Read the figures for
what they are:

- They are this subset's, not a pass rate of the web-platform-tests: twelve directories, most of them in part.
  Nothing of the others (`xhr`, `webstorage`, `cookies`, `FileAPI`, `uievents`, `service-workers`, `websockets`,
  `webaudio`…) is counted.
- Directories or parts were added in this order of worth until a whole run took about 75 minutes (81, in the
  end); the rest was left out for time alone, not for its results: `html/semantics/` `document-metadata`,
  `links` and `embedded-content` (557 tests, many of them waiting out a timeout), `css/selectors/` (821), `xhr/`
  (356), `webstorage/` (54), `cookies/` (89), `FileAPI/` (70), `uievents/` (74), and the 1,200 legacy
  multi-byte tests of `encoding/` (a minute each). Also out: `html/semantics/` media, audio, video and canvas
  (light has none of them), popovers, `interestfor`, `menu` and `permission-element`; `html/syntax/`
  `speculative-parsing` (tentative); `css-grid/grid-lanes` (the masonry layout); `fetch/api/` but `abort`,
  `basic`, `body`, `headers`, `request` and `response`.
- Within what is run, 469 tests are left out by name for what light does not have (`dom/` 247, the others 222,
  the 173 parser tests through `document.write` or a `blob:` frame among them), and the 1,023 variants of a test
  that run in a worker are not listed. The two crash tests of `domxpath/` are listed and not run, as light's
  product runs only script and reference tests.
- One test is left out because it stops light: `html/semantics/forms/the-input-element/image-click-form-data.html`,
  where light's WebDriver stops answering after the click on an image button in a frame and the runner has to
  restart light.
- Subtests weigh unevenly: the nine `html/dom/reflection-*` tests are 59,722 of them, a few `dom/ranges` tests
  thousands each, `url/` and `encoding/` tables thousands more. Without `html/dom/reflection-*`, 87,518 of
  144,482 subtests pass (60.6%).

The figures, the commits and the command are kept in `tools/wpt/results/2026-11-20.json`, and the line a test of
that run in `tools/wpt/results/2026-11-20.txt`. To run it again, check out those commits (ember beside light in the
workspace) and run `task wpt`; a new dated result goes beside it.

Earlier: on 19 November 2026, 461 of 625 tests and 122,244 of 124,986 subtests passed, on a subset of `dom/`,
`domxpath/` and 24 other tests (`tools/wpt/results/2026-11-19.json`).

The first run sets up `.wpt/`, which git ignores (`task wpt:setup` alone does only that; about 400 MB, 510 MB with
the light it builds):

- `.wpt/wpt`: a shallow clone of `github.com/web-platform-tests/wpt` at the commit `wpt.lock` pins, with only the
  folders `tools/wpt/sparse.txt` names. A test of another folder needs its folder there first, and what it loads.
- `.wpt/wpt/_venv3`: the virtualenv `wpt` makes for itself with `python3`, with the requirements it installs from
  PyPI and light's product for its runner (`tools/wpt`, a small package registered as the `wptrunner.products`
  entry point `light`). Nothing is installed outside that folder, no browser is downloaded, and the clone is left
  as it was fetched.

The tests' servers listen on this machine only, under the names `web-platform.test` and `not-web-platform.test`:
the product starts `light drive` with `--host-map` for them and `--trust-ca` for the authority of the tests'
certificates (`.wpt/wpt/tools/certs/cacert.pem`), so neither `/etc/hosts` nor the keychain is changed. `wpt run`
refuses a product it does not know until `/etc/hosts` names its servers; the product skips that one check for light.

To move to a newer commit, write it in `wpt.lock`; `rm -rf .wpt` removes everything.

### Under the hood

What is borrowed, in full:

| Part | From |
|---|---|
| JavaScript engine | V8, through [gost-dom/v8go](https://github.com/gost-dom/v8go) (prebuilt V8 libraries from [tommie/v8go](https://github.com/tommie/v8go)) |
| HTML tokenizer and tree builder | [golang.org/x/net/html](https://pkg.go.dev/golang.org/x/net/html) |
| GPU access | [wgpu-native](https://github.com/gfx-rs/wgpu-native), loaded at run time, through ember's own binding |
| Window, keyboard and mouse | [GLFW](https://www.glfw.org) 3.3, through [go-gl/glfw](https://github.com/go-gl/glfw) |
| HTTP, TLS, gzip, PNG, JPEG and GIF decoding | Go's standard library |
| WebP decoding, font file reading and rasterising | [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) |
| Brotli decoding | [andybalholm/brotli](https://github.com/andybalholm/brotli) |
| Grapheme clusters | [rivo/uniseg](https://github.com/rivo/uniseg), in ember |
| Fonts | The Go fonts and Source Code Pro, embedded in ember; the fonts installed on the system; the fonts pages bring |
| Command line | [alecthomas/kong](https://github.com/alecthomas/kong) |
| The optional model | [llama.cpp](https://github.com/ggml-org/llama.cpp) through [yzma](https://github.com/hybridgroup/yzma), in a separate program, downloaded only if you turn it on |

Everything else is this repository and ember.

A page goes through four steps:

1. **Fetch**: the address, through its redirects; the answer is shown by its type.
2. **Read the HTML** with [golang.org/x/net/html](https://pkg.go.dev/golang.org/x/net/html). ember has no HTML parser.
3. **Build**: each element becomes the ember element of the same name (`ui.Div`, `ui.P`, `ui.Img`…), with its id,
   classes, inline style and attributes; each `<link rel="stylesheet">` is fetched meanwhile.
4. **Style and draw**: the sheets go to ember as they are. ember parses the CSS, lays the page out and draws it.

What light changes in a page before showing it is one short table in `internal/web/adaptations.go`. light shows it
at `light://about`.

Every request goes through one `http.RoundTripper` (`pkg/netlog`), which is how the developer tools see them all,
wrapped in another (`pkg/cookies`) that adds the cookies of the one jar and takes those each answer sets, a redirect's
hops each on their own. A request's context says which page it is for (`cookies.Within`), or which page a
navigation began on (`cookies.From`): that is how a third party and a cross-site request are known. Under them
all, `pkg/localnet` opens the connections of a page's requests (`localnet.From`) only to addresses the page may
reach: the pages that came from the Internet share one pool of connections, which open only to the Internet, and
those of the local network another. A request of the user's own (an address typed) may reach every address, and
goes through the most public of those pools that reaches where it goes, so that the page it brings asks through
the connection it opened; a connection to this machine is never in a pool a page of the web asks through.

| Package | What it holds |
|---------|---------------|
| `cmd/light` | The entry: the command line (`main.go`), the options its commands share (`globals`: the folders and their overrides) and the commands (`commands`: `browse`, the default, and `drive`), which make the profile, the first window and the World. |
| `light` (the root) | The logo, embedded. |
| `internal/app` | The profile the windows share (settings, history, cookies, downloads, the files kept) and each window's screen: header, navigation, status bubble, light's own pages, the pages of the other kinds, the history, downloads and settings pages, the context menu, shortcuts. |
| `internal/web` | An address turned into a page: the answer read by its kind, HTML built as a ui Document, and every adaptation made to it. |
| `internal/menu` | The context menu as a ui element. |
| `internal/dom` | The DOM a page's scripts see: a JavaScript prelude over one Go function that reads and changes the parser's tree, the ui Document following what it shows of it. |
| `internal/devtools` | The developer tools' panel as ui elements. |
| `internal/shell` | The system's command that opens a file or shows it in its folder. |
| `pkg/tabs` | Plain Go, no ember: tabs, each one's back and forward, what a typed address means, the `light://` addresses. |
| `pkg/history` | Plain Go: the visits, by day, filtered, capped, and their file. |
| `pkg/downloads` | Plain Go: the downloads, each one's state and progress, free file names, partial files. |
| `pkg/config` | Plain Go: the settings, their defaults and bounds, and their file. |
| `pkg/search` | Plain Go: the search engines, one row each, and the address of what one finds. |
| `pkg/content` | Plain Go: which kind of answer a media type is (one table), file names for saving. |
| `pkg/fetch` | Plain Go: one GET with redirects, limits, charset and the Refresh header, read whole or as a stream. |
| `pkg/netlog` | Plain Go: the record of a tab's requests, and the transport that fills it. |
| `pkg/har` | Plain Go: a tab's requests as a HAR 1.2 file, sanitized or not, and the check that a file is one. |
| `pkg/script` | The JavaScript engine's boundary: V8 with the `v8` tag, nothing without. No ember. |
| `pkg/console` | Plain Go: what a tab's scripts said, capped, a line said again counted, filtered by level and text. |
| `pkg/instant` | Plain Go: the clock the console, the requests and the loads are timed by, each instant after the last, so they sort in the order they happened on a coarse clock. |
| `pkg/storage` | Plain Go: what pages keep by origin, and its file. |
| `pkg/idb` | Plain Go: the IndexedDB databases by site: keys and their order, stores, indexes, cursors, transactions and their scheduling, connections and upgrades, the quota, and their files. |
| `pkg/cookies` | Plain Go: the cookie jar, its rules, the transport that applies them, and its file. |
| `pkg/localnet` | Plain Go: the space of an address (public, local, loopback), and the transport that keeps pages from more private ones. |
| `pkg/webdriver` | Plain Go: the WebDriver server: its commands, sessions, capabilities, errors and JSON, over the `Browser` light implements. |
| `pkg/model` | Plain Go: the language model that runs on this device, behind one interface: its states, requests in a queue, the list of models and their installation (downloads checked against a pinned manifest), the helper program it starts and watches, and a fake for tests. |
| `helper/model` | A Go module of its own, not part of light's: `light-model`, the program that runs the model with llama.cpp through yzma. |
| `pkg/bookmarks` | Plain Go: the bookmarks, each page at most once, the folders they are filed in, and their file. |
| `pkg/identity` | Plain Go: what light tells a site: the User-Agent, the languages, and the headers of each request. |
| `pkg/assist` | Plain Go: which features the model helps with, and their file. |
| `pkg/categories` | Plain Go: the categories a reader sorts pages into, what each does with its pages, and what was chosen for a site or a page. |
| `pkg/recall` | Plain Go: what the pages visited said, as passages and their vectors, searched by a question's vector, and its file. |
| `pkg/testnet` | Plain Go: the network of a driven run: names resolved to this machine, and a certificate authority trusted by that run alone. |
| `internal/capture` | A light with a model that answers what it is told to, for pictures of the model's surfaces; never part of a build. |
| `pkg/frames` | Plain Go: the rules of frames: how many and how deep, what an `iframe` shows, its sandbox, whether a document lets itself be framed, where a target leads, which site its cookies are kept under, what the window of another frame answers, whom a message reaches and who may send a window elsewhere. |

Dependencies go one way: `cmd` → `internal` → `pkg`.
