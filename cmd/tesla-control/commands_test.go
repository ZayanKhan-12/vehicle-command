package main

import (
	"errors"
	"strconv"
	"testing"

	"github.com/teslamotors/vehicle-command/pkg/protocol"
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

func TestParseHomelinkDevice(t *testing.T) {
	t.Parallel()
	if got := parseHomelinkDevice(""); got.Index != nil || got.Name != "" {
		t.Errorf("empty selector = %+v, want zero value", got)
	}

	byIndex := parseHomelinkDevice("2")
	if byIndex.Index == nil || *byIndex.Index != 2 {
		t.Errorf("index selector = %+v, want Index=2", byIndex)
	}
	if byIndex.Name != "" {
		t.Errorf("index selector set Name %q", byIndex.Name)
	}

	zero := parseHomelinkDevice("0")
	if zero.Index == nil || *zero.Index != 0 {
		t.Errorf("index 0 = %+v, want explicit 0", zero)
	}

	byName := parseHomelinkDevice("Garage Right")
	if byName.Index != nil || byName.Name != "Garage Right" {
		t.Errorf("name selector = %+v, want Name=Garage Right", byName)
	}
}

func TestRenameKeyRequiresFleetAPI(t *testing.T) {
	t.Parallel()
	info, ok := commands["rename-key"]
	if !ok {
		t.Fatal("missing rename-key")
	}
	if !info.requiresFleetAPI {
		t.Error("rename-key must require Fleet API; Locks-screen names are not stored on the vehicle")
	}
	if info.requiresAuth {
		t.Error("rename-key talks to Tesla's account service, not a signed VCSEC session")
	}
}

func TestUpdateKeyWorksWithoutFleetAPI(t *testing.T) {
	t.Parallel()
	info, ok := commands["update-key"]
	if !ok {
		t.Fatal("missing update-key")
	}
	if info.requiresFleetAPI {
		t.Error("update-key must work over BLE")
	}
	if !info.requiresAuth {
		t.Error("update-key must be a signed whitelist operation")
	}
}

func TestFleetCommandBlockedByBLE(t *testing.T) {
	t.Parallel()
	if !errors.Is(fleetCommandBlockedByBLE("rename-key"), protocol.ErrKeyNameRequiresFleetAPI) {
		t.Fatal("rename-key over BLE must return ErrKeyNameRequiresFleetAPI")
	}
	if !errors.Is(fleetCommandBlockedByBLE("get"), ErrRequiresOAuth) {
		t.Fatal("other Fleet commands over BLE still need a generic OAuth error")
	}
}
