/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

POST honk_horn and other published Infotainment commands are still sent.
GET /api/1/vehicles state=online and a successful VCSEC signed_command do
not prove DOMAIN_INFOTAINMENT will be accepted. Tesla's gateway can return
HTTP 408 vehicle is offline for that domain (inet.ErrVehicleNotAwake).
POST infotainment_offline, domain_infotainment_online, or
vcsec_means_infotainment_online returns HTTP 400 with
[protocol.ErrInfotainmentSignedCommandOffline] before opening a vehicle
session. This proxy will not rewrite the domain to VCSEC or skip the
Infotainment handshake. See teslamotors/vehicle-command#285.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
