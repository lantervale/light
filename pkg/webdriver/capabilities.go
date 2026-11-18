package webdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// maxSafeInteger is the largest whole number JSON carries safely.
const maxSafeInteger = 1<<53 - 1

// The names of the capabilities.
const (
	capBrowserName      = "browserName"
	capBrowserVersion   = "browserVersion"
	capPlatformName     = "platformName"
	capInsecureCerts    = "acceptInsecureCerts"
	capPageLoadStrategy = "pageLoadStrategy"
	capProxy            = "proxy"
	capSetWindowRect    = "setWindowRect"
	capTimeouts         = "timeouts"
	capStrictFiles      = "strictFileInteractability"
	capPrompts          = "unhandledPromptBehavior"
	capUserAgent        = "userAgent"
	capWebSocketURL     = "webSocketUrl" //nolint:gosec // a capability's name, no credential
)

// timeoutScript is the name of the script timeout.
const timeoutScript = "script"

// DismissAndNotify is the one way prompts are handled when a session
// asks for none.
const DismissAndNotify = "dismiss and notify"

// About is what the browser says of itself, which a session's
// capabilities are matched against.
type About struct {
	// Name, Version and Platform are browserName, browserVersion and
	// platformName; UserAgent is the User-Agent of its requests.
	Name, Version, Platform, UserAgent string
}

// Timeouts are a session's: how long a script may run (for ever when
// ScriptEndless), a page may take to load, and an element is looked
// for.
type Timeouts struct {
	Script        time.Duration
	ScriptEndless bool
	PageLoad      time.Duration
	Implicit      time.Duration
}

// DefaultTimeouts are those of a session that names none.
func DefaultTimeouts() Timeouts {
	return Timeouts{Script: 30 * time.Second, PageLoad: 300 * time.Second}
}

// MarshalJSON writes them in milliseconds, an endless script as null.
func (t Timeouts) MarshalJSON() ([]byte, error) {
	var script any = t.Script.Milliseconds()
	if t.ScriptEndless {
		script = nil
	}
	return json.Marshal(map[string]any{
		timeoutScript: script, "pageLoad": t.PageLoad.Milliseconds(), "implicit": t.Implicit.Milliseconds(),
	})
}

// with is t with what a timeouts object names changed.
func (t Timeouts) with(raw json.RawMessage) (Timeouts, *Error) {
	fields, ok := object(raw)
	if !ok {
		return t, Errorf(InvalidArgument, "timeouts must be an object")
	}
	for name, value := range fields {
		null := isNull(value)
		ms, whole := integer(value)
		switch {
		case name == timeoutScript && null:
			t.ScriptEndless = true
			continue
		case !whole:
			return t, Errorf(InvalidArgument, fmt.Sprintf("the %s timeout must be a whole number of milliseconds "+
				"from 0 to 2^53-1", name))
		}
		switch name {
		case timeoutScript:
			t.Script, t.ScriptEndless = milliseconds(ms), false
		case "pageLoad":
			t.PageLoad = milliseconds(ms)
		case "implicit":
			t.Implicit = milliseconds(ms)
		}
	}
	return t, nil
}

// milliseconds is that many, as long as a Duration holds.
func milliseconds(ms int64) time.Duration {
	if ms > math.MaxInt64/int64(time.Millisecond) {
		return math.MaxInt64
	}
	return time.Duration(ms) * time.Millisecond
}

// Settings are what a session was created with.
type Settings struct {
	PageLoadStrategy string
	Timeouts         Timeouts
	// Prompts is unhandledPromptBehavior, as it was asked for.
	Prompts json.RawMessage
	// Extensions are the capabilities other than the standard's, those
	// whose name holds a colon, as they were sent.
	Extensions map[string]json.RawMessage
}

// capabilities are those of a session, as New Session answers them.
func (a About) capabilities(s Settings) map[string]any {
	prompts := any(DismissAndNotify)
	if s.Prompts != nil {
		prompts = s.Prompts
	}
	return map[string]any{
		capBrowserName: a.Name, capBrowserVersion: a.Version, capPlatformName: a.Platform,
		capInsecureCerts: false, capPageLoadStrategy: s.PageLoadStrategy, capProxy: map[string]any{},
		capSetWindowRect: true, capStrictFiles: false, capTimeouts: s.Timeouts, capPrompts: prompts,
		capUserAgent: a.UserAgent,
	}
}

// settle reads what New Session was sent: the capabilities it must
// have merged with each of those it may, the first the browser matches
// kept.
func (a About) settle(body map[string]json.RawMessage) (Settings, *Error) {
	asked, ok := object(body["capabilities"])
	if !ok {
		return Settings{}, Errorf(InvalidArgument, "capabilities must be an object")
	}
	always := map[string]json.RawMessage{}
	if raw, given := asked["alwaysMatch"]; given && !isNull(raw) {
		if always, ok = object(raw); !ok {
			return Settings{}, Errorf(InvalidArgument, "alwaysMatch must be an object")
		}
	}
	if err := validate(always); err != nil {
		return Settings{}, err
	}
	first := []json.RawMessage{json.RawMessage("{}")}
	if raw, given := asked["firstMatch"]; given && !isNull(raw) {
		if json.Unmarshal(raw, &first) != nil || len(first) == 0 {
			return Settings{}, Errorf(InvalidArgument, "firstMatch must be a list of at least one object")
		}
	}
	var merged []map[string]json.RawMessage
	for _, raw := range first {
		entry, isObject := object(raw)
		if !isObject {
			return Settings{}, Errorf(InvalidArgument, "each entry of firstMatch must be an object")
		}
		if err := validate(entry); err != nil {
			return Settings{}, err
		}
		for name, value := range always {
			if _, twice := entry[name]; twice {
				return Settings{}, Errorf(InvalidArgument, name+" is in alwaysMatch and in firstMatch")
			}
			entry[name] = value
		}
		merged = append(merged, entry)
	}
	var why []string
	for _, entry := range merged {
		settings, mismatch := a.match(entry)
		if mismatch == "" {
			return settings, nil
		}
		why = append(why, mismatch)
	}
	return Settings{}, Errorf(SessionNotCreated, "no capabilities asked for match this browser: "+
		strings.Join(why, "; "))
}

// validate drops the capabilities that are null and checks the others
// are what the standard says each is.
func validate(caps map[string]json.RawMessage) *Error {
	for name, value := range caps {
		if isNull(value) {
			delete(caps, name)
			continue
		}
		if why := invalid(name, value); why != "" {
			return Errorf(InvalidArgument, "the capability "+name+" "+why)
		}
	}
	return nil
}

// invalid is why a capability is not one the standard allows, "" for
// one that is.
func invalid(name string, value json.RawMessage) string {
	switch name {
	case capInsecureCerts, capSetWindowRect, capStrictFiles, capWebSocketURL:
		var flag bool
		if json.Unmarshal(value, &flag) != nil {
			return "must be a boolean"
		}
	case capBrowserName, capBrowserVersion, capPlatformName, capUserAgent:
		if _, ok := text(value); !ok {
			return "must be a string"
		}
	case capPageLoadStrategy:
		if strategy, _ := text(value); strategy != LoadNone && strategy != LoadEager && strategy != LoadNormal {
			return `must be "none", "eager" or "normal"`
		}
	case capProxy:
		if _, ok := object(value); !ok {
			return "must be an object"
		}
	case capTimeouts:
		if _, err := DefaultTimeouts().with(value); err != nil {
			return "is invalid: " + err.Message
		}
	case capPrompts:
		return invalidPrompts(value)
	default:
		if !strings.Contains(name, ":") {
			return "is not one of the standard's, and has no prefix of an extension"
		}
	}
	return ""
}

// invalidPrompts is why an unhandledPromptBehavior is none: it is one
// of the standard's words, or an object of them by kind of prompt.
func invalidPrompts(value json.RawMessage) string {
	known := func(word string) bool {
		switch word {
		case "dismiss", "accept", DismissAndNotify, "accept and notify", "ignore":
			return true
		}
		return false
	}
	if word, ok := text(value); ok {
		if !known(word) {
			return "names no way to handle a prompt"
		}
		return ""
	}
	kinds, ok := object(value)
	if !ok {
		return "must be a string or an object"
	}
	for _, each := range kinds {
		if word, isText := text(each); !isText || !known(word) {
			return "names no way to handle a prompt"
		}
	}
	return ""
}

// match is the settings of a session with those capabilities, or why
// the browser is not what they ask for.
func (a About) match(caps map[string]json.RawMessage) (Settings, string) {
	settings := Settings{PageLoadStrategy: LoadNormal, Timeouts: DefaultTimeouts(),
		Extensions: map[string]json.RawMessage{}}
	for name, value := range caps {
		word, _ := text(value)
		var flag bool
		_ = json.Unmarshal(value, &flag)
		switch name {
		case capBrowserName:
			if word != a.Name {
				return settings, fmt.Sprintf("browserName is %q, this is %q", word, a.Name)
			}
		case capBrowserVersion:
			if word != a.Version {
				return settings, fmt.Sprintf("browserVersion is %q, this is %q", word, a.Version)
			}
		case capPlatformName:
			if word != a.Platform {
				return settings, fmt.Sprintf("platformName is %q, this is %q", word, a.Platform)
			}
		case capInsecureCerts:
			if flag {
				return settings, "acceptInsecureCerts is true, and light checks every certificate"
			}
		case capSetWindowRect:
			if !flag {
				return settings, "setWindowRect is false, and light's window is moved and sized"
			}
		case capStrictFiles:
			if flag {
				return settings, "strictFileInteractability is true, and light sends no file"
			}
		case capWebSocketURL:
			if flag {
				return settings, "webSocketUrl is true, and light has no WebDriver BiDi"
			}
		case capProxy:
			if fields, _ := object(value); len(fields) > 0 {
				return settings, "a proxy is asked for, and light is given none through WebDriver"
			}
		case capUserAgent:
			if word != a.UserAgent {
				return settings, fmt.Sprintf("userAgent is %q, this is %q", word, a.UserAgent)
			}
		case capPageLoadStrategy:
			settings.PageLoadStrategy = word
		case capTimeouts:
			settings.Timeouts, _ = settings.Timeouts.with(value)
		case capPrompts:
			settings.Prompts = value
		default:
			settings.Extensions[name] = value
		}
	}
	return settings, ""
}

// object is raw as a JSON object, false for anything else.
func object(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || isNull(raw) || json.Unmarshal(raw, &fields) != nil {
		return nil, false
	}
	return fields, true
}

// text is raw as a JSON string, false for anything else.
func text(raw json.RawMessage) (string, bool) {
	var s string
	if len(raw) == 0 || isNull(raw) || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// integer is raw as a whole number from 0 to 2^53-1, false for
// anything else.
func integer(raw json.RawMessage) (int64, bool) {
	n, ok := number(raw)
	if !ok || n < 0 || n > maxSafeInteger || n != math.Trunc(n) {
		return 0, false
	}
	return int64(n), true
}

// number is raw as a JSON number, false for anything else.
func number(raw json.RawMessage) (float64, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] == '"' || isNull(trimmed) {
		return 0, false
	}
	var n float64
	if json.Unmarshal(trimmed, &n) != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }
