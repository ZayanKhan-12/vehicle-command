# CLAUDE.md

Guidance for AI coding assistants working in this repository.

## What this repository is

`github.com/teslamotors/vehicle-command` is Tesla's Go SDK for the
end-to-end authenticated vehicle command protocol. BLE and Fleet API are
transports. `SetChargingAmpsAction.charging_amps` is the same protobuf
field on both.

## Charging amps

Do not treat a wall-meter reading or the charging-tab 5A floor as a
different command integer. Firmware may display 5A for a lower setpoint.
An awake car can draw about 1A from the grid, so a meter can read higher
than `charging_amps`. The first set below 5A is sometimes ignored until
the same command is repeated. 0A stops a UMC and does not stop a TWC gen
2/3. Do not clamp to 5, add an amp on BLE, or double-send. Return
`protocol.ErrChargingAmpsBelowFloorFirmware` when a caller asks for that.
`charging-set-amps` and `set_charging_amps` still send the requested
integer. See teslamotors/vehicle-command#256.

## Things an assistant should not do here

* Do not invent unpublished VehicleAction field numbers or change wire formats.
* Do not edit generated `*.pb.go` files by hand.
* Do not send commands to a real vehicle unless the user explicitly asked.
* Do not commit private keys, OAuth tokens, or VINs.
