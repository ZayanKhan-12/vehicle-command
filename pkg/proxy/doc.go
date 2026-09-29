/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

POST charge_stop and set_charging_amps still send the published
Infotainment VehicleActions. Fleet Telemetry can report ACChargingPower
and Soc while Tesla signed_command returns vehicle unavailable (offline
or asleep); inet.ErrVehicleNotAwake maps to HTTP 408. wake starts
infotainment but does not inhibit sleep. POST charging_while_asleep,
charge_stop_asleep, or set_charging_amps_asleep returns HTTP 400 with
[protocol.ErrChargingWhileInfotainmentAsleep] before opening a vehicle
session. This proxy will not invent keep-awake or a charging-controller
bypass. See teslamotors/vehicle-command#452.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
