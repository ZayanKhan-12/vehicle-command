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
	"time"

	"golang.org/x/oauth2"
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

// fakeTokenSource hands out a fresh, numbered token each time it is consulted,
// so that a test can tell one token apart from the next.
type fakeTokenSource struct {
	mu       sync.Mutex
	calls    int
	lifetime time.Duration // zero means the token is already expired
	err      error
}

func (s *fakeTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	s.calls++
	expiry := time.Now().Add(-time.Second)
	if s.lifetime > 0 {
		expiry = time.Now().Add(s.lifetime)
	}
	return &oauth2.Token{
		AccessToken: makeTestJWT(&oauthPayload{
			Audiences: []string{"https://fleet-api.prd.eu.vn.cloud.tesla.com"},
			OUCode:    "EU",
			Subject:   fmt.Sprintf("SUBJECT-%d", s.calls),
		}),
		TokenType: "Bearer",
		Expiry:    expiry,
	}, nil
}

func (s *fakeTokenSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// authRecorder is an httptest handler that records the Authorization header of
// every request it serves.
type authRecorder struct {
	mu      sync.Mutex
	headers []string
}

func (r *authRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.headers = append(r.headers, req.Header.Get("Authorization"))
	r.mu.Unlock()
	w.Write([]byte(`{"response": {"state": "online"}}`))
}

func (r *authRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.headers...)
}

// subjectOf extracts the Subject claim from a "Bearer <jwt>" header, which is
// how these tests identify which token was used.
func subjectOf(t *testing.T, header string) string {
	t.Helper()
	jwt, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		t.Fatalf("header %q is not a bearer token", header)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("header %q does not carry a JWT", header)
	}
	body, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("token payload is not base64: %s", err)
	}
	var payload oauthPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("token payload is not JSON: %s", err)
	}
	return payload.Subject
}

// TestNewFromTokenSource checks that the region is derived from the first token
// the source produces, the same way New derives it from a token string.
func TestNewFromTokenSource(t *testing.T) {
	source := &fakeTokenSource{lifetime: time.Hour}
	acct, err := NewFromTokenSource(source, "")
	if err != nil {
		t.Fatalf("NewFromTokenSource failed: %s", err)
	}
	if want := "fleet-api.prd.eu.vn.cloud.tesla.com"; acct.Host != want {
		t.Errorf("Host = %s, want %s", acct.Host, want)
	}
	if acct.Subject != "SUBJECT-1" {
		t.Errorf("Subject = %s, want SUBJECT-1", acct.Subject)
	}
	if source.callCount() != 1 {
		t.Errorf("source consulted %d times during construction, want 1", source.callCount())
	}
	// The Account must not be holding a copy of that first token: that is the
	// bug this feature exists to remove.
	if acct.authHeader != "" {
		t.Errorf("authHeader = %q, want empty so the token source is the only source of truth", acct.authHeader)
	}
}

// TestNewFromTokenSourceErrors checks the two ways construction can fail.
func TestNewFromTokenSourceErrors(t *testing.T) {
	if _, err := NewFromTokenSource(nil, ""); err == nil {
		t.Error("expected an error from a nil TokenSource")
	}
	failing := &fakeTokenSource{err: fmt.Errorf("no network")}
	if _, err := NewFromTokenSource(failing, ""); err == nil {
		t.Error("expected an error when the source cannot produce a token")
	}
}

// TestTokenSourceRefreshes is the point of the feature: an expired token is
// replaced without the caller rebuilding the Account.
func TestTokenSourceRefreshes(t *testing.T) {
	recorder := &authRecorder{}
	server := httptest.NewTLSServer(recorder)
	defer server.Close()

	source := &fakeTokenSource{} // every token is already expired
	acct, err := NewFromTokenSource(source, "", WithClient(server.Client()))
	if err != nil {
		t.Fatalf("NewFromTokenSource failed: %s", err)
	}
	acct.Host, _ = strings.CutPrefix(server.URL, "https://")

	for i := 0; i < 3; i++ {
		if _, err := acct.Get(context.Background(), "api/1/users/me"); err != nil {
			t.Fatalf("request %d failed: %s", i, err)
		}
	}

	headers := recorder.seen()
	if len(headers) != 3 {
		t.Fatalf("server saw %d requests, want 3", len(headers))
	}
	want := []string{"SUBJECT-2", "SUBJECT-3", "SUBJECT-4"} // SUBJECT-1 was spent deriving the region
	got := []string{subjectOf(t, headers[0]), subjectOf(t, headers[1]), subjectOf(t, headers[2])}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens used = %v, want %v", got, want)
	}
}

// TestTokenSourceReusesValidToken checks the other half of the contract: a
// token that is still good is not thrown away, so enabling refresh does not
// turn one API call into two.
func TestTokenSourceReusesValidToken(t *testing.T) {
	recorder := &authRecorder{}
	server := httptest.NewTLSServer(recorder)
	defer server.Close()

	source := &fakeTokenSource{lifetime: time.Hour}
	acct, err := NewFromTokenSource(source, "", WithClient(server.Client()))
	if err != nil {
		t.Fatalf("NewFromTokenSource failed: %s", err)
	}
	acct.Host, _ = strings.CutPrefix(server.URL, "https://")

	for i := 0; i < 3; i++ {
		if _, err := acct.Get(context.Background(), "api/1/users/me"); err != nil {
			t.Fatalf("request %d failed: %s", i, err)
		}
	}

	headers := recorder.seen()
	if a, b, c := subjectOf(t, headers[0]), subjectOf(t, headers[1]), subjectOf(t, headers[2]); a != b || b != c {
		t.Errorf("a valid token was not reused: %s, %s, %s", a, b, c)
	}
	// Once to learn the region, once for the first request; then cached.
	if source.callCount() != 2 {
		t.Errorf("source consulted %d times, want 2", source.callCount())
	}
}

// TestTokenSourceReachesVehicles checks that a Vehicle obtained from the
// Account refreshes too. Without this, long-running programs would keep working
// at the account level and start failing at the command level.
func TestTokenSourceReachesVehicles(t *testing.T) {
	recorder := &authRecorder{}
	server := httptest.NewTLSServer(recorder)
	defer server.Close()

	source := &fakeTokenSource{}
	acct, err := NewFromTokenSource(source, "", WithClient(server.Client()))
	if err != nil {
		t.Fatalf("NewFromTokenSource failed: %s", err)
	}
	acct.Host, _ = strings.CutPrefix(server.URL, "https://")

	car, err := acct.GetVehicle(context.Background(), "VIN123", nil, nil)
	if err != nil {
		t.Fatalf("GetVehicle failed: %s", err)
	}
	defer car.Disconnect()

	if err := car.Wakeup(context.Background()); err != nil {
		t.Fatalf("Wakeup failed: %s", err)
	}
	headers := recorder.seen()
	if len(headers) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(headers))
	}
	if got := subjectOf(t, headers[0]); got != "SUBJECT-2" {
		t.Errorf("vehicle used token %s, want the refreshed SUBJECT-2", got)
	}
}

// TestTokenSourceComposesWithClient checks that the two options do not fight,
// in either order: the caller's Transport still carries the request, and the
// token source still supplies the credentials.
func TestTokenSourceComposesWithClient(t *testing.T) {
	for _, order := range []string{"client first", "token source first"} {
		t.Run(order, func(t *testing.T) {
			recorder := &authRecorder{}
			server := httptest.NewTLSServer(recorder)
			defer server.Close()

			transport := &countingTransport{base: server.Client().Transport}
			source := &fakeTokenSource{lifetime: time.Hour}
			options := []Option{WithClient(&http.Client{Transport: transport}), WithTokenSource(source)}
			if order == "token source first" {
				options[0], options[1] = options[1], options[0]
			}

			acct, err := New(makeTestJWT(&oauthPayload{Audiences: []string{"https://auth.tesla.com/nts"}}), "", options...)
			if err != nil {
				t.Fatalf("New failed: %s", err)
			}
			acct.Host, _ = strings.CutPrefix(server.URL, "https://")

			if _, err := acct.Get(context.Background(), "api/1/users/me"); err != nil {
				t.Fatalf("Get failed: %s", err)
			}
			if got := transport.seen(); !reflect.DeepEqual(got, []string{"/api/1/users/me"}) {
				t.Errorf("custom transport saw %v, want the request to pass through it", got)
			}
			if got := subjectOf(t, recorder.seen()[0]); got != "SUBJECT-1" {
				t.Errorf("token source did not supply credentials: subject %s", got)
			}
		})
	}
}

// TestStaticTokenUnchanged guards the existing path: an Account built from a
// token string still sends that token, and still sends it on vehicle requests.
func TestStaticTokenUnchanged(t *testing.T) {
	recorder := &authRecorder{}
	server := httptest.NewTLSServer(recorder)
	defer server.Close()

	token := makeTestJWT(&oauthPayload{Audiences: []string{"https://auth.tesla.com/nts"}, Subject: "STATIC"})
	acct := testAccount(t, server, WithClient(server.Client()))
	acct.authHeader = "Bearer " + token

	if _, err := acct.Get(context.Background(), "api/1/users/me"); err != nil {
		t.Fatalf("Get failed: %s", err)
	}
	if got := subjectOf(t, recorder.seen()[0]); got != "STATIC" {
		t.Errorf("subject = %s, want STATIC", got)
	}
}

// TestGetVehicleAdoptsRegionRedirect checks that when a vehicle connection is
// told it is talking to the wrong regional server, the Account records the
// correction.
//
// Without this the knowledge dies with the connection: the Account keeps
// handing out vehicles pointed at the wrong region, and anything that reads
// acct.Host afterwards -- notably the HTTP proxy, when it falls back to
// forwarding a request for a vehicle that turns out not to support the command
// protocol -- sends it to the region Tesla has already rejected. That is the
// failure in issue #131.
func TestGetVehicleAdoptsRegionRedirect(t *testing.T) {
	const correctHost = "fleet-api.prd.na.vn.cloud.tesla.com"
	body := `{"response":null,"error":"user out of region, use base URL: https://` + correctHost +
		`, see https://developer.tesla.com/docs/fleet-api#regional-requirements","error_description":""}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMisdirectedRequest)
		w.Write([]byte(body))
	}))
	defer server.Close()

	acct := testAccount(t, server, WithClient(server.Client()))
	misroutedHost := acct.Host

	car, err := acct.GetVehicle(context.Background(), "VIN123", nil, nil)
	if err != nil {
		t.Fatalf("GetVehicle failed: %s", err)
	}
	defer car.Disconnect()

	// Wakeup treats HTTP 421 as retryable and would otherwise wait before
	// trying again; one attempt is all this needs, so bound it with a context.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = car.Wakeup(ctx)

	if acct.Host == misroutedHost {
		t.Errorf("Host is still %s; the redirect was not recorded", acct.Host)
	}
	if acct.Host != correctHost {
		t.Errorf("Host = %s, want %s", acct.Host, correctHost)
	}
}

// TestGetVehicleIgnoresUntrustedRedirect checks that the Account does not adopt
// a host outside Tesla's domains. acct.Host decides where the OAuth token is
// sent, so a redirect named by a response body must be checked before it is
// believed.
func TestGetVehicleIgnoresUntrustedRedirect(t *testing.T) {
	body := `{"response":null,"error":"user out of region, use base URL: https://fleet-api.attacker.example.com, see https://developer.tesla.com/docs/fleet-api#regional-requirements","error_description":""}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMisdirectedRequest)
		w.Write([]byte(body))
	}))
	defer server.Close()

	acct := testAccount(t, server, WithClient(server.Client()))
	misroutedHost := acct.Host

	car, err := acct.GetVehicle(context.Background(), "VIN123", nil, nil)
	if err != nil {
		t.Fatalf("GetVehicle failed: %s", err)
	}
	defer car.Disconnect()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = car.Wakeup(ctx)

	if acct.Host != misroutedHost {
		t.Errorf("Host = %s, want it unchanged at %s", acct.Host, misroutedHost)
	}
}
