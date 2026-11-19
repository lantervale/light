package webdriver

import (
	"encoding/json"
	"errors"
	"strings"
)

// The kinds of Prompt.
const (
	PromptAlert   = "alert"
	PromptConfirm = "confirm"
	PromptPrompt  = "prompt"
)

// Prompt is a question a page asks the user that is open: alert,
// confirm or prompt, and its message.
type Prompt struct {
	Kind, Text string
}

// unprompted reports whether a command leaves an open prompt alone:
// those the standard does not have handle the user prompts.
func unprompted(method, path string) bool {
	switch {
	case path == "status", path == "session", path == "session/{session}":
		return true
	case path == inSession+"timeouts", path == inSession+"window/handles", strings.HasPrefix(path, inSession+"alert/"):
		return true
	case path == inSession+"window":
		return method != remove
	}
	return false
}

// handler is how a session handles a prompt of that kind it did not
// expect: whether it accepts it, whether it dismisses it (neither
// leaves it open), and whether the command that met it fails. A session that says nothing
// dismisses it and fails.
func handler(asked json.RawMessage, kind string) (bool, bool, bool) {
	word, plain := text(asked)
	if kinds, byKind := object(asked); byKind {
		if word, plain = text(kinds[kind]); !plain {
			word, plain = text(kinds["default"])
		}
	}
	if !plain {
		word = DismissAndNotify
	}
	switch word {
	case "accept":
		return true, false, false
	case "dismiss":
		return false, true, false
	case "accept and notify":
		return true, false, true
	case "ignore":
		return false, false, true
	}
	return false, true, true
}

// prompts handles the prompt that is open before a command runs, as
// the session asked for: nothing with none.
func (s *Server) prompts(c *call) error {
	open, err := s.browser.Prompt(c.ctx)
	var none *Error
	if errors.As(err, &none) && none.Code == NoSuchAlert {
		return nil
	}
	if err != nil {
		return err
	}
	accept, dismiss, notify := handler(c.session.settings.Prompts, open.Kind)
	switch {
	case accept:
		err = s.browser.AcceptAlert(c.ctx)
	case dismiss:
		err = s.browser.DismissAlert(c.ctx)
	}
	if err != nil {
		return err
	}
	if notify {
		return &Error{Code: UnexpectedAlertOpen, Message: "the page asks: " + open.Text,
			Data: map[string]any{"text": open.Text}}
	}
	return nil
}
