/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# BLE GetDriveState latency

POST ble_state_fast, drive_state_fast, or set_ble_poll_interval returns
HTTP 400 with [protocol.ErrBLEStateLatencyFirmware] before opening a
vehicle session. GetState is one Infotainment round-trip (~250–300ms
observed). This proxy cannot guarantee <150ms, disable response
encryption, or stream DriveState. Reuse StartSession; fleet-telemetry
is a separate product. See teslamotors/vehicle-command#414.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
