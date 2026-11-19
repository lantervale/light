# light: the model on this device

light can run a small language model on your machine. It is experimental and **off** by default. Until you turn it
on ("Turn on a model on this device…", under Experimental in `light://config`) and accept the download, nothing of
it is downloaded, started or written.

- **It runs on the device.** The model runs in a separate program, `light-model`, which is llama.cpp loaded
  through yzma. light speaks to it over its standard input and output only: it listens on no port and opens no
  connection. Nothing you ask and nothing of a page leaves the machine.
- **Downloads are pinned.** The llama.cpp libraries come from the project's own release on `github.com`
  (13 to 25 MB) and the model from its publisher's repository on `huggingface.co`, over https with no cookie. A
  file is used only once its size and SHA-256 are the ones this version of light was built with
  (`pkg/model/manifest.json`).
- **Which models.** light proposes Qwen3 4B (2.5 GB) on Apple silicon with 16 GB of memory or more, and Qwen2.5
  1.5B Instruct (1.1 GB) elsewhere. The list also has Granite 3.3 2B Instruct (1.5 GB) and an embedding model,
  Nomic Embed Text v1.5 (146 MB). All are Apache-2.0. A GGUF file of your own can take their place.
- **A page's text is data.** It is given to the model between marks, with the instruction to treat it as data and
  never do what it asks. That lowers the risk of a page steering the model; it does not remove it.
- **Its output can be wrong.** These are small models. What the model wrote is marked with a robot, which also
  takes its part away.
- **It turns off.** Each feature below has its own switch. Turning the model off stops the helper at once, and
  deleting the model removes its whole folder.

<!-- model-features -->
With the model on, it helps with these, each of which can be turned off alone:

- **Answers in the address bar**: a line that starts with `?` is a question about the shown page, answered in a
  card under the address field, with the sentence of the page it comes from selected. Words typed there without
  the `?` get a first row in the list under the field, "Ask this page: …", which asks the same.
- **Translation**: `light://translate` translates a text you type or paste, and is not noted in the history. A
  page in a language you do not read has a label in the address field, "FR → EN", that translates it in place, a
  paragraph at a time, and puts the original back; per language you can ask each time, always translate, or never
  (`translate_always`, `translate_never`).
- **Actions on selected text**: select text by dragging over it with the right button, and a card opens under
  it when you let go, with Explain, Summarise and Translate. A selection made with the left button or the keys
  stays a plain one, and a right click without a drag opens the menu as ever. A translation is into British
  English, American English or French, and the language chosen is kept (`translate_into`).
- **Find by meaning**: Find in page (Cmd+F) works without the model; with a model that embeds text, it also finds
  the passages that mean what you typed when the words are not on the page.
- **Bookmark suggestions**: a name, a line of what the page says and a folder for a bookmark just made, in its
  panel; the panel's robot puts back the page's own title and your usual folder. The line is kept with the
  bookmark and shown on `light://bookmarks`.
- **Page summaries**: a page you leave or close is summed up in one line, kept with its visit in the history and
  shown on its card of the start page; the robot by "Continue" hides the lines (`hide_summaries`).
- **Page categories**: categories you name and describe in your own words, edited on `light://categories`; the
  model judges each page, and a category can mark its pages, warn first with a banner (information, warning or
  danger, with a message of your own if you write one), or dim them. The page's category, or the short tag you
  give it, stands at the end of the address field. A judgment is kept for the page, by its address, for a day
  (`category_hours`); a page not judged yet shows three dots, after the last category of its site if one is known.
  You can correct a page or a whole site.
- **Remember what pages said** (off until you turn it on, apart from the model): the history page then takes a
  question and lists the pages that said something like it. Sites you exclude, local files and light's own
  pages are never remembered, and the page forgets what it holds on request.

Find by meaning and the questions asked of the history need a second, small model that embeds text (Nomic Embed
Text v1.5, 146 MB, Apache-2.0, from `huggingface.co`). The card that proposes the model has a ticked line for it,
"Also download…", with the total to download: untick it and it is not downloaded. A model that is on without it
says so under "It helps with" and offers "Download Nomic Embed Text v1.5 (146 MB)", which asks before it downloads.
Until it is on the device, Find in page finds words only and the history page filters by title and address, with
no question card. "Turn off and delete the model" deletes it too.
<!-- /model-features -->

[The model in detail](reference.md#the-model-in-detail) has the settings, the files and the developer options.
