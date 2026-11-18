package webdriver

import (
	"encoding/base64"
	"net/http"
)

const (
	get    = http.MethodGet
	post   = http.MethodPost
	remove = http.MethodDelete

	inSession = "session/{session}/"
	inElement = inSession + "element/{element}/"
)

// routes are the commands of the standard light answers. Those it has
// no route for (print, the computed role and label, the extensions of
// other standards) are unknown commands.
func routes() []route {
	return append(append(sessionRoutes(), elementRoutes()...), pageRoutes()...)
}

func sessionRoutes() []route {
	return []route{
		{get, "status", (*Server).status},
		{post, "session", (*Server).create},
		{remove, "session/{session}", (*Server).end},
		{get, inSession + "timeouts", func(_ *Server, c *call) (any, error) { return c.session.timeouts, nil }},
		{post, inSession + "timeouts", (*Server).setTimeouts},
		{post, inSession + "url", said("url", func(s *Server, c *call, address string) error {
			return s.browser.Navigate(c.ctx, address, c.load())
		})},
		{get, inSession + "url", func(s *Server, c *call) (any, error) { return s.browser.CurrentURL(c.ctx) }},
		{post, inSession + "back", null(func(s *Server, c *call) error { return s.browser.Back(c.ctx, c.load()) })},
		{post, inSession + "forward", null(func(s *Server, c *call) error {
			return s.browser.Forward(c.ctx, c.load())
		})},
		{post, inSession + "refresh", null(func(s *Server, c *call) error {
			return s.browser.Refresh(c.ctx, c.load())
		})},
		{get, inSession + "title", func(s *Server, c *call) (any, error) { return s.browser.Title(c.ctx) }},
		{get, inSession + "window", func(s *Server, c *call) (any, error) { return s.browser.WindowHandle(c.ctx) }},
		{remove, inSession + "window", (*Server).closeWindow},
		{post, inSession + "window", said("handle", func(s *Server, c *call, handle string) error {
			return s.browser.SwitchToWindow(c.ctx, handle)
		})},
		{get, inSession + "window/handles", func(s *Server, c *call) (any, error) {
			handles, err := s.browser.WindowHandles(c.ctx)
			if handles == nil {
				handles = []string{}
			}
			return handles, err
		}},
		{post, inSession + "window/new", (*Server).newWindow},
		{post, inSession + "frame", (*Server).switchFrame},
		{post, inSession + "frame/parent", null(func(s *Server, c *call) error {
			return s.browser.SwitchToParentFrame(c.ctx)
		})},
		{get, inSession + "window/rect", func(s *Server, c *call) (any, error) { return s.browser.WindowRect(c.ctx) }},
		{post, inSession + "window/rect", (*Server).setRect},
		{post, inSession + "window/maximize", func(s *Server, c *call) (any, error) {
			return s.browser.Maximize(c.ctx)
		}},
		{post, inSession + "window/minimize", func(s *Server, c *call) (any, error) {
			return s.browser.Minimize(c.ctx)
		}},
		{post, inSession + "window/fullscreen", func(s *Server, c *call) (any, error) {
			return s.browser.Fullscreen(c.ctx)
		}},
	}
}

func elementRoutes() []route {
	of := func(c *call) string { return c.vars["element"] }
	return []route{
		{get, inSession + "element/active", func(s *Server, c *call) (any, error) {
			id, err := s.browser.ActiveElement(c.ctx)
			if err != nil {
				return nil, err
			}
			return element(id), nil
		}},
		{post, inSession + "element", (*Server).findOne},
		{post, inSession + "elements", func(s *Server, c *call) (any, error) { return s.find(c) }},
		{post, inElement + "element", (*Server).findOne},
		{post, inElement + "elements", func(s *Server, c *call) (any, error) { return s.find(c) }},
		{get, inElement + "shadow", func(*Server, *call) (any, error) {
			return nil, Errorf(NoSuchShadowRoot, "light has no shadow tree: no element has a shadow root")
		}},
		{post, inSession + "shadow/{shadow}/element", func(*Server, *call) (any, error) {
			return nil, Errorf(NoSuchShadowRoot, "light has no shadow tree: no shadow root is known")
		}},
		{post, inSession + "shadow/{shadow}/elements", func(*Server, *call) (any, error) {
			return nil, Errorf(NoSuchShadowRoot, "light has no shadow tree: no shadow root is known")
		}},
		{get, inElement + "selected", func(s *Server, c *call) (any, error) {
			return s.browser.ElementSelected(c.ctx, of(c))
		}},
		{get, inElement + "attribute/{name}", func(s *Server, c *call) (any, error) {
			return s.browser.ElementAttribute(c.ctx, of(c), c.vars["name"])
		}},
		{get, inElement + "property/{name}", func(s *Server, c *call) (any, error) {
			return s.browser.ElementProperty(c.ctx, of(c), c.vars["name"])
		}},
		{get, inElement + "css/{name}", func(s *Server, c *call) (any, error) {
			return s.browser.ElementCSSValue(c.ctx, of(c), c.vars["name"])
		}},
		{get, inElement + "text", func(s *Server, c *call) (any, error) { return s.browser.ElementText(c.ctx, of(c)) }},
		{get, inElement + "name", func(s *Server, c *call) (any, error) {
			return s.browser.ElementTagName(c.ctx, of(c))
		}},
		{get, inElement + "rect", func(s *Server, c *call) (any, error) { return s.browser.ElementRect(c.ctx, of(c)) }},
		{get, inElement + "enabled", func(s *Server, c *call) (any, error) {
			return s.browser.ElementEnabled(c.ctx, of(c))
		}},
		{get, inElement + "displayed", func(s *Server, c *call) (any, error) {
			return s.browser.ElementDisplayed(c.ctx, of(c))
		}},
		{post, inElement + "click", null(func(s *Server, c *call) error {
			return s.browser.ElementClick(c.ctx, of(c), c.load())
		})},
		{post, inElement + "clear", null(func(s *Server, c *call) error {
			return s.browser.ElementClear(c.ctx, of(c))
		})},
		{post, inElement + "value", said("text", func(s *Server, c *call, text string) error {
			return s.browser.ElementSendKeys(c.ctx, of(c), text)
		})},
		{get, inElement + "screenshot", func(s *Server, c *call) (any, error) {
			return shot(s.browser.ElementScreenshot(c.ctx, of(c)))
		}},
	}
}

func pageRoutes() []route {
	return []route{
		{get, inSession + "source", func(s *Server, c *call) (any, error) { return s.browser.PageSource(c.ctx) }},
		{post, inSession + "execute/sync", func(s *Server, c *call) (any, error) { return s.execute(c, false) }},
		{post, inSession + "execute/async", func(s *Server, c *call) (any, error) { return s.execute(c, true) }},
		{get, inSession + "cookie", func(s *Server, c *call) (any, error) {
			all, err := s.browser.Cookies(c.ctx)
			if all == nil {
				all = []Cookie{}
			}
			return all, err
		}},
		{get, inSession + "cookie/{name}", (*Server).cookie},
		{post, inSession + "cookie", (*Server).addCookie},
		{remove, inSession + "cookie/{name}", null(func(s *Server, c *call) error {
			return s.browser.DeleteCookies(c.ctx, c.vars["name"])
		})},
		{remove, inSession + "cookie", null(func(s *Server, c *call) error {
			return s.browser.DeleteCookies(c.ctx, "")
		})},
		{post, inSession + "actions", (*Server).perform},
		{remove, inSession + "actions", null(func(s *Server, c *call) error {
			return s.browser.ReleaseActions(c.ctx)
		})},
		{post, inSession + "alert/dismiss", null(func(s *Server, c *call) error {
			return s.browser.DismissAlert(c.ctx)
		})},
		{post, inSession + "alert/accept", null(func(s *Server, c *call) error {
			return s.browser.AcceptAlert(c.ctx)
		})},
		{get, inSession + "alert/text", func(s *Server, c *call) (any, error) { return s.browser.AlertText(c.ctx) }},
		{post, inSession + "alert/text", said("text", func(s *Server, c *call, text string) error {
			return s.browser.SendAlertText(c.ctx, text)
		})},
		{get, inSession + "screenshot", func(s *Server, c *call) (any, error) {
			return shot(s.browser.Screenshot(c.ctx))
		}},
	}
}

// shot is a PNG as a screenshot command answers it.
func shot(png []byte, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.EncodeToString(png), nil
}
