package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveFrom sends every request to the test server regardless of the host asked
// for, then restores the original request on the response. Restoring it matters:
// checkDomain treats a change of host as a redirect, and the rewrite is an
// artefact of the test rather than something the operator did.
type serveFrom struct {
	base http.RoundTripper
	host string
}

func (t serveFrom) RoundTrip(req *http.Request) (*http.Response, error) {
	routed := req.Clone(req.Context())
	routed.URL.Host = t.host
	rsp, err := t.base.RoundTrip(routed)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

// clientFor returns a client that reaches server no matter which domain is
// requested.
func clientFor(server *httptest.Server) *http.Client {
	host := strings.TrimPrefix(server.URL, "https://")
	return &http.Client{Transport: serveFrom{base: server.Client().Transport, host: host}}
}

func pemPublicKey(t *testing.T, pub interface{}) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshalling public key: %s", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func p256Key(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %s", err)
	}
	return key
}

// serving returns a server that answers the well-known path with body, and 404
// everywhere else, so a test that gets the path wrong fails loudly.
func serving(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != KeyPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

// run returns the checks as a map from name to check, for readable assertions.
func run(t *testing.T, server *httptest.Server, domain string, want *ecdsa.PublicKey) map[string]check {
	t.Helper()
	byName := make(map[string]check)
	for _, c := range checkDomain(context.Background(), clientFor(server), domain, want) {
		byName[c.name] = c
	}
	return byName
}

func TestPublishedKeyIsAccepted(t *testing.T) {
	key := p256Key(t)
	checks := run(t, serving(t, pemPublicKey(t, &key.PublicKey)), "example.com", nil)

	for _, name := range []string{"domain", "fetch", "pem", "key"} {
		c, ok := checks[name]
		if !ok {
			t.Errorf("check %q did not run", name)
			continue
		}
		if c.status != statusOK {
			t.Errorf("check %q: %s", name, c)
		}
	}
	if _, ok := checks["redirect"]; ok {
		t.Error("a redirect was reported where none happened")
	}
}

func TestPublishedKeyMatchesLocalKey(t *testing.T) {
	key := p256Key(t)
	server := serving(t, pemPublicKey(t, &key.PublicKey))

	if c := run(t, server, "example.com", &key.PublicKey)["matches local key"]; c.status != statusOK {
		t.Errorf("a key that matches was reported as %s: %s", c.status, c.detail)
	}

	other := p256Key(t)
	if c := run(t, server, "example.com", &other.PublicKey)["matches local key"]; c.status != statusFail {
		t.Errorf("a key that does not match was reported as %s: %s", c.status, c.detail)
	}
}

// TestKeyNotPublished is the case behind most of the reports: the pairing link
// fails because the file is not where Tesla looks for it.
func TestKeyNotPublished(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	checks := run(t, server, "example.com", nil)
	c, ok := checks["fetch"]
	if !ok || c.status != statusFail {
		t.Fatalf("a missing key file was not reported as a failure: %+v", checks)
	}
	if !strings.Contains(c.detail, "404") || !strings.Contains(c.detail, KeyPath) {
		t.Errorf("detail %q should name the status and the path", c.detail)
	}
	if _, ran := checks["pem"]; ran {
		t.Error("parsing was attempted after the fetch failed")
	}
}

// TestLoginPageInsteadOfKey covers a host that answers 200 with a web page,
// which is what an access-controlled or catch-all site does. Without naming it,
// this looks identical to a corrupt key.
func TestLoginPageInsteadOfKey(t *testing.T) {
	checks := run(t, serving(t, []byte("<!DOCTYPE html>\n<html><body>Sign in</body></html>")), "example.com", nil)
	c := checks["pem"]
	if c.status != statusFail {
		t.Fatalf("an HTML page was accepted: %+v", c)
	}
	if !strings.Contains(c.detail, "HTML") {
		t.Errorf("detail %q should say the response looks like a web page", c.detail)
	}
}

func TestEmptyFile(t *testing.T) {
	c := run(t, serving(t, nil), "example.com", nil)["pem"]
	if c.status != statusFail || !strings.Contains(c.detail, "empty") {
		t.Errorf("an empty file was reported as %s: %s", c.status, c.detail)
	}
}

// TestWrongCurve covers the requirement that vehicles only accept prime256v1.
// A P-384 key is valid, publishes cleanly and is silently useless.
func TestWrongCurve(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %s", err)
	}
	c := run(t, serving(t, pemPublicKey(t, &key.PublicKey)), "example.com", nil)["key"]
	if c.status != statusFail {
		t.Fatalf("a P-384 key was accepted: %+v", c)
	}
	if !strings.Contains(c.detail, "prime256v1") {
		t.Errorf("detail %q should name the curve vehicles require", c.detail)
	}
}

func TestWrongKeyType(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %s", err)
	}
	c := run(t, serving(t, pemPublicKey(t, &key.PublicKey)), "example.com", nil)["key"]
	if c.status != statusFail || !strings.Contains(c.detail, "prime256v1") {
		t.Errorf("an RSA key was reported as %s: %s", c.status, c.detail)
	}
}

// TestPrivateKeyPublished is worth its own case because it is a live secret
// leak, not just a misconfiguration.
func TestPrivateKeyPublished(t *testing.T) {
	key := p256Key(t)
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling private key: %s", err)
	}
	body := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	c := run(t, serving(t, body), "example.com", nil)["pem"]
	if c.status != statusFail {
		t.Fatalf("a private key was accepted as the published key: %+v", c)
	}
	if !strings.Contains(c.detail, "EC PRIVATE KEY") {
		t.Errorf("detail %q should name the block type found", c.detail)
	}
}

func TestValidDomain(t *testing.T) {
	for _, test := range []struct {
		domain string
		ok     bool
		says   string
	}{
		{"example.com", true, ""},
		{"key.example.co.uk", true, ""},
		{"", false, "no domain"},
		{"https://example.com", false, "scheme"},
		{"example.com/keys", false, "path"},
		{"example.com:8443", false, "port"},
		{"user@example.com", false, "credentials"},
		{"localhost", false, "not a domain"},
	} {
		t.Run(test.domain, func(t *testing.T) {
			detail, ok := validDomain(test.domain)
			if ok != test.ok {
				t.Fatalf("validDomain(%q) = %v, want %v (%s)", test.domain, ok, test.ok, detail)
			}
			if !ok && !strings.Contains(detail, test.says) {
				t.Errorf("detail %q should mention %q", detail, test.says)
			}
		})
	}
}

func TestLoadPublicKey(t *testing.T) {
	key := p256Key(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "public.pem")
	if err := os.WriteFile(good, pemPublicKey(t, &key.PublicKey), 0600); err != nil {
		t.Fatalf("writing key: %s", err)
	}
	loaded, err := loadPublicKey(good)
	if err != nil {
		t.Fatalf("loadPublicKey: %s", err)
	}
	if !loaded.Equal(&key.PublicKey) {
		t.Error("loaded key differs from the one written")
	}

	bad := filepath.Join(dir, "not-a-key.txt")
	if err := os.WriteFile(bad, []byte("hello"), 0600); err != nil {
		t.Fatalf("writing file: %s", err)
	}
	if _, err := loadPublicKey(bad); err == nil {
		t.Error("a non-PEM file was accepted")
	}
}
