/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Tent mode and suspension

POST /api/1/vehicles/{vin}/command/set_tent_mode with {"on": true} enables
Cybertruck tent mode (VehicleAction field 94). The vehicle typically refuses
unless it is in Park.

POST set_suspension_level with {"suspension_level": "medium"} (string names
or 1–6) sets air-suspension ride height (VehicleAction field 118). "level"
is an alias for medium. POST level_suspension with an empty body is the same
as medium. Prefer set_tent_mode when the bed should auto-level each corner.

See teslamotors/vehicle-command#424.

# WiFi configuration

POST wifi_on, wifi_off, set_wifi, add_wifi_network, forget_wifi_network, or
wifi_connect_in_drive returns HTTP 400 with
[protocol.ErrWiFiNotInProtocol] before opening a vehicle session. Tesla has
not published signed VehicleAction fields for those in-car settings; this
proxy will not invent them or forward a PSK. See
teslamotors/vehicle-command#419. WiFi/cellular *state* streaming is
teslamotors/fleet-telemetry#407.

# Keeping infotainment awake

POST keep_awake or keep_alive returns HTTP 400 with
[protocol.ErrKeepAwakeNotInProtocol] before opening a vehicle session.
wake starts infotainment but does not inhibit sleep.
keep_accessory_power_mode powers the 12V jack and charging USB ports, not
the glovebox dashcam/data USB. Tesla has not published a keep-alive
VehicleAction. See teslamotors/vehicle-command#397.

# Battery option codes

POST battery_size, get_battery_option, or get_battery_size returns HTTP 400
with [protocol.ErrBatteryOptionRequiresFleetAPI] before opening a vehicle
session. Pack identity is Tesla catalog metadata
(GET /api/1/dx/vehicles/options), not a signed command. Tesla often omits
$BT* codes; this proxy will not invent them. See
teslamotors/vehicle-command#391.

# BLE key presence / Walk-Away Door Lock

POST ble_presence_exempt or command_only_key returns HTTP 400 with
[protocol.ErrBLEKeyPresenceNotInProtocol] before opening a vehicle session.
add-key-request enrolls a VCSEC whitelist key. Tesla has not published a
flag that lets an in-car BLE client authorize commands while being ignored
for Walk-Away Door Lock. Disconnect after each command. See
teslamotors/vehicle-command#480.

# Scheduled charging / cabin overheat

POST scheduled_charging_overheat or force_scheduled_charging returns HTTP
400 with [protocol.ErrScheduledChargingFirmware] before opening a vehicle
session. set_scheduled_charging and set_scheduled_departure still deliver
the published schedule commands. This proxy will not disable cabin
overheat protection as a workaround for Intel-MCU Model S firmware that
does not sleep/wake the scheduler. See teslamotors/vehicle-command#342.

# Remote boombox

POST remote_boombox returns HTTP 400 with
[protocol.ErrBoomboxNotInProtocol] before opening a vehicle session.
Fleet API still lists the path; Tesla has not published a VehicleAction
for the external speaker pending legal review of Pedestrian Warning
System restrictions (teslamotors/vehicle-command#266). honk_horn and
flash_lights are published. This proxy will not invent a field number,
copy a third-party firmware dump, or map boombox onto honk. See
teslamotors/vehicle-command#266 and #411.

# HVAC Auto vs climate power

POST auto_conditioning_start / auto_conditioning_stop map to
Vehicle.SetClimatePower / ClimateOff (HvacAutoAction.power_on).
That is climate power, not Auto vs Manual HVAC. Optional
manual_override on auto_conditioning_start is the published low-SOC
override bit. POST hvac_auto_mode, set_hvac_auto, climate_manual,
hvac_manual, or auto_hvac_mode returns HTTP 400 with
[protocol.ErrHvacAutoModeNotInProtocol] before opening a vehicle
session. Tesla has not published a VehicleAction for Auto vs Manual
HVAC. set_temps requires driver_temp and/or passenger_temp
(ChangeClimateTemp); an empty body is not encoded as 0 °C (LO). See
teslamotors/vehicle-command#283.

# Request cancellation

Per-request timeouts in forwardRequest and handleVehicleCommand are derived
from req.Context() so a client disconnect cancels pending Fleet API I/O and
vehicle lookup. Dispatcher.Start stops its listener if that context is
canceled before Disconnect would run. This does not recall a command that
has already been delivered to the vehicle. See teslamotors/vehicle-command#491.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
