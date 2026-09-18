package inet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teslamotors/vehicle-command/pkg/protocol"
)

func TestSendAfterClose(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"response": ""}`))
	}))
	defer server.Close()
	domain, _ := strings.CutPrefix(server.URL, "https://")
	conn := NewConnection("VIN123", "", domain, "", WithClient(server.Client()))
	if err := conn.Send(context.Background(), []byte{}); err != nil {
		t.Errorf("Send failed: %s", err)
	}
	conn.Close()
	if err := conn.Send(context.Background(), []byte{}); err != protocol.ErrNotConnected {
		t.Errorf("Expected ErrNotConnected but got %s", err)
	}
}

// TestWithClient checks that WithClient replaces the client the Connection
// sends with, and that the default is left alone when no option is passed.
func TestWithClient(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"response": ""}`))
	}))
	defer server.Close()
	domain, _ := strings.CutPrefix(server.URL, "https://")

	// The default client does not trust the test server's self-signed
	// certificate, so this request has to fail.
	plain := NewConnection("VIN123", "", domain, "")
	defer plain.Close()
	if _, err := plain.SendFleetAPICommand(context.Background(), "api/1/test", nil); err == nil {
		t.Error("expected the default client to reject the test server's certificate")
	}

	conn := NewConnection("VIN123", "", domain, "", WithClient(server.Client()))
	defer conn.Close()
	if _, err := conn.SendFleetAPICommand(context.Background(), "api/1/test", nil); err != nil {
		t.Errorf("SendFleetAPICommand through injected client failed: %s", err)
	}

	// A nil client must not clear the default.
	if NewConnection("VIN123", "", domain, "", WithClient(nil)).client == nil {
		t.Error("WithClient(nil) left the Connection without a client")
	}
}

// outOfRegionBody is the JSON error the Fleet API returns with HTTP 421. The
// region that should have been used is named in the body, not in a header.
func outOfRegionBody(host string) string {
	return `{"response":null,"error":"user out of region, use base URL: https://` + host +
		`, see https://developer.tesla.com/docs/fleet-api#regional-requirements","error_description":""}`
}

// TestRegionHandler checks that a Connection reports an out-of-region redirect
// to whoever created it, as well as following it itself.
func TestRegionHandler(t *testing.T) {
	const correctHost = "fleet-api.prd.na.vn.cloud.tesla.com"

	for _, test := range []struct {
		name     string
		named    string
		wantHost string
	}{
		{"valid tesla domain", correctHost, correctHost},
		{"domain outside tesla", "fleet-api.attacker.example.com", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusMisdirectedRequest)
				w.Write([]byte(outOfRegionBody(test.named)))
			}))
			defer server.Close()
			domain, _ := strings.CutPrefix(server.URL, "https://")

			var reported string
			conn := NewConnection("VIN123", "", domain, "",
				WithClient(server.Client()),
				WithRegionHandler(func(host string) { reported = host }))
			defer conn.Close()

			// The error is expected; the redirect is the point.
			_, _ = conn.SendFleetAPICommand(context.Background(), "api/1/test", nil)

			if reported != test.wantHost {
				t.Errorf("handler was told %q, want %q", reported, test.wantHost)
			}
			// The Connection's own redirect and the report must agree.
			if test.wantHost != "" && conn.serverURL != test.wantHost {
				t.Errorf("connection redirected to %q, want %q", conn.serverURL, test.wantHost)
			}
			if test.wantHost == "" && conn.serverURL != domain {
				t.Errorf("connection redirected to %q on an untrusted domain, want it unchanged", conn.serverURL)
			}
		})
	}
}
