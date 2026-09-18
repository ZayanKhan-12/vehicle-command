package account

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// b64Encode encodes a string to base64 without padding.
func b64Encode(payload string) string {
	return base64.RawStdEncoding.EncodeToString([]byte(payload))
}

// TestNewAccount tests the creation of a new account with various JWT scenarios.
func TestNewAccount(t *testing.T) {
	validDomain := "fleet-api.example.tesla.com"

	tests := []struct {
		jwt         string
		shouldError bool
		description string
	}{
		{"", true, "empty JWT"},
		{b64Encode(validDomain), true, "one-field JWT"},
		{"x." + b64Encode(validDomain), true, "two-field JWT"},
		{"x." + b64Encode(validDomain) + "y.z", true, "four-field JWT"},
		{"x." + validDomain + ".y", true, "non-base64 encoded JWT"},
		{"x." + b64Encode("{\"aud\": \"example.com\"}") + ".y", true, "untrusted domain"},
		{"x." + b64Encode(fmt.Sprintf("{\"aud\": \"%s\"}", validDomain)) + ".y", true, "aud field not a list"},
		{"x." + b64Encode(fmt.Sprintf("{\"aud\": [\"%s\"]}", validDomain)) + ".y", false, "valid JWT"},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			acct, err := New(test.jwt, "")
			if (err != nil) != test.shouldError {
				t.Errorf("Unexpected result: err = %v, shouldError = %v", err, test.shouldError)
			}
			if !test.shouldError && (acct == nil || acct.Host != validDomain) {
				t.Errorf("acct = %+v, expected Host = %s", acct, validDomain)
			}
		})
	}
}

// TestDomainDefault tests the default domain extraction.
func TestDomainDefault(t *testing.T) {
	payload := &oauthPayload{
		Audiences: []string{"https://auth.tesla.com/nts"},
	}

	acct, err := New(makeTestJWT(payload), "")
	if err != nil {
		t.Fatalf("Returned error on valid JWT: %s", err)
	}
	if acct == nil || acct.Host != defaultDomain {
		t.Errorf("acct = %+v, expected Host = %s", acct, defaultDomain)
	}
}

// TestDomainExtraction tests the extraction of the correct domain based on OUCode.
func TestDomainExtraction(t *testing.T) {
	payload := &oauthPayload{
		Audiences: []string{
			"https://auth.tesla.com/nts",
			"https://fleet-api.prd.na.vn.cloud.tesla.com",
			"https://fleet-api.prd.eu.vn.cloud.tesla.com",
		},
		OUCode:  "EU",
		Subject: "SUBJECT",
	}

	acct, err := New(makeTestJWT(payload), "")
	if err != nil {
		t.Fatalf("Returned error on valid JWT: %s", err)
	}
	expectedHost := "fleet-api.prd.eu.vn.cloud.tesla.com"
	if acct == nil || acct.Host != expectedHost || acct.Subject != "SUBJECT" {
		t.Errorf("acct = %+v, expected Host = %s", acct, expectedHost)
	}
}

// makeTestJWT creates a JWT string with the given payload.
func makeTestJWT(payload *oauthPayload) string {
	jwtBody, _ := json.Marshal(payload)
	return fmt.Sprintf("x.%s.y", b64Encode(string(jwtBody)))
}

// countingTransport forwards requests to base and records the paths it saw, so
// that a test can assert an injected client (and therefore an injected
// [http.RoundTripper]) is the one carrying the traffic.
type countingTransport struct {
	base  http.RoundTripper
	mu    sync.Mutex
	paths []string
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.paths = append(t.paths, req.URL.Path)
	t.mu.Unlock()
	return t.base.RoundTrip(req)
}

func (t *countingTransport) seen() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.paths...)
}

// testAccount returns an Account whose Host points at server. The JWT audience
// cannot name server's address -- it is not a Tesla domain -- so Host is
// overridden after construction.
func testAccount(t *testing.T, server *httptest.Server, options ...Option) *Account {
	t.Helper()
	acct, err := New(makeTestJWT(&oauthPayload{Audiences: []string{"https://auth.tesla.com/nts"}}), "", options...)
	if err != nil {
		t.Fatalf("New returned error on valid JWT: %s", err)
	}
	acct.Host, _ = strings.CutPrefix(server.URL, "https://")
	return acct
}

// TestDefaultClientRejectsTestServer establishes the baseline the other tests in
// this group depend on: the Account's default client does not trust the
// httptest server's self-signed certificate, so a successful request can only
// come from an injected client.
func TestDefaultClientRejectsTestServer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"response": ""}`))
	}))
	defer server.Close()

	acct := testAccount(t, server)
	if _, err := acct.Get(context.Background(), "api/1/users/me"); err == nil {
		t.Fatal("expected the default client to reject the test server's certificate")
	}
}

// TestWithClient checks that WithClient replaces the client the Account uses.
func TestWithClient(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"response": "ok"}`))
	}))
	defer server.Close()

	acct := testAccount(t, server, WithClient(server.Client()))
	body, err := acct.Get(context.Background(), "api/1/users/me")
	if err != nil {
		t.Fatalf("Get through injected client failed: %s", err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Errorf("unexpected body %q", body)
	}
}

// TestWithClientUsesCustomTransport covers the use case from the issue: a
// custom http.RoundTripper, wrapped around the client, observes the requests.
func TestWithClientUsesCustomTransport(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"response": "ok"}`))
	}))
	defer server.Close()

	transport := &countingTransport{base: server.Client().Transport}
	acct := testAccount(t, server, WithClient(&http.Client{Transport: transport}))

	if _, err := acct.Get(context.Background(), "api/1/users/me"); err != nil {
		t.Fatalf("Get through injected client failed: %s", err)
	}
	if _, err := acct.Post(context.Background(), "api/1/users/keys", []byte(`{}`)); err != nil {
		t.Fatalf("Post through injected client failed: %s", err)
	}

	want := []string{"/api/1/users/me", "/api/1/users/keys"}
	if got := transport.seen(); !reflect.DeepEqual(got, want) {
		t.Errorf("custom transport saw %v, want %v", got, want)
	}
}

// TestWithClientReachesVehicles checks that a Vehicle obtained from the Account
// inherits the Account's client. Without the inheritance a caller can configure
// the Account's transport and still have every vehicle command bypass it.
func TestWithClientReachesVehicles(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"response": {"state": "online"}}`))
	}))
	defer server.Close()

	transport := &countingTransport{base: server.Client().Transport}
	acct := testAccount(t, server, WithClient(&http.Client{Transport: transport}))

	car, err := acct.GetVehicle(context.Background(), "VIN123", nil, nil)
	if err != nil {
		t.Fatalf("GetVehicle failed: %s", err)
	}
	defer car.Disconnect()

	if err := car.Wakeup(context.Background()); err != nil {
		t.Fatalf("Wakeup through inherited client failed: %s", err)
	}
	want := []string{"/api/1/vehicles/VIN123/wake_up"}
	if got := transport.seen(); !reflect.DeepEqual(got, want) {
		t.Errorf("custom transport saw %v, want %v", got, want)
	}
}

// TestWithNilClient checks that WithClient(nil) leaves a usable client in place
// rather than producing an Account that panics on first use.
func TestWithNilClient(t *testing.T) {
	acct, err := New(makeTestJWT(&oauthPayload{Audiences: []string{"https://auth.tesla.com/nts"}}), "", WithClient(nil))
	if err != nil {
		t.Fatalf("New returned error on valid JWT: %s", err)
	}
	if acct.client == nil {
		t.Error("WithClient(nil) left the Account without a client")
	}
}
