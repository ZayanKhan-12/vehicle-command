package main

import (
	"context"
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

func TestVirtualKeyReturnCommandRejectsRedirect(t *testing.T) {
	t.Parallel()
	info, ok := commands["virtual-key-return"]
	if !ok {
		t.Fatal("missing virtual-key-return")
	}
	if info.requiresFleetAPI || info.requiresAuth {
		t.Error("virtual-key-return must not require a session")
	}
	err := info.handler(context.Background(), nil, nil, map[string]string{
		"DOMAIN":     "example.com",
		"RETURN_URI": "https://evil.test/phish",
	})
	if !errors.Is(err, protocol.ErrVirtualKeyReturnURI) {
		t.Fatalf("virtual-key-return = %v, want ErrVirtualKeyReturnURI", err)
	}
	err = info.handler(context.Background(), nil, nil, map[string]string{
		"DOMAIN":     "example.com",
		"RETURN_URI": "https://example.com/finish-setup",
	})
	if !errors.Is(err, protocol.ErrVirtualKeyReturnURI) {
		t.Fatalf("same-domain return_uri = %v, want ErrVirtualKeyReturnURI", err)
	}
}

func TestVirtualKeyLinkCommandPrintsHostedURL(t *testing.T) {
	t.Parallel()
	info, ok := commands["virtual-key-link"]
	if !ok {
		t.Fatal("missing virtual-key-link")
	}
	if err := info.handler(context.Background(), nil, nil, map[string]string{"DOMAIN": "example.com"}); err != nil {
		t.Fatalf("virtual-key-link: %v", err)
	}
	if err := info.handler(context.Background(), nil, nil, map[string]string{"DOMAIN": "https://evil.test"}); !errors.Is(err, protocol.ErrVirtualKeyReturnURI) {
		t.Fatalf("virtual-key-link bad domain = %v", err)
	}
}
