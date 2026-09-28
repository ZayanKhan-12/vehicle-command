/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Error responses

The proxy does not maintain a closed enum of every string a vehicle may return.
Infotainment refusals are firmware-defined plaintext in
CarServer.ActionStatus.result_reason. New reasons appear without a protocol
change. Clients should match on error class first, then on the reason string.

Success (HTTP 200):

	{"response":{"result":true,"reason":""}}

Vehicle refused the command after authenticating it (HTTP 200, [protocol.NominalError]).
The reason is "car could not execute command: " plus the firmware string. Example
from teslamotors/vehicle-command#425:

	{"response":{"result":false,"reason":"car could not execute command: low_power_mode_low_soc"}}

Library helpers: [protocol.IsNominalError], [protocol.CommandRefusalReason].
Reasons mentioned in this repository include low_power_mode_low_soc (climate
while the vehicle is in low-power mode due to a low battery) and
low_power_mode_enforced (SetLowPowerMode while the vehicle will not leave
low-power mode). Treat any other suffix as a new firmware reason, not a proxy
bug.

Proxy request problems (typically HTTP 200 with a NominalError reason, or HTTP
400 for an unknown command): missing <param> param, invalid <param> param,
invalid_command.

Fleet API / transport errors (HTTP status from Tesla or a mapped library
error): HTTP 403/408/412 and other [inet.HTTPError] bodies passed through
unchanged; vehicle asleep maps to 408; unpaired keys map to 412.
Protocol-layer faults are [protocol.RoutableMessageError] with a MessageFault_E
code; see universal_message.proto.

# Forcing climate at low SOC

POST /api/1/vehicles/{vin}/command/auto_conditioning_start with an empty body
sends HvacAutoAction with manual_override unset, which vehicles may refuse with
low_power_mode_low_soc. The Tesla app's confirmation sets that proto field.
Pass {"manual_override": true} on the same endpoint (or tesla-control
climate-on force, or Vehicle.SetClimate with manualOverride true).

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
