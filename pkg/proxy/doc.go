/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Charging Manager vs charge port

POST charge_port_door_open / charge_port_door_close still send
ChargePortDoorOpen/Close. Role checks are firmware. Charging Manager
keys can charging-start/stop/set-amps; charge-port may return
INSUFFICIENT_PRIVILEGES until Tesla expands that ACL. POST
charging_manager_charge_port, grant_charging_manager_charge_port, or
charging_manager_port returns HTTP 400 with
[protocol.ErrChargingManagerChargePortFirmware] before opening a
vehicle session. This proxy will not enroll Owner or rewrite charge-port
as a VCSEC closure. See teslamotors/vehicle-command#413.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
