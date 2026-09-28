/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Battery option codes

POST battery_size, get_battery_option, or get_battery_size returns HTTP 400
with [protocol.ErrBatteryOptionRequiresFleetAPI] before opening a vehicle
session. Pack identity is Tesla catalog metadata
(GET /api/1/dx/vehicles/options), not a signed command. Tesla often omits
$BT* codes; this proxy will not invent them. See
teslamotors/vehicle-command#391.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
