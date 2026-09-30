package main

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/teslamotors/vehicle-command/pkg/protocol"
	"github.com/teslamotors/vehicle-command/pkg/vehicle"
)

func TestMinutesAfterMidnight(t *testing.T) {
	type params struct {
		str     string
		minutes int32
		err     error
	}
	testCases := []params{
		{str: "3:03", minutes: 183},
		{str: "0:00", minutes: 0},
		{str: "", err: ErrInvalidTime},
		{str: "3:", err: ErrInvalidTime},
		{str: ":40", err: ErrInvalidTime},
		{str: "3:40pm", err: ErrInvalidTime},
		{str: "25:40", err: ErrInvalidTime},
		{str: "23:40", minutes: 23*60 + 40},
		{str: "23:60", err: ErrInvalidTime},
		{str: "23:-01", err: ErrInvalidTime},
		{str: "24:00", err: ErrInvalidTime},
		{str: "-2:00", err: ErrInvalidTime},
	}
	for _, test := range testCases {
		minutes, err := MinutesAfterMidnight(test.str)
		if !errors.Is(err, test.err) {
			t.Errorf("expected '%s' to result in error %s, but got %s", test.str, test.err, err)
		} else if test.minutes != minutes {
			t.Errorf("expected MinutesAfterMidnight('%s') = %d, but got %d", test.str, test.minutes, minutes)
		}
	}
}

func TestGetDays(t *testing.T) {
	type params struct {
		str   string
		mask  int32
		isErr bool
	}
	testCases := []params{
		{str: "SUN", mask: 1},
		{str: "SUN, WED", mask: 1 + 8},
		{str: "SUN, WEDnesday", mask: 1 + 8},
		{str: "sUN,wEd", mask: 1 + 8},
		{str: "all", mask: 127},
		{str: "sun,all", mask: 127},
		{str: "mon,tues,wed,thurs", mask: 2 + 4 + 8 + 16},
		{str: "marketday", isErr: true},
		{str: "sun mon", isErr: true},
	}
	for _, test := range testCases {
		mask, err := GetDays(test.str)
		if (err != nil) != test.isErr {
			t.Errorf("day string '%s' gave unexpected err = %s", test.str, err)
		} else if mask != test.mask {
			t.Errorf("day string '%s' gave mask %s instead of %s", test.str, strconv.FormatInt(int64(mask), 2), strconv.FormatInt(int64(test.mask), 2))
		}
	}
}

func TestClimateKeeperCPDCommandReturnsFirmwareError(t *testing.T) {
	t.Parallel()
	info, ok := commands["climate-keeper-cpd"]
	if !ok {
		t.Fatal("missing climate-keeper-cpd")
	}
	if info.requiresFleetAPI || info.requiresAuth {
		t.Error("climate-keeper-cpd help command must not require a session")
	}
	if err := info.handler(context.Background(), nil, nil, nil); !errors.Is(err, protocol.ErrClimateKeeperCPDFirmware) {
		t.Fatalf("climate-keeper-cpd handler = %v, want ErrClimateKeeperCPDFirmware", err)
	}
}

func TestClimateKeeperCommandIsPublished(t *testing.T) {
	t.Parallel()
	info, ok := commands["climate-keeper"]
	if !ok {
		t.Fatal("missing climate-keeper")
	}
	if !info.requiresAuth || info.requiresFleetAPI {
		t.Error("climate-keeper must send a signed Infotainment command")
	}
	if len(info.args) != 1 || info.args[0].name != "MODE" {
		t.Fatal("climate-keeper must take MODE")
	}
}

func TestParseClimateKeeperMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    vehicle.ClimateKeeperMode
		wantErr bool
	}{
		{"dog", vehicle.ClimateKeeperModeDog, false},
		{"2", vehicle.ClimateKeeperModeDog, false},
		{"camp", vehicle.ClimateKeeperModeCamp, false},
		{"3", vehicle.ClimateKeeperModeCamp, false},
		{"off", vehicle.ClimateKeeperModeOff, false},
		{"on", vehicle.ClimateKeeperModeOn, false},
		{"party", 0, true},
	}
	for _, tt := range tests {
		got, err := parseClimateKeeperMode(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseClimateKeeperMode(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseClimateKeeperMode(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseClimateKeeperMode(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
