// light-model runs a language model for light, the browser, which
// starts it and speaks to it over its standard input and output (one
// JSON value a line: see internal/wire). It is not a program to run by
// hand: it reads only the files light names, opens no connection,
// writes nothing but its answers, and leaves when light does.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lantervale/light/helper/model/internal/engine"
	"github.com/lantervale/light/helper/model/internal/serve"
)

// version is the light this helper was released with, "dev" for one
// built from the sources: a release's build sets it.
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "light-model %s runs a language model for light, which starts it: it takes no argument.\n",
			version)
		os.Exit(2)
	}
	release := version != "dev" && !strings.Contains(version, "SNAPSHOT")
	if err := serve.Run(os.Stdin, os.Stdout, &engine.Engine{}, serve.Options{Build: version,
		Release: release}); err != nil {
		fmt.Fprintln(os.Stderr, "light-model:", err)
		os.Exit(1)
	}
}
