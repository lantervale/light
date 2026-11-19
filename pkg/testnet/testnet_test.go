package testnet_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/localnet"
	"github.com/lantervale/light/pkg/testnet"
)

func hosts(t *testing.T, list map[string]string) testnet.Hosts {
	t.Helper()
	h, err := testnet.ParseHosts(list)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func port(t *testing.T, server *httptest.Server) string {
	t.Helper()
	_, p, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func get(ctx context.Context, transport http.RoundTripper, address string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func said(text string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, text) })
}

// authority is a certificate authority made for one test, in a file,
// and a server whose certificate it signed for names.
func authority(t *testing.T, names ...string) (string, *httptest.Server) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "a test's authority"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(said("secure"))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: key}},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return file, server
}

func TestAMappedNameAndItsSubdomainsAreThisMachine(t *testing.T) {
	h := hosts(t, map[string]string{"Web-Platform.test": "127.0.0.1", "other.test.": "::1"})
	for name, want := range map[string]string{
		"web-platform.test": "127.0.0.1", "WWW.web-platform.test": "127.0.0.1", "a.b.web-platform.test.": "127.0.0.1",
		"other.test": "::1", "notweb-platform.test": "", "test": "", "web-platform.test.example.com": "", "": "",
	} {
		at, mapped := h.Lookup(name)
		if got := map[bool]string{true: at.String()}[mapped]; got != want {
			t.Errorf("%q is at %q, want %q", name, got, want)
		}
	}
}

func TestOnlyNamesMappedToThisMachineAreTaken(t *testing.T) {
	for _, list := range []map[string]string{
		{"web-platform.test": "192.168.1.10"}, {"web-platform.test": "93.184.216.34"}, {"web-platform.test": "here"},
		{"web-platform.test": ""}, {"": "127.0.0.1"}, {"127.0.0.1": "127.0.0.1"}, {"a b": "127.0.0.1"},
		{"*.test": "127.0.0.1"},
	} {
		if _, err := testnet.ParseHosts(list); err == nil {
			t.Errorf("%v is taken", list)
		}
	}
}

func TestARequestToAMappedNameIsDialledOnThisMachine(t *testing.T) {
	server := httptest.NewServer(said("here"))
	defer server.Close()
	transport, err := testnet.Transport(hosts(t, map[string]string{"web-platform.test": "127.0.0.1"}), "")
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	for _, name := range []string{"web-platform.test", "www1.web-platform.test"} {
		got, failed := get(t.Context(), transport, "http://"+name+":"+port(t, server)+"/")
		if failed != nil || got != "here" {
			t.Errorf("%s answers %q, %v", name, got, failed)
		}
	}
	if _, err = get(t.Context(), transport, "http://unmapped.invalid:"+port(t, server)+"/"); err == nil {
		t.Error("a name that is not mapped reached the server")
	}
	if got, failed := get(t.Context(), transport, server.URL); failed != nil || got != "here" {
		t.Errorf("an address answers %q, %v", got, failed)
	}
}

func TestAMappedNameIsNotAskedOfAProxy(t *testing.T) {
	server := httptest.NewServer(said("here"))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	transport, err := testnet.Transport(hosts(t, map[string]string{"web-platform.test": "127.0.0.1"}), "")
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	if got, failed := get(t.Context(), transport, "http://web-platform.test:"+port(t, server)+"/"); got != "here" {
		t.Fatalf("a mapped name behind a proxy answers %q, %v", got, failed)
	}
}

func TestTheAuthorityGivenIsTrustedAndTheOthersStillChecked(t *testing.T) {
	file, server := authority(t, "web-platform.test")
	_, stranger := authority(t, "web-platform.test")
	names := hosts(t, map[string]string{"web-platform.test": "127.0.0.1", "other.test": "127.0.0.1"})
	trusting, err := testnet.Transport(names, file)
	if err != nil {
		t.Fatal(err)
	}
	defer trusting.CloseIdleConnections()
	if got, failed := get(t.Context(), trusting, "https://web-platform.test:"+port(t, server)+"/"); got != "secure" {
		t.Fatalf("the authority's server answers %q, %v", got, failed)
	}
	var unknown x509.UnknownAuthorityError
	if _, err = get(t.Context(), trusting, "https://web-platform.test:"+port(t, stranger)+"/"); !errors.As(err, &unknown) {
		t.Errorf("a server of another authority: %v", err)
	}
	var name x509.HostnameError
	if _, err = get(t.Context(), trusting, "https://other.test:"+port(t, server)+"/"); !errors.As(err, &name) {
		t.Errorf("a certificate for another name: %v", err)
	}
	plain, err := testnet.Transport(names, "")
	if err != nil {
		t.Fatal(err)
	}
	defer plain.CloseIdleConnections()
	_, err = get(t.Context(), plain, "https://web-platform.test:"+port(t, server)+"/")
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("without the authority: %v", err)
	}
	if _, err = get(t.Context(), http.DefaultTransport, server.URL); err == nil {
		t.Error("the authority is trusted outside the transport it was given to")
	}
}

func TestAFileThatHoldsNoCertificateIsRefused(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(empty, []byte("no certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{empty, filepath.Join(t.TempDir(), "missing.pem")} {
		if _, err := testnet.Transport(nil, file); err == nil || !strings.Contains(err.Error(), file) {
			t.Errorf("%s: %v", file, err)
		}
	}
}

func TestAPageOfTheInternetStillDoesNotReachAMappedName(t *testing.T) {
	server := httptest.NewServer(said("here"))
	defer server.Close()
	transport, err := testnet.Transport(hosts(t, map[string]string{"web-platform.test": "127.0.0.1"}), "")
	if err != nil {
		t.Fatal(err)
	}
	guard := &localnet.Guard{Base: transport}
	defer guard.CloseIdleConnections()
	address := "http://web-platform.test:" + port(t, server) + "/"
	var blocked *localnet.BlockedError
	for _, from := range []localnet.Space{localnet.Public, localnet.Private} {
		_, err = get(localnet.From(t.Context(), from), guard, address)
		if !errors.As(err, &blocked) || blocked.To != localnet.Loopback {
			t.Errorf("a page of the %s network asking a mapped name: %v", from, err)
		}
	}
	if got, failed := get(localnet.From(t.Context(), localnet.Loopback), guard, address); got != "here" {
		t.Errorf("a page of this machine asking a mapped name gets %q, %v", got, failed)
	}
	if got, failed := get(t.Context(), guard, address); got != "here" {
		t.Errorf("the user asking a mapped name gets %q, %v", got, failed)
	}
}
