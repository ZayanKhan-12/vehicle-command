/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Keeping infotainment awake

POST keep_awake or keep_alive returns HTTP 400 with
[protocol.ErrKeepAwakeNotInProtocol] before opening a vehicle session.
wake starts infotainment but does not inhibit sleep.
keep_accessory_power_mode powers the 12V jack and charging USB ports, not
the glovebox dashcam/data USB. Tesla has not published a keep-alive
VehicleAction. See teslamotors/vehicle-command#397.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
