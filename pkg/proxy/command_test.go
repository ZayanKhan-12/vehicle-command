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
		{"set_tent_mode", proxy.RequestParameters{"on": true}, nil, nil},
		{"set_tent_mode", proxy.RequestParameters{"on": false}, nil, nil},
		{"set_tent_mode", nil, nil, &protocol.NominalError{Details: fmt.Errorf("missing on param")}},
		{"set_tent_mode", proxy.RequestParameters{"on": "yes"}, nil, &protocol.NominalError{Details: fmt.Errorf("invalid on param")}},
		{"set_suspension_level", proxy.RequestParameters{"suspension_level": "medium"}, nil, nil},
		{"set_suspension_level", proxy.RequestParameters{"suspension_level": "level"}, nil, nil},
		{"set_suspension_level", proxy.RequestParameters{"suspension_level": 3.0}, nil, nil},
		{"set_suspension_level", proxy.RequestParameters{"suspension_level": "entry"}, nil, nil},
		{"set_suspension_level", nil, nil, &protocol.NominalError{Details: fmt.Errorf("missing suspension_level param")}},
		{"set_suspension_level", proxy.RequestParameters{"suspension_level": "park"}, nil, &protocol.NominalError{Details: fmt.Errorf("invalid suspension_level param")}},
		{"set_suspension_level", proxy.RequestParameters{"suspension_level": 0.0}, nil, &protocol.NominalError{Details: fmt.Errorf("invalid suspension_level param")}},
		{"level_suspension", nil, nil, nil},
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
