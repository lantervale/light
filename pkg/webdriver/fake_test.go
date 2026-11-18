package webdriver_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lantervale/light/pkg/webdriver"
)

// fake is a browser that notes what it was asked and answers what a
// test set.
type fake struct {
	calls    []string
	settings webdriver.Settings
	load     webdriver.Load
	script   webdriver.Script
	locator  webdriver.Locator
	wait     time.Duration
	frame    webdriver.Frame
	placed   webdriver.Placement
	cookie   webdriver.Cookie
	sources  []webdriver.Source
	sent     string
	prompt   *webdriver.Prompt

	err     error
	found   []string
	handles []string
	cookies []webdriver.Cookie
	// hold, when set, is waited for by Navigate.
	hold chan struct{}
}

func (f *fake) note(call string) error {
	f.calls = append(f.calls, call)
	return f.err
}

func (f *fake) NewSession(_ context.Context, settings webdriver.Settings) error {
	f.settings = settings
	return f.note("NewSession")
}
func (f *fake) EndSession(context.Context) error { return f.note("EndSession") }

func (f *fake) Navigate(ctx context.Context, address string, wait webdriver.Load) error {
	f.load = wait
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.note("Navigate " + address)
}
func (f *fake) CurrentURL(context.Context) (string, error) {
	return "http://a.test/", f.note("CurrentURL")
}
func (f *fake) Back(context.Context, webdriver.Load) error { return f.note("Back") }
func (f *fake) Forward(context.Context, webdriver.Load) error {
	return f.note("Forward")
}
func (f *fake) Refresh(context.Context, webdriver.Load) error { return f.note("Refresh") }
func (f *fake) Title(context.Context) (string, error)         { return "A title", f.note("Title") }
func (f *fake) PageSource(context.Context) (string, error) {
	return "<html></html>", f.note("PageSource")
}
func (f *fake) WindowHandle(context.Context) (string, error) { return "w1", f.note("WindowHandle") }
func (f *fake) WindowHandles(context.Context) ([]string, error) {
	return f.handles, f.note("WindowHandles")
}
func (f *fake) SwitchToWindow(_ context.Context, handle string) error {
	return f.note("SwitchToWindow " + handle)
}
func (f *fake) CloseWindow(context.Context) ([]string, error) {
	return f.handles, f.note("CloseWindow")
}
func (f *fake) NewWindow(_ context.Context, kind string) (string, string, error) {
	return "w2", kind, f.note("NewWindow " + kind)
}
func (f *fake) WindowRect(context.Context) (webdriver.Rect, error) {
	return webdriver.Rect{X: 1, Y: 2, Width: 800, Height: 600}, f.note("WindowRect")
}
func (f *fake) SetWindowRect(_ context.Context, to webdriver.Placement) (webdriver.Rect, error) {
	f.placed = to
	return webdriver.Rect{Width: 800, Height: 600}, f.note("SetWindowRect")
}
func (f *fake) Maximize(context.Context) (webdriver.Rect, error) {
	return webdriver.Rect{}, f.note("Maximize")
}
func (f *fake) Minimize(context.Context) (webdriver.Rect, error) {
	return webdriver.Rect{}, f.note("Minimize")
}
func (f *fake) Fullscreen(context.Context) (webdriver.Rect, error) {
	return webdriver.Rect{}, f.note("Fullscreen")
}
func (f *fake) SwitchToFrame(_ context.Context, to webdriver.Frame) error {
	f.frame = to
	return f.note("SwitchToFrame")
}
func (f *fake) SwitchToParentFrame(context.Context) error { return f.note("SwitchToParentFrame") }
func (f *fake) FindElements(_ context.Context, by webdriver.Locator, wait time.Duration) ([]string, error) {
	f.locator, f.wait = by, wait
	return f.found, f.note("FindElements")
}
func (f *fake) ActiveElement(context.Context) (string, error) { return "e9", f.note("ActiveElement") }
func (f *fake) ElementAttribute(_ context.Context, element, name string) (*string, error) {
	if name == "missing" {
		return nil, f.note("ElementAttribute " + element + " " + name)
	}
	value := "v"
	return &value, f.note("ElementAttribute " + element + " " + name)
}
func (f *fake) ElementProperty(_ context.Context, element, name string) (json.RawMessage, error) {
	return json.RawMessage(`{"a":[1,null]}`), f.note("ElementProperty " + element + " " + name)
}
func (f *fake) ElementCSSValue(_ context.Context, element, name string) (string, error) {
	return "red", f.note("ElementCSSValue " + element + " " + name)
}
func (f *fake) ElementText(_ context.Context, element string) (string, error) {
	return "words", f.note("ElementText " + element)
}
func (f *fake) ElementTagName(_ context.Context, element string) (string, error) {
	return "p", f.note("ElementTagName " + element)
}
func (f *fake) ElementRect(_ context.Context, element string) (webdriver.Rect, error) {
	return webdriver.Rect{X: 3, Y: 4, Width: 5, Height: 6}, f.note("ElementRect " + element)
}
func (f *fake) ElementEnabled(_ context.Context, element string) (bool, error) {
	return true, f.note("ElementEnabled " + element)
}
func (f *fake) ElementSelected(_ context.Context, element string) (bool, error) {
	return false, f.note("ElementSelected " + element)
}
func (f *fake) ElementDisplayed(_ context.Context, element string) (bool, error) {
	return true, f.note("ElementDisplayed " + element)
}
func (f *fake) ElementClick(_ context.Context, element string, wait webdriver.Load) error {
	f.load = wait
	return f.note("ElementClick " + element)
}
func (f *fake) ElementClear(_ context.Context, element string) error {
	return f.note("ElementClear " + element)
}
func (f *fake) ElementSendKeys(_ context.Context, element, text string) error {
	f.sent = text
	return f.note("ElementSendKeys " + element)
}
func (f *fake) Execute(_ context.Context, script webdriver.Script) (json.RawMessage, error) {
	f.script = script
	return json.RawMessage(`[1,{"` + webdriver.ElementKey + `":"e1"}]`), f.note("Execute")
}
func (f *fake) Cookies(context.Context) ([]webdriver.Cookie, error) {
	return f.cookies, f.note("Cookies")
}
func (f *fake) AddCookie(_ context.Context, cookie webdriver.Cookie) error {
	f.cookie = cookie
	return f.note("AddCookie")
}
func (f *fake) DeleteCookies(_ context.Context, name string) error {
	return f.note("DeleteCookies " + name)
}
func (f *fake) PerformActions(_ context.Context, sources []webdriver.Source) error {
	f.sources = sources
	return f.note("PerformActions")
}
func (f *fake) ReleaseActions(context.Context) error { return f.note("ReleaseActions") }
func (f *fake) Prompt(context.Context) (webdriver.Prompt, error) {
	if f.prompt == nil {
		return webdriver.Prompt{}, webdriver.Errorf(webdriver.NoSuchAlert, "no prompt is open")
	}
	return *f.prompt, nil
}
func (f *fake) DismissAlert(ctx context.Context) error {
	if _, err := f.Prompt(ctx); err != nil {
		return err
	}
	f.prompt = nil
	return f.note("DismissAlert")
}
func (f *fake) AcceptAlert(ctx context.Context) error {
	if _, err := f.Prompt(ctx); err != nil {
		return err
	}
	f.prompt = nil
	return f.note("AcceptAlert")
}
func (f *fake) AlertText(ctx context.Context) (string, error) {
	open, err := f.Prompt(ctx)
	return open.Text, err
}
func (f *fake) SendAlertText(ctx context.Context, text string) error {
	if _, err := f.Prompt(ctx); err != nil {
		return err
	}
	return f.note("SendAlertText " + text)
}
func (f *fake) Screenshot(context.Context) ([]byte, error) {
	return []byte("png"), f.note("Screenshot")
}
func (f *fake) ElementScreenshot(_ context.Context, element string) ([]byte, error) {
	return []byte("png"), f.note("ElementScreenshot " + element)
}
