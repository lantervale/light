// Package webdriver is the W3C WebDriver protocol (classic,
// https://www.w3.org/TR/webdriver2/) as a server speaks it, in plain Go:
// the routes, the one session, the JSON sent and answered, the errors
// with their codes and statuses, the capabilities, the timeouts, the
// references to elements, windows and frames, and the input actions
// read into values. What the commands do is a Browser's, which the
// program that is driven implements.
//
// A WebDriver server is a remote control. Listen binds this machine
// alone, and the Server answers only requests addressed to it by a name
// or an address of this machine that carry no Origin, so no page of the
// web can drive it, by a fetch or by a name that turns to 127.0.0.1.
package webdriver

import (
	"context"
	"encoding/json"
	"time"
)

// The keys a reference to an element, a window, a frame or a shadow
// root is sent under.
const (
	ElementKey = "element-6066-11e4-a52e-4f735466cecf"
	WindowKey  = "window-fcc6-11e5-b4f8-330a88ab9d7f"
	FrameKey   = "frame-075b-4da1-b6ba-e579c2d3230a"
	ShadowKey  = "shadow-6066-11e4-a52e-4f735466cecf"
)

// The page load strategies.
const (
	LoadNone   = "none"
	LoadEager  = "eager"
	LoadNormal = "normal"
)

// The location strategies.
const (
	ByCSS             = "css selector"
	ByLinkText        = "link text"
	ByPartialLinkText = "partial link text"
	ByTagName         = "tag name"
	ByXPath           = "xpath"
)

// Load is how long a command waits for the page it leads to: until
// what Strategy names, Timeout at most.
type Load struct {
	Strategy string
	Timeout  time.Duration
}

// Rect is a window or an element: where it is and its size, in CSS
// pixels.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Placement is what Set Window Rect asks for: nil leaves a value as it
// is.
type Placement struct {
	X, Y, Width, Height *float64
}

// Locator is what Find Elements looks for: by a strategy, from an
// element when From names one, the document otherwise.
type Locator struct {
	Using, Value, From string
}

// Frame is what Switch To Frame names: the top document, the frame
// numbered Index of the current document, or the frame an element is.
type Frame struct {
	Top     bool
	Index   int
	Element string
}

// Script is what Execute Script runs: the body of a function, its
// arguments as the JSON array sent, and for how long. An Async one is
// given a callback as its last argument and ends when it is called.
type Script struct {
	Source string
	Args   json.RawMessage
	Async  bool
	// Timeout is how long it may take, for ever when Endless.
	Timeout time.Duration
	Endless bool
}

// Cookie is a cookie as the protocol tells it. Expiry is in seconds
// since 1970, nil for one that ends with the session.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
	Expiry   *int64 `json:"expiry,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

// Browser is the program a Server drives. Its methods are called one at
// a time, from the goroutine of the request, and answer an *Error for
// what the standard has a code for; ctx ends when the client hangs up.
type Browser interface {
	// NewSession begins the session and EndSession ends it, leaving the
	// browser ready for another.
	NewSession(ctx context.Context, settings Settings) error
	EndSession(ctx context.Context) error

	Navigate(ctx context.Context, address string, wait Load) error
	CurrentURL(ctx context.Context) (string, error)
	Back(ctx context.Context, wait Load) error
	Forward(ctx context.Context, wait Load) error
	Refresh(ctx context.Context, wait Load) error
	Title(ctx context.Context) (string, error)
	PageSource(ctx context.Context) (string, error)

	WindowHandle(ctx context.Context) (string, error)
	WindowHandles(ctx context.Context) ([]string, error)
	SwitchToWindow(ctx context.Context, handle string) error
	// CloseWindow closes the current window and answers those left.
	CloseWindow(ctx context.Context) ([]string, error)
	// NewWindow opens a "tab" or a "window" and answers its handle and
	// which of the two it is.
	NewWindow(ctx context.Context, kind string) (handle, opened string, err error)
	WindowRect(ctx context.Context) (Rect, error)
	SetWindowRect(ctx context.Context, to Placement) (Rect, error)
	Maximize(ctx context.Context) (Rect, error)
	Minimize(ctx context.Context) (Rect, error)
	Fullscreen(ctx context.Context) (Rect, error)

	SwitchToFrame(ctx context.Context, to Frame) error
	SwitchToParentFrame(ctx context.Context) error

	// FindElements answers the references of what a locator finds,
	// looking again for as long as wait while it finds nothing.
	FindElements(ctx context.Context, by Locator, wait time.Duration) ([]string, error)
	ActiveElement(ctx context.Context) (string, error)
	// ElementAttribute answers nil for an attribute the element has not.
	ElementAttribute(ctx context.Context, element, name string) (*string, error)
	// ElementProperty answers the property as JSON.
	ElementProperty(ctx context.Context, element, name string) (json.RawMessage, error)
	ElementCSSValue(ctx context.Context, element, name string) (string, error)
	ElementText(ctx context.Context, element string) (string, error)
	ElementTagName(ctx context.Context, element string) (string, error)
	ElementRect(ctx context.Context, element string) (Rect, error)
	ElementEnabled(ctx context.Context, element string) (bool, error)
	ElementSelected(ctx context.Context, element string) (bool, error)
	ElementDisplayed(ctx context.Context, element string) (bool, error)
	ElementClick(ctx context.Context, element string, wait Load) error
	ElementClear(ctx context.Context, element string) error
	ElementSendKeys(ctx context.Context, element, text string) error

	// Execute answers what the script returned as JSON, references to
	// elements included.
	Execute(ctx context.Context, script Script) (json.RawMessage, error)

	Cookies(ctx context.Context) ([]Cookie, error)
	AddCookie(ctx context.Context, cookie Cookie) error
	// DeleteCookies deletes the cookie of that name, every cookie of the
	// document for "".
	DeleteCookies(ctx context.Context, name string) error

	PerformActions(ctx context.Context, sources []Source) error
	ReleaseActions(ctx context.Context) error

	DismissAlert(ctx context.Context) error
	AcceptAlert(ctx context.Context) error
	AlertText(ctx context.Context) (string, error)
	// Prompt is the prompt that is open, a no such alert error for
	// none.
	Prompt(ctx context.Context) (Prompt, error)
	SendAlertText(ctx context.Context, text string) error

	// Screenshot answers a PNG of the page's view, ElementScreenshot of
	// an element.
	Screenshot(ctx context.Context) ([]byte, error)
	ElementScreenshot(ctx context.Context, element string) ([]byte, error)
}
