//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package script //nolint:testpackage // what the tests read of the engine that nothing else needs

// Retained is how many values of ctx the engine keeps for Go.
func Retained(ctx Context) int {
	c, ok := ctx.(*context)
	if !ok || c.ctx == nil {
		return 0
	}
	return c.ctx.RetainedValueCount()
}
