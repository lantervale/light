package devtools //nolint:testpackage // the keys of every system, not only the one the tests run on

import (
	"strings"
	"testing"
)

func TestTheConsolesHistoryTakesControlOnlyOnMacOS(t *testing.T) {
	for system, control := range map[string]bool{"darwin": true, "windows": false, "linux": false} {
		previous, next := historyKeys(system)
		if !strings.Contains(previous, "Alt+ArrowUp") || !strings.Contains(next, "Alt+ArrowDown") {
			t.Errorf("on %s the history is walked by %q and %q, without Alt and the arrows", system, previous, next)
		}
		if strings.Contains(previous+next, "Control+") != control {
			t.Errorf("on %s the history is walked by %q and %q", system, previous, next)
		}
	}
}
