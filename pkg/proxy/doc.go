/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# BLE key presence / Walk-Away Door Lock

POST ble_presence_exempt or command_only_key returns HTTP 400 with
[protocol.ErrBLEKeyPresenceNotInProtocol] before opening a vehicle session.
add-key-request enrolls a VCSEC whitelist key. Tesla has not published a
flag that lets an in-car BLE client authorize commands while being ignored
for Walk-Away Door Lock. Disconnect after each command. See
teslamotors/vehicle-command#480.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
