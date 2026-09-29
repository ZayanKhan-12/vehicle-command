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
		{"hvac_auto_mode", nil, nil, protocol.ErrHvacAutoModeNotInProtocol},
		{"set_hvac_auto", proxy.RequestParameters{"on": true}, nil, protocol.ErrHvacAutoModeNotInProtocol},
		{"climate_manual", nil, nil, protocol.ErrHvacAutoModeNotInProtocol},
		{"hvac_manual", nil, nil, protocol.ErrHvacAutoModeNotInProtocol},
		{"auto_hvac_mode", nil, nil, protocol.ErrHvacAutoModeNotInProtocol},
		{"set_temps", nil, nil, &protocol.NominalError{Details: fmt.Errorf("missing driver_temp param")}},
		{"set_temps", proxy.RequestParameters{"driver_temp": 22.0}, nil, nil},
		{"set_temps", proxy.RequestParameters{"passenger_temp": 21.0}, nil, nil},
		{"set_temps", proxy.RequestParameters{"driver_temp": 22.0, "passenger_temp": 21.0}, nil, nil},
		{"auto_conditioning_start", nil, nil, nil},
		{"auto_conditioning_start", proxy.RequestParameters{"manual_override": true}, nil, nil},
		{"auto_conditioning_stop", nil, nil, nil},
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
