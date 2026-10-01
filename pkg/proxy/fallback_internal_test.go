package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRESTFallbackDoesNotWriteAResponse pins down what must happen when
// ExtractCommandAction reports that a command has to go to the REST API.
//
// ServeHTTP responds to that by calling forwardRequest, so the request has to
// arrive there intact: nothing may have been written to the ResponseWriter yet,
// and the body must still be readable. Both were broken -- a 400 was written
// first, and the body had been consumed while parsing parameters -- which is
// what issue #136 reports as a response that arrives empty with nothing
// happening on the vehicle.
func TestRESTFallbackDoesNotWriteAResponse(t *testing.T) {
	const command = "remote_steering_wheel_heat_level_request"
	const body = `{"level":2}`

	p := &Proxy{Timeout: DefaultTimeout}
	req := httptest.NewRequest(http.MethodPost, "/api/1/vehicles/VIN/command/"+command, strings.NewReader(body))
	recorder := httptest.NewRecorder()

	// A nil account is safe here: a REST-only command returns before the
	// account is used to fetch a vehicle.
	_, _, err := p.loadVehicleAndCommandFromRequest(context.Background(), nil, recorder, req, command, "VIN")

	if !errors.Is(err, ErrCommandUseRESTAPI) {
		t.Fatalf("got error %v, want ErrCommandUseRESTAPI", err)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("a response was written before forwarding: %d %s", recorder.Code, recorder.Body)
	}

	remaining, readErr := io.ReadAll(req.Body)
	if readErr != nil {
		t.Fatalf("reading the request body failed: %s", readErr)
	}
	if string(remaining) != body {
		t.Errorf("body left for forwarding is %q, want %q", remaining, body)
	}
}

// TestExtractCommandActionLeavesBodyReadable covers the other route to
// forwardRequest: a command that is implemented, parsed and dispatched
// normally, on a vehicle that then turns out not to support the command
// protocol. handleVehicleCommand forwards the original request in that case, so
// parsing the parameters must not consume the body it forwards.
func TestExtractCommandActionLeavesBodyReadable(t *testing.T) {
	const body = `{"volume":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/1/vehicles/VIN/command/adjust_volume", strings.NewReader(body))

	action, err := extractCommandAction(context.Background(), req, "adjust_volume")
	if err != nil {
		t.Fatalf("parsing adjust_volume failed: %s", err)
	}
	if action == nil {
		t.Fatal("got no action for a supported command")
	}

	remaining, readErr := io.ReadAll(req.Body)
	if readErr != nil {
		t.Fatalf("reading the request body failed: %s", readErr)
	}
	if string(remaining) != body {
		t.Errorf("body left for forwarding is %q, want %q", remaining, body)
	}
}
