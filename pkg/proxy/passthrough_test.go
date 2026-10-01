package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNonCommandEndpointsAreForwarded records what the proxy does with the rest
// of the Fleet API.
//
// Only "/api/1/vehicles/{vin}/command/{name}" is interpreted, because only a
// vehicle command has to be signed with the application's private key.
// Everything else -- energy sites, user endpoints, vehicle_data -- is relayed
// with the caller's own OAuth token and is not the proxy's business.
//
// This comes up as a question rather than a bug: issue #219 asks whether
// utility rate plan data is reachable. It is, and always was, because
// /api/1/energy_sites/... never enters the command path. The risk is that a
// future change to the routing in ServeHTTP swallows these paths, which would
// break them silently, so the behaviour is pinned here.
func TestNonCommandEndpointsAreForwarded(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"energy site tariff", http.MethodGet, "/api/1/energy_sites/123456/tariff_rate", ""},
		{"energy site time of use", http.MethodPost, "/api/1/energy_sites/123456/time_of_use_settings", `{"tou_settings":{}}`},
		{"vehicle data is not a command", http.MethodGet, "/api/1/vehicles/" + testVIN + "/vehicle_data", ""},
		{"products", http.MethodGet, "/api/1/products", ""},
		{"user", http.MethodGet, "/api/1/users/me", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := newTestProxy(t)

			var gotHost, gotPath, gotMethod, gotBody string
			forwarded := false
			p.client = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				forwarded = true
				gotHost, gotPath, gotMethod = req.URL.Host, req.URL.Path, req.Method
				if req.Body != nil {
					raw, err := io.ReadAll(req.Body)
					if err != nil {
						return nil, err
					}
					gotBody = string(raw)
				}
				return jsonResponse(http.StatusOK, `{"response":{"ok":true}}`, nil), nil
			})

			var reader io.Reader
			if test.body != "" {
				reader = strings.NewReader(test.body)
			}
			req := httptest.NewRequest(test.method, test.path, reader)
			req.Header.Set("Authorization", authHeader())
			rec := httptest.NewRecorder()

			p.ServeHTTP(rec, req)

			if !forwarded {
				t.Fatalf("request was answered by the proxy (status %d, body %s) instead of being forwarded",
					rec.Code, rec.Body.String())
			}
			if gotHost != testHost {
				t.Errorf("forwarded to host %q, want %q", gotHost, testHost)
			}
			if gotPath != test.path {
				t.Errorf("forwarded path %q, want %q", gotPath, test.path)
			}
			if gotMethod != test.method {
				t.Errorf("forwarded method %q, want %q", gotMethod, test.method)
			}
			if gotBody != test.body {
				t.Errorf("forwarded body %q, want %q", gotBody, test.body)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("client saw status %d, want the upstream 200", rec.Code)
			}
		})
	}
}

// TestVehicleCommandPathIsNotForwardedBlindly is the counterweight: the one
// shape that must not be relayed untouched is a vehicle command, which the
// proxy exists to sign. Without this, a change that widened the pass-through
// above would go unnoticed.
func TestVehicleCommandPathIsNotForwardedBlindly(t *testing.T) {
	p := newTestProxy(t)
	forwarded := false
	p.client = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		forwarded = true
		return jsonResponse(http.StatusOK, `{}`, nil), nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/1/vehicles/"+testVIN+"/command/not_a_command", strings.NewReader("{}"))
	req.Header.Set("Authorization", authHeader())
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	if forwarded {
		t.Error("an unrecognised vehicle command was relayed to Tesla unsigned")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
