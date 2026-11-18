package identity

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// System is the languages the user of this machine prefers, most
// wanted first: those macOS keeps, else the environment's. It asks
// macOS, so it is read once, as light starts.
func System() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
		if tags := Tags(string(out)); len(tags) > 0 {
			return tags
		}
	}
	return Environment(os.Getenv)
}
