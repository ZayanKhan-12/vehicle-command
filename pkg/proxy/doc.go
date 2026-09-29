/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Scheduled charging / cabin overheat

POST scheduled_charging_overheat or force_scheduled_charging returns HTTP
400 with [protocol.ErrScheduledChargingFirmware] before opening a vehicle
session. set_scheduled_charging and set_scheduled_departure still deliver
the published schedule commands. This proxy will not disable cabin
overheat protection as a workaround for Intel-MCU Model S firmware that
does not sleep/wake the scheduler. See teslamotors/vehicle-command#342.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
