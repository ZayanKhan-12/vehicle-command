# Tesla Control Utility

The `tesla-control` application provides a command-line interface for sending
commands to Tesla vehicles.

This application does not run on Windows due to limitations in the available
Golang BLE packages.

## Building

Run `go get` to install Golang dependencies, and `go build` to compile.

You may also run `go install` to place `tesla-control` in your GOBIN directory.

## Key management

Commands are end-to-end authenticated, which means `tesla-control` requires
access to a private key, and the public key must be enrolled on the target
vehicle.

We'll use `tesla-keygen` to generate a key. See the root directory
[README](/README.md) file for instructions on installing this tool.

Export environment variables shared by these tools:

```bash
export TESLA_KEY_NAME=$(whoami)
export TESLA_VIN=<your Tesla's VIN>
```

Generate a private key in your system keyring, and save the public key to a file:

```
tesla-keygen create > public_key.pem
```

Now you can pair your public key with your Tesla. Get in your car, enable bluetooth
on your laptop and have your NFC card handy. Then run:

```
tesla-control -ble add-key-request public_key.pem owner cloud_key
```

The program should instruct you to confirm the new key by tapping your NFC card
on the center console.

The Locks-screen **name** of a key is Tesla account metadata, not something
stored on the vehicle. `tesla-control -ble rename-key` cannot work; use
`tesla-control rename-key` with a Fleet API token, or
`tesla-control -ble update-key` to change role and form factor locally.

WiFi provisioning (enable, add SSID/PSK, forget, connect-in-drive) is not in
the published vehicle command protocol. `tesla-control wifi` returns
`ErrWiFiNotInProtocol` and does not send credentials. See
[issue #419](https://github.com/teslamotors/vehicle-command/issues/419).

`tesla-control wake` starts infotainment if it is asleep; it does not keep
the vehicle awake. `keep-accessory-power` is the published 12V / charging-USB
setting and does not power the glovebox dashcam USB.
`tesla-control keep-awake` returns `ErrKeepAwakeNotInProtocol`. See
[issue #397](https://github.com/teslamotors/vehicle-command/issues/397).

Battery option codes (`$BT*`) are Tesla catalog metadata, not a vehicle
command. `tesla-control options VIN` calls
`GET /api/1/dx/vehicles/options`. `tesla-control battery-option VIN` prints
the `$BT*` code when Tesla included it, or `ErrBatteryOptionNotInCatalog`
when omitted (as in [issue #391](https://github.com/teslamotors/vehicle-command/issues/391)).
Do not infer pack size from `$MT322`. `tesla-control -ble options` returns
`ErrBatteryOptionRequiresFleetAPI`.

Enrolling a BLE client (`add-key-request`) creates a VCSEC whitelist key.
A controller left connected inside the vehicle can prevent Walk-Away Door
Lock, the same as leaving a phone key behind. There is no published
command-only / presence-exempt enrollment. `tesla-control ble-presence-exempt`
returns `ErrBLEKeyPresenceNotInProtocol`. Disconnect after each command.
See [issue #480](https://github.com/teslamotors/vehicle-command/issues/480).

`tesla-control charging-schedule` delivers `ScheduledChargingAction`.
Whether the car later sleeps and runs the scheduler is firmware. Cabin
overheat protection can prevent that on some Intel-MCU Model S vehicles
([issue #342](https://github.com/teslamotors/vehicle-command/issues/342)).
`tesla-control charging-schedule-overheat` returns
`ErrScheduledChargingFirmware`. This tool does not disable cabin overheat
as a workaround.

Fleet API lists `remote_boombox`, but it is not in the published vehicle
command protocol. Tesla has not released a VehicleAction pending legal
review of Pedestrian Warning System restrictions
([issue #266](https://github.com/teslamotors/vehicle-command/issues/266)).
`tesla-control boombox` returns `ErrBoomboxNotInProtocol`.
`honk` and `flash-lights` are the published alternatives.

`tesla-control climate-on` / `climate-off` send `HvacAutoAction.power_on`
(Fleet `auto_conditioning_start` / `stop`). That is climate power, not
the in-car Auto vs Manual HVAC toggle
([issue #283](https://github.com/teslamotors/vehicle-command/issues/283)).
`tesla-control hvac-auto-mode` returns `ErrHvacAutoModeNotInProtocol`.
`climate-set-temp` sets `driver_temp_celsius` and `passenger_temp_celsius`;
sending only `absolute_celsius` leaves those at proto3 zero (LO).

Enroll BLE charging gadgets as `charging_manager`, not Owner
([issue #413](https://github.com/teslamotors/vehicle-command/issues/413)).
`charge-port-open` still sends `ChargePortDoorOpen`; firmware may refuse
Charging Manager keys with insufficient privileges. This tool does not
enroll Owner as a workaround.
`tesla-control charging-manager-charge-port` returns
`ErrChargingManagerChargePortFirmware`.

`tesla-control state drive` is one Infotainment round-trip (~250–300ms
observed). BLE GetDriveState latency is vehicle firmware plus radio, not
a client timer
([issue #414](https://github.com/teslamotors/vehicle-command/issues/414)).
Reuse the session; handshake plus GetState is two RTTs (~500ms).
`tesla-control ble-state-fast` returns `ErrBLEStateLatencyFirmware`.
This tool does not disable response encryption or stream DriveState.

`seat-heater` and `seat-cooler` send published `HvacSeatHeaterActions` /
`HvacSeatCoolerActions`
([issue #383](https://github.com/teslamotors/vehicle-command/issues/383)).
Tesla `signed_command` HTTP 501 Unauthorized is Fleet API
partner/region/OAuth allowlist, not a missing command.
`tesla-control seat-heater-not-implemented` returns
`ErrSeatClimateFleetAPI`.

## Sending commands

You should now be able to send commands over BLE:

```
tesla-control -ble lock
```

If you've set up your OAuth token (see [repository README file](/README.md)),
you can also send commands over the Internet:

```
tesla-control lock
```

Run `tesla-control -h` to see a full list of supported commands.
