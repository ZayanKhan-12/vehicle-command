/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api

POST set_charging_amps still sends the requested charging_amps integer on
both transports, including values below 5. Firmware may show a 5A floor,
and a wall meter can read about 1A higher while the car is awake. POST
charging_amps_floor, ble_charging_amps, or match_wall_amps returns HTTP 400
with [protocol.ErrChargingAmpsBelowFloorFirmware] before opening a vehicle
session. This proxy will not clamp the value, add an amp on BLE, or send
the command twice. See teslamotors/vehicle-command#256.
*/
package proxy
