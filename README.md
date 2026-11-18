<div align="center">

# <img src="logo.svg" width="36" align="absmiddle" alt=""> light

**A small web browser, made as a demo of [ember](https://github.com/lantervale/ember).**

[![Go 1.27](https://img.shields.io/badge/go-1.27-00ADD8?logo=go&logoColor=white)](go.mod)
[![MIT](https://img.shields.io/badge/licence-MIT-green)](LICENSE)
[![Status: demo](https://img.shields.io/badge/status-demo-orange)](#what-it-does-not-do)
[![Made with ember](https://img.shields.io/badge/made%20with-ember-orange)](https://github.com/lantervale/ember)

<img src=".github/readme/zen-garden.png" width="640" alt="light showing CSS Zen Garden: tabs in the title bar, a navigation row, the page below">

<sub>[CSS Zen Garden 001](https://csszengarden.com/001/), fetched from the web and drawn by ember.</sub>

</div>

ember is a 2D game engine in plain Go. Its UI is HTML elements built from Go and styled by a CSS subset. This demo asks
how far that UI goes on pages written for real browsers: it fetches a page, turns it into ember elements and draws it.

It is a demo, not a browser to live in: see [what it does not do](#what-it-does-not-do).

## Highlights

- 🗂️ **Tabs in the title bar**: the header sits where the system's title bar is, as a browser's does (macOS).
- 🧭 **Navigation**: back, forward, reload and stop, an address field, a list of the open tabs, favicons.
- 🌐 **Real pages**: an `http` or `https` address is fetched with its stylesheets and pictures, through its redirects.
- 🧾 **Other kinds of answer**: a picture alone on a dark page (PNG, JPEG, GIF, SVG, an ICO holding a PNG; a click
  switches between fitted and its own size), text kinds (CSS, JavaScript, JSON, XML, plain text…) as they were sent.
  The rest (PDF, archives, audio, video, fonts, WebP, AVIF…) is downloaded.
- ⬇️ **Downloads**: a button in the navigation row with the progress and the latest files, and `light://downloads`
  with all of them: open, show in folder, cancel, retry, remove. A link with `download`, "Save link as…",
  "Save image as…" and "Save page as…" go the same way.
- ⚙️ **Settings**: `light://config` sets the start page, the dark mode, the downloads folder, the history, the zoom,
  pictures, timeouts, redirects and the User-Agent. They apply at once and are kept between runs.
- 🌙 **Dark mode**: light's own pages on black, the logo's navy and white swapped, and "light" struck through. A
  click on the start page's logo switches it; pages of the web stay as their sites made them.
- 🏠 **Its own pages**: `light://home`, `guide`, `examples`, `about`, `history`, `downloads` and `config`.
- 🕘 **History**: `light://history` lists every visit by day, filtered as you type, kept between runs.
- 🖱️ **Right click**: a menu for what is under the pointer — a link, a picture, selected text, a text field, the
  page, a tab, a request.
- 📄 **View source**: `view-source:<address>`, with line numbers.
- 🛠️ **Developer tools**: a Network tab listing every request of the tab (each redirect hop, sheets, pictures,
  favicon) with its status, type, size, time, headers and response, and a Warnings tab with what ember's ui refused.
- 🔍 **Zoom**, from 80 % to 200 %, and **text you can select** and copy.
- 🧱 **All of it is ember**: the header, the menus, the tools and the page are plain ui elements and stylesheets.

| | |
|---|---|
| <img src=".github/readme/home.png" width="400" alt="light's start page: its logo, a search field and eight tiles"> | <img src=".github/readme/developer-tools.png" width="400" alt="The developer tools under CSS Zen Garden: the requests of the tab and the details of one"> |
| <img src=".github/readme/settings.png" width="400" alt="The settings page: the start page, the downloads folder, the history, the zoom"> | <img src=".github/readme/downloads.png" width="400" alt="The downloads page: one file failed, one done, one in progress"> |
| <img src=".github/readme/history.png" width="400" alt="The history page: visits of a day, newest first"> | <img src=".github/readme/context-menu.png" width="400" alt="The menu of a right click on a link"> |
| <img src=".github/readme/view-source.png" width="400" alt="The source of a page, its lines numbered"> | <img src=".github/readme/picture.png" width="400" alt="A JPEG shown alone on a dark page"> |

## Get started

You need Go 1.27, [Task](https://taskfile.dev) (`brew install go-task`) and a
[WebGPU-capable GPU](https://github.com/lantervale/ember/blob/main/docs/01-getting-started/README.md).

light builds on the ember cloned beside it, through a Go workspace:

**1.** Clone ember and light side by side, and tie them:

```sh
mkdir lantervale && cd lantervale
git clone https://github.com/lantervale/ember.git ember
git clone https://github.com/lantervale/light.git light
go work init ./ember ./light
```

**2.** Fetch ember's WebGPU library (one time only):

```sh
(cd ember && task wgpu)
```

**3.** Run it, on the start page or at an address:

```sh
cd light
go run ./cmd/light
go run ./cmd/light https://csszengarden.com
```

The start page has a tile for CSS Zen Garden; any address typed in the field works the same way.

> ⚠️ light does not build against ember's `main` yet: it uses parts of ember's ui that have not landed there
> so far. Until they do, `go.mod` pins an older commit and the steps above fail at the build.

## Shortcuts

Cmd is Command on macOS and Control elsewhere.

| Keys | What they do |
|------|--------------|
| Cmd+T, Cmd+W | A new tab, close the tab |
| Cmd+L | The address field |
| Cmd+R | Reload, or stop a page that is loading |
| Cmd+[ and Cmd+] (Alt+Left and Alt+Right off macOS) | Back, forward |
| Cmd+Y on macOS, Ctrl+H elsewhere | The history |
| Cmd+Shift+J on macOS, Ctrl+J elsewhere | The downloads |
| Cmd+, | The settings |
| Cmd++, Cmd+-, Cmd+0 | Zoom in, zoom out, the actual size |
| Cmd+Alt+U on macOS, Ctrl+U elsewhere | The page's source, in a new tab |
| F12, or Cmd+Alt+I on macOS, Ctrl+Shift+I elsewhere | The developer tools |
| Right click | The menu of what is under the pointer; arrows, Enter and Escape work in it |

## Settings

`light://config` (Cmd+,) shows them; a change applies at once and is written to `config.json`. A value out of its
bounds, or a file that does not read, falls back to the default, setting by setting.

| Setting | Default | Overridden by |
|---------|---------|---------------|
| Start page (a new tab, the home button) | `light://home` | The address argument, at launch only |
| Dark mode (light's own pages; a click on the start page's logo switches it too) | Off | |
| Downloads folder | The system's (`~/Downloads`) | `LIGHT_DOWNLOADS` |
| Keep the pages visited | On | |
| Visits kept (100 to 100,000) | 2,000 | |
| Zoom of web pages (80 to 200 %) | 100 % | |
| Load pictures | On | |
| Follow a page that refreshes elsewhere at once | On | |
| Request timeout (1 to 300 s) | 30 s | |
| Redirects followed (0 to 20) | 10 | |
| User-Agent | `light/1.0 (ember)` | |
| Preserve the Network log in new tabs | Off | |
| Requests kept in each tab's Network panel (100 to 10,000) | 1,000 | |

Pictures and refreshes apply to pages loaded afterwards. The page also clears the history and resets every setting.
`LIGHT_DIR` names the folder the settings are kept in (see [below](#where-things-are-kept)).

## Downloads

- What light cannot show is downloaded, and so is a link with a `download` attribute, an answer sent as an
  attachment, and what "Save link as…", "Save image as…" and "Save page as…" name.
- A file goes to the downloads folder under a free name: light never writes over a file. It is written as
  `<name>.lightdownload` and takes its name once whole; a cancelled or failed download leaves nothing behind.
- **Downloaded files are not scanned**, and light never opens or runs one by itself. "Open" and "Show in folder"
  act only on your click, only on a download of light's that is complete, through the system's own opener.
- There is no size cap, no pause and no resume: a download that failed is fetched again from its start by "Retry".
- A navigation that turns out to be a download is fetched twice: once to learn its kind, once to save it.
- The list is kept between runs, 500 entries at most. A download that a quit interrupted shows as failed.

## Where things are kept

`history.json`, `downloads.json` and `config.json` are in light's folder of your config folder
(`~/Library/Application Support/light` on macOS). `LIGHT_DIR` names another folder. A hidden run (`EMBER_HIDDEN`)
not given one keeps them in memory.

## What it does not do

- It runs no script. A page that builds itself in JavaScript shows little, and fonts a script or an `@font-face`
  loads are replaced (an icon font shows its letters).
- It sends no form, keeps no cookie and no credentials: a page that asks to sign in (401, 407) says so and stops.
- It has no cache: every page shown for the first time in a tab is fetched, and nothing is revalidated (no 304).
- It follows redirects (301, 302, 303, 307, 308) and a refresh of 0 seconds, as many in a row as the settings say,
  to `http` and `https` only. A refresh that waits is left alone.
- It reads gzip only: no Brotli, no zstd, and it does not ask for them.
- It holds a page's answer in memory, 64 MB at most, and lays out the first 512 kB of a text. A download is
  streamed to its file instead.
- It decodes PNG, JPEG, GIF and SVG. WebP, AVIF and BMP are downloaded.
- It does not scan what it downloads, and neither pauses nor resumes a download.
- It zooms web pages only, not its own pages. The keys zoom one tab; the setting is the zoom of the others.
- It colours no syntax in a source, and completes no address from the history.
- A Control+click is not a right click on macOS.
- It draws what ember's ui draws. What is missing is listed in ember's guide:
  [What is missing](https://github.com/lantervale/ember/blob/main/docs/05-ui/10-missing.md).

## Under the hood

A page goes through four steps:

1. **Fetch**: the address, through its redirects; the answer is shown by its type.
2. **Read the HTML** with [golang.org/x/net/html](https://pkg.go.dev/golang.org/x/net/html). ember has no HTML parser.
3. **Build**: each element becomes the ember element of the same name (`ui.Div`, `ui.P`, `ui.Img`…), with its id,
   classes, inline style and attributes; each `<link rel="stylesheet">` is fetched meanwhile.
4. **Style and draw**: the sheets go to ember as they are. ember parses the CSS, lays the page out and draws it.

What light changes in a page before showing it is one short table in `internal/web/adaptations.go`. light shows it
at `light://about`.

Every request goes through one `http.RoundTripper` (`pkg/netlog`), which is how the developer tools see them all.

| Package | What it holds |
|---------|---------------|
| `cmd/light` | The entry: the window, the folders and their overrides, the World. |
| `light` (the root) | The logo, embedded. |
| `internal/app` | The screen: header, navigation, status bubble, light's own pages, the pages of the other kinds, the history, downloads and settings pages, the context menu, shortcuts. |
| `internal/web` | An address turned into a page: the answer read by its kind, HTML built as a ui Document, and every adaptation made to it. |
| `internal/menu` | The context menu as a ui element. |
| `internal/devtools` | The developer tools' panel as ui elements. |
| `internal/shell` | The system's command that opens a file or shows it in its folder. |
| `pkg/tabs` | Plain Go, no ember: tabs, each one's back and forward, what a typed address means, the `light://` addresses. |
| `pkg/history` | Plain Go: the visits, by day, filtered, capped, and their file. |
| `pkg/downloads` | Plain Go: the downloads, each one's state and progress, free file names, partial files. |
| `pkg/config` | Plain Go: the settings, their defaults and bounds, and their file. |
| `pkg/content` | Plain Go: which kind of answer a media type is (one table), file names for saving. |
| `pkg/fetch` | Plain Go: one GET with redirects, limits, charset and the Refresh header, read whole or as a stream. |
| `pkg/netlog` | Plain Go: the record of a tab's requests, and the transport that fills it. |

Dependencies go one way: `cmd` → `internal` → `pkg`.

## Development

The tests need no GPU and no network: pages are served by a local test server, and clicks and keys are simulated.
They write nothing in your config or Downloads folders, and open no file: the system's opener is replaced by a fake.

```sh
task test             # go vet ./... and go test ./...
task golangci:lint    # after task golangci:boilerplate, once
```

A screenshot without opening a window:

```sh
EMBER_HIDDEN=1 EMBER_CAPTURE=shot.png EMBER_CAPTURE_FRAME=30 go run ./cmd/light
```

## Licence

Code under [MIT](LICENSE). The pages in the screenshots belong to their authors.
