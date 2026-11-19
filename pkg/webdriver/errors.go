package webdriver

import (
	"errors"
	"net/http"
)

// Code is an error code of the WebDriver standard, as it is sent.
type Code string

// The error codes of the standard.
const (
	ElementClickIntercepted Code = "element click intercepted"
	ElementNotInteractable  Code = "element not interactable"
	InsecureCertificate     Code = "insecure certificate"
	InvalidArgument         Code = "invalid argument"
	InvalidCookieDomain     Code = "invalid cookie domain"
	InvalidElementState     Code = "invalid element state"
	InvalidSelector         Code = "invalid selector"
	InvalidSessionID        Code = "invalid session id"
	JavaScriptError         Code = "javascript error"
	MoveTargetOutOfBounds   Code = "move target out of bounds"
	NoSuchAlert             Code = "no such alert"
	NoSuchCookie            Code = "no such cookie"
	NoSuchElement           Code = "no such element"
	NoSuchFrame             Code = "no such frame"
	NoSuchWindow            Code = "no such window"
	NoSuchShadowRoot        Code = "no such shadow root"
	ScriptTimeout           Code = "script timeout"
	SessionNotCreated       Code = "session not created"
	StaleElementReference   Code = "stale element reference"
	DetachedShadowRoot      Code = "detached shadow root"
	Timeout                 Code = "timeout"
	UnableToSetCookie       Code = "unable to set cookie"
	UnableToCaptureScreen   Code = "unable to capture screen"
	UnexpectedAlertOpen     Code = "unexpected alert open"
	UnknownCommand          Code = "unknown command"
	UnknownError            Code = "unknown error"
	UnknownMethod           Code = "unknown method"
	UnsupportedOperation    Code = "unsupported operation"
)

// Status is the HTTP status the standard answers a code with.
func (c Code) Status() int {
	switch c {
	case ElementClickIntercepted, ElementNotInteractable, InsecureCertificate, InvalidArgument, InvalidCookieDomain,
		InvalidElementState, InvalidSelector:
		return http.StatusBadRequest
	case InvalidSessionID, NoSuchAlert, NoSuchCookie, NoSuchElement, NoSuchFrame, NoSuchWindow, NoSuchShadowRoot,
		StaleElementReference, DetachedShadowRoot, UnknownCommand:
		return http.StatusNotFound
	case UnknownMethod:
		return http.StatusMethodNotAllowed
	case JavaScriptError, MoveTargetOutOfBounds, ScriptTimeout, SessionNotCreated, Timeout, UnableToSetCookie,
		UnableToCaptureScreen, UnexpectedAlertOpen, UnknownError, UnsupportedOperation:
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}

// Known reports whether c is a code of the standard.
func (c Code) Known() bool {
	switch c {
	case ElementClickIntercepted, ElementNotInteractable, InsecureCertificate, InvalidArgument, InvalidCookieDomain,
		InvalidElementState, InvalidSelector, InvalidSessionID, JavaScriptError, MoveTargetOutOfBounds, NoSuchAlert,
		NoSuchCookie, NoSuchElement, NoSuchFrame, NoSuchWindow, NoSuchShadowRoot, ScriptTimeout, SessionNotCreated,
		StaleElementReference, DetachedShadowRoot, Timeout, UnableToSetCookie, UnableToCaptureScreen,
		UnexpectedAlertOpen, UnknownCommand, UnknownError, UnknownMethod, UnsupportedOperation:
		return true
	}
	return false
}

// Error is why a command failed, as the standard tells it: a code, a
// message for whoever reads it, and the calls that led to it when it is
// a script's.
type Error struct {
	Code    Code
	Message string
	Stack   string
	// Data is what the standard has an error carry beside them, such
	// as the text of the prompt a command met.
	Data map[string]any
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// Errorf is an Error of that code.
func Errorf(code Code, message string) *Error { return &Error{Code: code, Message: message} }

// failure is err as it is sent: itself when it is an Error, an unknown
// error otherwise.
func failure(err error) *Error {
	var known *Error
	if errors.As(err, &known) {
		return known
	}
	return &Error{Code: UnknownError, Message: err.Error()}
}
