/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# HVAC Auto vs climate power

POST auto_conditioning_start / auto_conditioning_stop map to
Vehicle.SetClimatePower / ClimateOff (HvacAutoAction.power_on).
That is climate power, not Auto vs Manual HVAC. Optional
manual_override on auto_conditioning_start is the published low-SOC
override bit. POST hvac_auto_mode, set_hvac_auto, climate_manual,
hvac_manual, or auto_hvac_mode returns HTTP 400 with
[protocol.ErrHvacAutoModeNotInProtocol] before opening a vehicle
session. Tesla has not published a VehicleAction for Auto vs Manual
HVAC. set_temps requires driver_temp and/or passenger_temp
(ChangeClimateTemp); an empty body is not encoded as 0 °C (LO). See
teslamotors/vehicle-command#283.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
