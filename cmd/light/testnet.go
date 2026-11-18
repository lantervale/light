package main

import (
	"errors"
	"net/http"

	"github.com/lantervale/light/pkg/testnet"
)

// Validate refuses the test network's options to a light nothing
// drives: they are a test runner's, never a person's.
func (c CLI) Validate() error {
	if c.WebDriver == "" && (len(c.HostMap) > 0 || c.TrustCA != "") {
		return errors.New("--host-map and --trust-ca are for a run driven by a test runner: they need --webdriver")
	}
	return nil
}

// transport makes the requests of a run given a test network, nil for
// any other.
func (c CLI) transport() (http.RoundTripper, error) {
	if len(c.HostMap) == 0 && c.TrustCA == "" {
		return nil, nil //nolint:nilnil // no transport is light's own
	}
	hosts, err := testnet.ParseHosts(c.HostMap)
	if err != nil {
		return nil, err
	}
	t, err := testnet.Transport(hosts, c.TrustCA)
	if err != nil {
		return nil, err
	}
	return t, nil
}
