package proxy_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/teslamotors/vehicle-command/pkg/connector/inet"
	"github.com/teslamotors/vehicle-command/pkg/proxy"
)

// fleetAPIVehicleCommands is every command documented under
// /api/1/vehicles/{id}/command/ at
// https://developer.tesla.com/docs/fleet-api/endpoints/vehicle-commands,
// transcribed 2026-09-18.
//
// Issue #188 tracked the gap between this list and the proxy by hand, in a
// GitHub comment, for about two years. This test is the automated version: if
// Tesla publishes a new command, add it here and the test says whether the
// proxy needs to grow a case for it.
var fleetAPIVehicleCommands = []string{
	"actuate_trunk",
	"add_charge_schedule",
	"add_precondition_schedule",
	"adjust_volume",
	"auto_conditioning_start",
	"auto_conditioning_stop",
	"cancel_software_update",
	"charge_max_range",
	"charge_port_door_close",
	"charge_port_door_open",
	"charge_standard",
	"charge_start",
	"charge_stop",
	"clear_pin_to_drive_admin",
	"door_lock",
	"door_unlock",
	"erase_user_data",
	"flash_lights",
	"guest_mode",
	"honk_horn",
	"media_next_fav",
	"media_next_track",
	"media_prev_fav",
	"media_prev_track",
	"media_toggle_playback",
	"media_volume_down",
	"media_volume_up",
	"navigation_gps_request",
	"navigation_request",
	"navigation_sc_request",
	"navigation_waypoints_request",
	"parental_controls_activate",
	"parental_controls_clear_pin_admin",
	"parental_controls_deactivate",
	"parental_controls_enable_setting",
	"parental_controls_set_speed_limit",
	"remote_auto_seat_climate_request",
	"remote_auto_steering_wheel_heat_climate_request",
	"remote_boombox",
	"remote_seat_cooler_request",
	"remote_seat_heater_request",
	"remote_start_drive",
	"remote_steering_wheel_heat_level_request",
	"remote_steering_wheel_heater_request",
	"remove_charge_schedule",
	"remove_precondition_schedule",
	"reset_pin_to_drive_pin",
	"reset_valet_pin",
	"schedule_software_update",
	"set_bioweapon_mode",
	"set_cabin_overheat_protection",
	"set_charge_limit",
	"set_charging_amps",
	"set_climate_keeper_mode",
	"set_cop_temp",
	"set_pin_to_drive",
	"set_preconditioning_max",
	"set_scheduled_charging",
	"set_scheduled_departure",
	"set_sentry_mode",
	"set_temps",
	"set_valet_mode",
	"set_vehicle_name",
	"speed_limit_activate",
	"speed_limit_clear_pin",
	"speed_limit_clear_pin_admin",
	"speed_limit_deactivate",
	"speed_limit_set_limit",
	"sun_roof_control",
	"trigger_homelink",
	"upcoming_calendar_entries",
	"window_control",
}

// unrecognized reports whether err is the answer ExtractCommandAction gives for
// a command it has never heard of, as opposed to one it knows but cannot build
// from the parameters supplied.
func unrecognized(err error) bool {
	var httpErr *inet.HTTPError
	return errors.As(err, &httpErr) &&
		httpErr.Code == http.StatusBadRequest &&
		strings.Contains(httpErr.Message, "invalid_command")
}

// TestFleetAPICommandCoverage checks that the proxy recognises every documented
// Fleet API vehicle command.
//
// Recognising a command means one of two things, and the test accepts either:
// the proxy builds an action for it, or it reports ErrCommandUseRESTAPI so that
// ServeHTTP forwards the request. What it must not do is answer HTTP 400
// invalid_command, because that blocks the request outright -- the proxy is the
// only route to Tesla for its callers, so an unrecognised command is not merely
// unimplemented, it is unreachable.
//
// Missing parameters are not a failure here. This test is about the command
// name being known; command_test.go covers parameter handling.
func TestFleetAPICommandCoverage(t *testing.T) {
	ctx := context.Background()
	for _, command := range fleetAPIVehicleCommands {
		t.Run(command, func(t *testing.T) {
			_, err := proxy.ExtractCommandAction(ctx, command, proxy.RequestParameters{})
			if unrecognized(err) {
				t.Errorf("the proxy answers HTTP 400 invalid_command for %s, so callers cannot reach it", command)
			}
		})
	}
}

// TestUnrecognizedDetectsAnUnknownCommand guards the helper above: if the
// wording of the 400 ever changes, TestFleetAPICommandCoverage would start
// passing for every command, including ones the proxy does not know.
func TestUnrecognizedDetectsAnUnknownCommand(t *testing.T) {
	_, err := proxy.ExtractCommandAction(context.Background(), "definitely_not_a_command", proxy.RequestParameters{})
	if !unrecognized(err) {
		t.Errorf("got error %v; the coverage test cannot detect an unknown command", err)
	}
}
