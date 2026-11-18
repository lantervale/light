package dom

import "github.com/lantervale/ember/ui"

// Inspect names n, the node the developer tools have selected, for the
// console: its $0, null for nil.
func (r *Runtime) Inspect(n *ui.Node) {
	r.free(func() { _, _ = r.quietly("inspect", r.refOf(n)) })
}
