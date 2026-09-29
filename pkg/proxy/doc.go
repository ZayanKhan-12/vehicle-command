/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Request cancellation

Per-request timeouts in forwardRequest and handleVehicleCommand are derived
from req.Context() so a client disconnect cancels pending Fleet API I/O and
vehicle lookup. Dispatcher.Start stops its listener if that context is
canceled before Disconnect would run. This does not recall a command that
has already been delivered to the vehicle. See teslamotors/vehicle-command#491.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
