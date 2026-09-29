/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Remote boombox

POST remote_boombox returns HTTP 400 with
[protocol.ErrBoomboxNotInProtocol] before opening a vehicle session.
Fleet API still lists the path; Tesla has not published a VehicleAction
for the external speaker. honk_horn and flash_lights are published.
This proxy will not invent a field number or map boombox onto honk. See
teslamotors/vehicle-command#411.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
