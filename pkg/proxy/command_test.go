package proxy_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/teslamotors/vehicle-command/pkg/connector/inet"
	"github.com/teslamotors/vehicle-command/pkg/protocol"
	"github.com/teslamotors/vehicle-command/pkg/proxy"
	"github.com/teslamotors/vehicle-command/pkg/vehicle"
)

func TestExtractCommandAction(t *testing.T) {
	ctx := context.Background()
	params := proxy.RequestParameters{
		"volume":        5.0,
		"on":            true,
		"seat_position": 0,
		"level":         2.0,
		// Add more test cases for different commands and parameters
	}

	tests := []struct {
		command      string
		params       proxy.RequestParameters
		expectedFunc func(*vehicle.Vehicle) error
		expected     error
	}{
		{"adjust_volume", params, func(v *vehicle.Vehicle) error { return v.SetVolume(ctx, 0.0) }, nil},
		{"adjust_volume", nil, nil, &protocol.NominalError{Details: fmt.Errorf("missing volume param")}},
		{"remote_boombox", params, nil, proxy.ErrCommandNotImplemented},
		{"invalid_command", params, nil, &inet.HTTPError{Code: http.StatusBadRequest, Message: "{\"response\":null,\"error\":\"invalid_command\",\"error_description\":\"\"}"}},
	}

	for _, test := range tests {
		action, err := proxy.ExtractCommandAction(ctx, test.command, test.params)

		if errors.Is(err, test.expected) {
			if test.expected != nil && action != nil {

				t.Errorf("Expected error %#v but got action %p for command %#v", test.expected, action, test.command)
			}
		} else if err != nil && err.Error() != test.expected.Error() {
			t.Errorf("Unexpected error for command %s: %v", test.command, err)
		}
	}
}

// restOnlyCommands are the Fleet API vehicle commands that have no
// representation in the vehicle command protocol, and so must be forwarded to
// the REST API rather than signed and sent to the vehicle.
var restOnlyCommands = []string{
	// Server-side processing: the vehicle is not the only participant.
	"navigation_request",
	"navigation_gps_request",
	"navigation_sc_request",
	"navigation_waypoints_request",
	"upcoming_calendar_entries",
	// HvacSteeringWheelHeaterAction carries only a power_on boolean.
	"remote_steering_wheel_heat_level_request",
	"remote_auto_steering_wheel_heat_climate_request",
	// Managed charging is a server-side feature.
	"set_managed_charge_current_request",
	"set_managed_charger_location",
	"set_managed_scheduled_charging_time",
}

// TestRESTOnlyCommandsAreForwarded checks that each of those commands asks the
// proxy to forward it. The distinction matters: ErrCommandUseRESTAPI makes
// Proxy.ServeHTTP call forwardRequest, whereas an unrecognised command is
// answered with HTTP 400 invalid_command and never reaches Tesla at all.
func TestRESTOnlyCommandsAreForwarded(t *testing.T) {
	for _, command := range restOnlyCommands {
		t.Run(command, func(t *testing.T) {
			action, err := proxy.ExtractCommandAction(context.Background(), command, proxy.RequestParameters{})
			if !errors.Is(err, proxy.ErrCommandUseRESTAPI) {
				t.Errorf("got error %v, want ErrCommandUseRESTAPI", err)
			}
			if action != nil {
				t.Error("got an action to run against the vehicle, want none")
			}
		})
	}
}

// TestUnknownCommandIsRejected guards the other side of that distinction, so
// that the fix above cannot be generalised into forwarding anything at all.
func TestUnknownCommandIsRejected(t *testing.T) {
	for _, command := range []string{"", "not_a_command", "navigation"} {
		action, err := proxy.ExtractCommandAction(context.Background(), command, proxy.RequestParameters{})
		var httpErr *inet.HTTPError
		if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
			t.Errorf("command %q: got error %v, want HTTP 400", command, err)
		}
		if action != nil {
			t.Errorf("command %q: got an action to run against the vehicle, want none", command)
		}
	}
}
