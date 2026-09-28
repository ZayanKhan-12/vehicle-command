package inet

import (
	"context"
	"errors"
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
	conn := NewConnection("VIN123", "", domain, "")
	conn.client = server.Client()
	if err := conn.Send(context.Background(), []byte{}); err != nil {
		t.Errorf("Send failed: %s", err)
	}
	conn.Close()
	if err := conn.Send(context.Background(), []byte{}); err != protocol.ErrNotConnected {
		t.Errorf("Expected ErrNotConnected but got %s", err)
	}
}

func TestParseAccountDisabled(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantNil    bool
		wantReason string
	}{
		{
			name:       "issue 403 payload",
			body:       `{"error":"account disabled: EXCEEDED_LIMIT"}`,
			wantReason: "EXCEEDED_LIMIT",
		},
		{
			name:       "bare string",
			body:       "account disabled: EXCEEDED_LIMIT",
			wantReason: "EXCEEDED_LIMIT",
		},
		{
			name:       "no reason suffix",
			body:       `{"error":"account disabled"}`,
			wantReason: "",
		},
		{
			name:    "unrelated 403",
			body:    `{"error":"unauthorized"}`,
			wantNil: true,
		},
		{
			name:    "empty",
			body:    "",
			wantNil: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseAccountDisabled([]byte(test.body))
			if test.wantNil {
				if got != nil {
					t.Fatalf("parseAccountDisabled(%q) = %+v, want nil", test.body, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseAccountDisabled(%q) = nil, want reason %q", test.body, test.wantReason)
			}
			if got.Reason != test.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, test.wantReason)
			}
			if !errors.Is(got, ErrAccountDisabled) {
				t.Error("errors.Is(..., ErrAccountDisabled) = false")
			}
			if protocol.ShouldRetry(got) || protocol.Temporary(got) || protocol.MayHaveSucceeded(got) {
				t.Error("account disabled must not be retried or treated as possibly succeeded")
			}
			if !strings.Contains(got.Error(), "billing hold") {
				t.Errorf("Error() = %q, want billing-hold guidance", got.Error())
			}
		})
	}
}

func TestSendFleetAPICommandAccountDisabled(t *testing.T) {
	const teslaBody = `{"error":"account disabled: EXCEEDED_LIMIT"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(teslaBody))
	}))
	t.Cleanup(server.Close)

	_, err := SendFleetAPICommand(context.Background(), server.Client(), "ua", "Bearer x", server.URL, map[string]string{})
	if err == nil {
		t.Fatal("expected error")
	}
	if protocol.ShouldRetry(err) {
		t.Error("ShouldRetry() = true; retrying a disabled account cannot succeed")
	}
	if !errors.Is(err, ErrAccountDisabled) {
		t.Fatalf("errors.Is = false for %v", err)
	}
	var acc *AccountDisabledError
	if !errors.As(err, &acc) || acc.Reason != "EXCEEDED_LIMIT" {
		t.Errorf("AccountDisabledError = %+v, want EXCEEDED_LIMIT", acc)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusForbidden {
		t.Errorf("HTTPError = %+v, want 403", httpErr)
	}
	if httpErr != nil && httpErr.Message != teslaBody {
		t.Errorf("HTTPError.Message = %q, want Tesla payload", httpErr.Message)
	}
}

func TestSendFleetAPICommandUnrelatedForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"mobile access disabled"}`))
	}))
	t.Cleanup(server.Close)

	_, err := SendFleetAPICommand(context.Background(), server.Client(), "ua", "Bearer x", server.URL, map[string]string{})
	if errors.Is(err, ErrAccountDisabled) {
		t.Fatal("unrelated 403 classified as account disabled")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusForbidden {
		t.Fatalf("HTTPError = %+v, want 403", httpErr)
	}
}
