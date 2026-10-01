# CLAUDE.md

Guidance for AI coding assistants working in this repository.

## What this repository is

`github.com/teslamotors/vehicle-command` is Tesla's Go SDK for the
end-to-end authenticated vehicle command protocol. `pkg/vehicle` is the
command API. `pkg/connector/inet` posts signed protobufs to Fleet API
`signed_command`. `pkg/connector/ble` is a separate transport. The domain
(`DOMAIN_INFOTAINMENT` or `DOMAIN_VEHICLE_SECURITY`) is a field inside the
`RoutableMessage`, not a separate HTTP path.

## Infotainment signed_command while the vehicle list says online

`GET /api/1/vehicles` `state=online` and a successful VCSEC `signed_command`
do not mean `DOMAIN_INFOTAINMENT` will accept `session_info_request`.
Tesla's gateway can return HTTP 408 `vehicle is offline`
(`inet.ErrVehicleNotAwake`) for Infotainment while the list says online,
VCSEC works, BLE reaches both domains, and the operator is in the car
(reported on `fleet-api.prd.cn`). Do not rewrite the domain to VCSEC or
skip the Infotainment handshake. `wake_up` is unsigned REST and is not a
guarantee. Return `protocol.ErrInfotainmentSignedCommandOffline` for
clients that ask this SDK to treat those signals as Infotainment
reachability. Published Infotainment commands are still sent. See
teslamotors/vehicle-command#285.

## Things an assistant should not do here

* Do not invent unpublished VehicleAction field numbers or change wire formats.
* Do not edit generated `*.pb.go` files by hand.
* Do not send commands to a real vehicle unless the user explicitly asked.
* Do not commit private keys, OAuth tokens, or VINs.
