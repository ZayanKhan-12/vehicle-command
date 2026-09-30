/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

POST set_climate_keeper_mode still sends HvacClimateKeeperAction
(0=off, 1=on, 2=Dog, 3=Camp). Firmware may refuse with HTTP 200
result:false and reason cpd_enabled (Child Presence Detection
occupancy). That is not the Child Left Alone Detection setting.
manual_override is a low-SOC override, not a CPD bypass
(teslamotors/vehicle-command#437). POST climate_keeper_cpd,
override_cpd, dog_mode_cpd, or camp_mode_cpd returns HTTP 400 with
[protocol.ErrClimateKeeperCPDFirmware] before opening a vehicle
session. This proxy will not invent a CPD-disable VehicleAction.
See teslamotors/vehicle-command#509.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
