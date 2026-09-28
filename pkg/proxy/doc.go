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

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
