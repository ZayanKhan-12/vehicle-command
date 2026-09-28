/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

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
