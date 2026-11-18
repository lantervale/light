// Package identity is what light tells a site of itself, of the machine
// it runs on and of each request, in plain Go: the User-Agent, the
// languages, and the headers a request carries by what it is for. Every
// value is true: none names another browser's engine or brand.
package identity

import (
	"runtime"
	"strings"
)

// Alpha is the version a build that was given none says it is.
const Alpha = "0.1.0-alpha"

// Version is the version light says it is: the one its build was given,
// Alpha for a build made by hand.
func Version(build string) string {
	build = strings.TrimPrefix(strings.TrimSpace(build), "v")
	if build == "" || build == "dev" {
		return Alpha
	}
	return build
}

// The systems as runtime.GOOS names them.
const (
	darwin  = "darwin"
	windows = "windows"
)

// UserAgent is the User-Agent of light at version on the system goos, as
// runtime.GOOS names it: the token every browser begins with, the
// platform as browsers froze it, and light.
func UserAgent(goos, version string) string {
	platform := "X11; Linux x86_64"
	switch goos {
	case darwin:
		platform = "Macintosh; Intel Mac OS X 10_15_7"
	case windows:
		platform = "Windows NT 10.0; Win64; x64"
	}
	return "Mozilla/5.0 (" + platform + ") light/" + version
}

// Default is the User-Agent of this build of light on this machine.
func Default(build string) string { return UserAgent(runtime.GOOS, Version(build)) }

// Platform is what navigator.platform says on the system goos and the
// processor goarch: what browsers say there, a Mac of any processor
// being MacIntel.
func Platform(goos, goarch string) string {
	switch goos {
	case darwin:
		return "MacIntel"
	case windows:
		return "Win32"
	}
	switch goarch {
	case "amd64":
		goarch = "x86_64"
	case "arm64":
		goarch = "aarch64"
	}
	return "Linux " + goarch
}

// Driven reports whether a run with that environment is driven by
// something else than a person at its window: ember's automation
// server, a capture and its scripted input, or a window that is hidden.
// It is what navigator.webdriver says.
func Driven(environ []string) bool {
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		if value == "" {
			continue
		}
		if name == "EMBER_AUTOMATION" || name == "EMBER_HIDDEN" || strings.HasPrefix(name, "EMBER_CAPTURE") {
			return true
		}
	}
	return false
}
