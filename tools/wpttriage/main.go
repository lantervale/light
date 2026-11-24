// Command wpttriage groups the failures of a web-platform-tests run by cause and ranks the causes by the tests they
// would unlock: go run ./tools/wpttriage -report .wpt/results/wptreport.json -json triage.json -md triage.md.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lantervale/light/tools/wpttriage/triage"
)

func main() {
	report := flag.String("report", ".wpt/results/wptreport.json", "the wptreport.json of a run")
	jsonOut := flag.String("json", "", "where the groups go as JSON")
	mdOut := flag.String("md", "", "where the report goes as Markdown (standard output when empty)")
	top := flag.Int("top", 60, "the groups the Markdown report shows")
	flag.Parse()
	if err := run(*report, *jsonOut, *mdOut, *top); err != nil {
		fmt.Fprintln(os.Stderr, "wpttriage:", err)
		os.Exit(1)
	}
}

func run(reportPath, jsonOut, mdOut string, top int) error {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return err
	}
	var report triage.Report
	if err = json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("%s: %w", reportPath, err)
	}
	res := triage.Triage(&report)
	if jsonOut != "" {
		out, merr := json.MarshalIndent(res, "", "  ")
		if merr != nil {
			return merr
		}
		if err = os.WriteFile(jsonOut, append(out, '\n'), 0o600); err != nil {
			return err
		}
	}
	if mdOut == "" {
		return triage.Markdown(os.Stdout, res, top)
	}
	f, err := os.Create(mdOut)
	if err != nil {
		return err
	}
	if err = triage.Markdown(f, res, top); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
