/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Seat heater and cooler

POST remote_seat_heater_request and remote_seat_cooler_request already
map to published HvacSeatHeaterActions (field 36) and
HvacSeatCoolerActions (field 49). The heater body accepts Owner/Fleet
JSON "heater" as an alias for "seat_position" (0–8) plus "level"
(0–3). Cooler uses "seat_position" plus "seat_cooler_level" ("level"
is an alias).

HTTP 501 with JSON error Unauthorized from Tesla signed_command is
Fleet API partner/region/OAuth allowlist. writeJSONError forwards
Tesla's status, so clients see "Not Implemented" even though this
proxy already signed and POSTed the VehicleAction. That is not
[ErrCommandNotImplemented]. POST seat_heater_not_implemented,
seat_cooler_not_implemented, or remote_seat_climate_not_implemented
returns HTTP 400 with [protocol.ErrSeatClimateFleetAPI] before opening
a vehicle session. See teslamotors/vehicle-command#383.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
