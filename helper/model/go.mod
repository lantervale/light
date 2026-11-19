// light-model is the helper that runs a language model for light. It is
// a module of its own so that what it is built on (yzma, which loads
// llama.cpp without cgo) stays out of light's own module: light never
// links it, and starts this program instead.
module github.com/lantervale/light/helper/model

go 1.27.0

require github.com/hybridgroup/yzma v1.33.0

require (
	github.com/ardanlabs/jinja v1.1.0 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/jupiterrider/ffi v0.8.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
)
