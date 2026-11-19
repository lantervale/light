//go:build !(v8 && cgo && (darwin || linux) && (amd64 || arm64))

package script_test

import (
	"errors"
	"testing"

	"github.com/lantervale/light/pkg/script"
)

func TestABuildWithoutAnEngineSaysSo(t *testing.T) {
	if script.Available() || script.Engine() != "" {
		t.Fatalf("a build without an engine names %q", script.Engine())
	}
	if ctx, err := script.New(script.Options{}); ctx != nil || !errors.Is(err, script.ErrUnavailable) {
		t.Fatalf("a context was made: %v, %v", ctx, err)
	}
}
