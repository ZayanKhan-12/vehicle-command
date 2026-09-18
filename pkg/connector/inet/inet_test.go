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
