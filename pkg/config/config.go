// Package config is light's settings, in plain Go: their defaults, the
// bounds each is kept in, and the file they are kept in between runs.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/lantervale/light/pkg/identity"
	"github.com/lantervale/light/pkg/search"
	"github.com/lantervale/light/pkg/tabs"
)

// version is the version of the file's form.
const version = 1

// The bounds of the settings that are numbers.
const (
	MinHistory   = 100
	MaxHistory   = 100000
	MinZoom      = 80
	MaxZoom      = 200
	MinTimeout   = 1
	MaxTimeout   = 300
	MinRedirects = 0
	MaxRedirects = 20
	MinRequests  = 100
	MaxRequests  = 10000
	// A height of the developer tools out of these is their own.
	MinToolsHeight = 120
	MaxToolsHeight = 4000
)

// Config is every setting. The file names each by its json name.
type Config struct {
	// StartPage is the address a new tab and the home button show.
	StartPage string `json:"start_page"`
	// SearchEngine is the engine what is typed in the address bar and
	// is not an address is sent to, by its id (search.Engines).
	SearchEngine string `json:"search_engine"`
	// DarkMode is whether light's own pages are drawn dark.
	DarkMode bool `json:"dark_mode"`
	// Downloads is the folder files are saved to, "" for the user's
	// Downloads folder.
	Downloads string `json:"downloads"`
	// KeepHistory is whether visits are noted; HistoryLimit how many are
	// kept.
	KeepHistory  bool `json:"keep_history"`
	HistoryLimit int  `json:"history_limit"`
	// Zoom is the size a page of the web is shown at, in percent.
	Zoom int `json:"zoom"`
	// Pictures is whether a page's pictures are fetched.
	Pictures bool `json:"pictures"`
	// Timeout is how long a page and its sheets may take, in seconds;
	// Redirects how many redirects one navigation follows.
	Timeout   int `json:"timeout"`
	Redirects int `json:"redirects"`
	// UserAgent is the User-Agent requests are sent with, "" for
	// light's own (identity.UserAgent).
	UserAgent string `json:"user_agent"`
	// Languages is the languages a site is asked for, most wanted
	// first, as "fr-FR, en"; "" for the system's.
	Languages string `json:"accept_language"`
	// MetaRefresh is whether a page that refreshes elsewhere at once is
	// followed there.
	MetaRefresh bool `json:"meta_refresh"`
	// JavaScript is whether a page's scripts run, in a build that has an
	// engine for them.
	JavaScript bool `json:"javascript"`
	// Frames is whether the documents a page shows in its iframes are
	// loaded.
	Frames bool `json:"frames"`
	// Cookies is whether sites' cookies are kept and sent, and
	// ThirdPartyCookies whether those of another site than the page's
	// are too.
	Cookies           bool `json:"cookies"`
	ThirdPartyCookies bool `json:"third_party_cookies"`
	// LocalNetwork is whether pages of the Internet reach this machine
	// and the network around it, which they do not unless it is set.
	LocalNetwork bool `json:"local_network"`
	// PreserveLog is whether a new tab's developer tools keep the
	// requests over a navigation; Requests how many its log keeps.
	PreserveLog bool `json:"preserve_log"`
	Requests    int  `json:"requests"`
	// ToolsHeight is the height the developer tools were dragged to, 0
	// for their own.
	ToolsHeight int `json:"tools_height"`
	// Model is whether the language model that runs on this device is
	// on: nothing of it is downloaded or started until it is. ModelID is
	// the one chosen among those light knows (model.Known), "" for the
	// one proposed to this device, and ModelFile the path of a GGUF file
	// of the user's in its place: never both. ModelThreads is how many
	// threads compute, 0 for light's own choice, and ModelCPU whether
	// the graphics card is left out of it.
	Model        bool   `json:"model"`
	ModelID      string `json:"model_id"`
	ModelFile    string `json:"model_file"`
	ModelThreads int    `json:"model_threads"`
	ModelCPU     bool   `json:"model_cpu"`
}

// formerUserAgent is the User-Agent light once had by default and wrote
// in its file: it is read as the default.
const formerUserAgent = "light/1.0 (ember)"

// Default is the settings light starts with.
func Default() Config {
	return Config{
		StartPage: tabs.Home, SearchEngine: search.Default, KeepHistory: true, HistoryLimit: 2000, Zoom: 100,
		Pictures: true, Timeout: 30, Redirects: 10, MetaRefresh: true, Requests: 1000,
		JavaScript: true, Frames: true, Cookies: true,
	}
}

// Valid is c with every value out of its bounds replaced by its default.
func (c Config) Valid() Config {
	d := Default()
	c.StartPage = text(c.StartPage, d.StartPage)
	if !search.Known(c.SearchEngine) {
		c.SearchEngine = d.SearchEngine
	}
	c.Downloads = strings.TrimSpace(c.Downloads)
	if c.UserAgent = text(c.UserAgent, d.UserAgent); c.UserAgent == formerUserAgent {
		c.UserAgent = d.UserAgent
	}
	c.Languages = strings.Join(identity.Tags(text(c.Languages, d.Languages)), ", ")
	c.HistoryLimit = within(c.HistoryLimit, MinHistory, MaxHistory, d.HistoryLimit)
	c.Zoom = within(c.Zoom, MinZoom, MaxZoom, d.Zoom)
	c.Timeout = within(c.Timeout, MinTimeout, MaxTimeout, d.Timeout)
	c.Redirects = within(c.Redirects, MinRedirects, MaxRedirects, d.Redirects)
	c.Requests = within(c.Requests, MinRequests, MaxRequests, d.Requests)
	c.ToolsHeight = within(c.ToolsHeight, MinToolsHeight, MaxToolsHeight, d.ToolsHeight)
	return c.validModel()
}

// text is value trimmed, or fallback when nothing is left or it holds a
// control character.
func text(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsFunc(value, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return fallback
	}
	return value
}

func within(value, low, high, fallback int) int {
	if value < low || value > high {
		return fallback
	}
	return value
}

// Encode is c as the file holds it.
func (c Config) Encode() ([]byte, error) {
	fields := map[string]any{"version": version}
	value := reflect.ValueOf(c)
	for i := range value.NumField() {
		fields[value.Type().Field(i).Tag.Get("json")] = value.Field(i).Interface()
	}
	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding the settings: %w", err)
	}
	return data, nil
}

// Decode reads what Encode wrote, each setting alone: a key it does not
// know is ignored, and a value that is not of its kind or out of its
// bounds is the default. Data that is no such file is the defaults and
// an error.
func Decode(data []byte) (Config, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return Default(), errors.New("the settings' file is not one light wrote")
	}
	c := Default()
	value := reflect.ValueOf(&c).Elem()
	for i := range value.NumField() {
		raw, ok := fields[value.Type().Field(i).Tag.Get("json")]
		if !ok || string(raw) == "null" {
			continue
		}
		read := reflect.New(value.Field(i).Type())
		if json.Unmarshal(raw, read.Interface()) == nil {
			value.Field(i).Set(read.Elem())
		}
	}
	return c.Valid(), nil
}

// Load reads the settings saved at path. A file that is not there is the
// defaults; one that cannot be read is the defaults too, and the reason.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Default(), nil
	case err != nil:
		return Default(), fmt.Errorf("reading the settings: %w", err)
	}
	return Decode(data)
}

// Save writes c at path, making its folder, the file whole or not at
// all.
func Save(path string, c Config) error {
	data, err := c.Encode()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("saving the settings: %w", err)
	}
	draft := path + ".tmp"
	if err = os.WriteFile(draft, data, 0o600); err != nil {
		return fmt.Errorf("saving the settings: %w", err)
	}
	if err = os.Rename(draft, path); err != nil {
		return fmt.Errorf("saving the settings: %w", err)
	}
	return nil
}
