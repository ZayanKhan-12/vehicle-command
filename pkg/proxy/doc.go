/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Climate split / SYNC

POST climate_split, set_climate_split, climate_sync, or
set_climate_sync returns HTTP 400 with
[protocol.ErrClimateSplitNotInProtocol] before opening a vehicle
session. Tesla has not published a VehicleAction for the in-car
split/SYNC control. Independent driver and passenger setpoints are
already set_temps (HvacTemperatureAdjustmentAction). GetClimateState
returns the two temp settings, not a split boolean. See
teslamotors/vehicle-command#386.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
