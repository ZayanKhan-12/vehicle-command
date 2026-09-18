# CLAUDE.md

Guidance for Claude Code and other AI assistants working in this repository.

## What this is

The Tesla Vehicle Command SDK is a Go library and a set of command-line tools for sending
commands to Tesla vehicles. Its defining property is that **authentication happens twice, at
two different layers, and the two are independent**:

1. Tesla's servers forward a message to a vehicle only if the caller presents a valid OAuth
   token.
2. The vehicle executes the command only if it can authenticate it against a public key in the
   vehicle's own keychain.

Almost every design decision follows from that split. OAuth concerns the *transport* to Tesla's
Fleet API; the command-authentication private key concerns the *payload*, which is an end-to-end
authenticated protobuf that Tesla's servers relay without being able to forge. A change that
blurs the two — for example, one that lets a transport-level credential influence what the
vehicle accepts — is wrong even if the tests pass.

The same command payload can reach a vehicle over the internet (`pkg/connector/inet`) or over
Bluetooth (`pkg/connector/ble`), which is why the payload layer knows nothing about HTTP.

## Layout

| Path | Contents |
| :--- | :--- |
| `pkg/account/` | `Account`: parses the OAuth JWT, derives the regional Fleet API host, and hands out `vehicle.Vehicle` values. The entry point for library users. |
| `pkg/vehicle/` | `Vehicle`: the command surface (`Lock`, `ChargeStart`, …). Transport-agnostic. |
| `pkg/connector/` | The `Connector` and `FleetAPIConnector` interfaces, plus `inet/` (HTTPS to Fleet API) and `ble/` (Bluetooth) implementations. |
| `pkg/protocol/` | Wire format, protobuf definitions (`protobuf/`), and error classification. |
| `pkg/proxy/` | The REST proxy that holds the private key so that existing Fleet API clients need no protocol changes. |
| `pkg/cli/` | Shared flag parsing and credential loading for the `cmd/` tools. |
| `pkg/cache/`, `pkg/sign/` | Session cache (avoids a handshake round trip) and JWS signing. |
| `internal/dispatcher/` | Sequencing, retries and session state between `Vehicle` and `Connector`. |
| `internal/authentication/` | ECDH/HMAC/GCM key handling. Has a native fallback for toolchains without `crypto/ecdh`. |
| `cmd/` | `tesla-control`, `tesla-http-proxy`, `tesla-auth-token`, `tesla-keygen`, `tesla-jws`. |

`Vehicle` talks to a `Connector`, never to `net/http`. When adding transport behaviour, add it to
the connector (or to the `http.Client` the connector holds), not to `Vehicle`.

## Checks

CI is `.github/workflows/build.yml` on Go **1.23.0**, and it runs four gates. Run all four before
proposing a change:

```sh
make format && git diff --exit-code   # gofmt must be a no-op
make linters                          # golangci-lint, config in .golangci.yml
make test                             # go test -cover ./... && go vet ./...
go build ./...
```

Two things about `make linters` that cost time to discover:

- **CI pins golangci-lint v1.61.0**, and the flags in the `Makefile` (`--exclude-use-default`)
  were removed in golangci-lint v2. A v2 binary fails before it lints anything. Install the
  pinned version rather than editing the `Makefile`.
- **Run it under Go 1.23**, e.g. `GOTOOLCHAIN=go1.23.0`. Under a newer toolchain, v1.61.0's
  embedded type checker reports about a dozen phantom `typecheck` errors in
  `internal/authentication/native.go` and `pkg/protocol/key.go` — these are build-tag artifacts
  of the version skew, not real problems, and they appear on an untouched `main` too. Confirm
  against `main` before believing any failure in those files.

`make test` depends on `install`, so it builds `cmd/...` as a side effect. `make format` depends
on `set-version`, which rewrites `pkg/account/version.txt` from `git describe --tags`; in a clone
without tags it is a no-op, which is why CI's `git diff --exit-code` passes.

## Conventions

- Exported identifiers get doc comments in complete sentences, and the codebase uses the
  `[Symbol]` link form. `revive`'s `exported` rule is deliberately disabled (there is a comment
  saying "uncomment when all issues are fixed"), so the linter will not catch a missing comment —
  write it anyway.
- `errcheck` is on. An intentionally discarded error is written `_ = thing.Close()`.
- `revive`'s `unused-parameter` is on: name an unused parameter `_`. This bites most often in
  `http.HandlerFunc` literals in tests.
- Tests are standard library only — `testing`, `net/http/httptest`. There is no assertion
  library and no mocking framework; don't add one.
- Errors that a caller may need to act on are classified, not just returned: see
  `protocol.Temporary`, `protocol.MayHaveSucceeded` and `inet.HTTPError`. A new failure mode
  should say whether it is retryable and whether the command may already have taken effect.

## Configuring the HTTP client

`Account` and `inet.Connection` each own an `http.Client`. Until recently both were private with
no way in, so a caller who wanted request logging, a proxy, a custom TLS configuration or a
timeout had only one lever: mutating `http.DefaultClient`, which is a process-global change that
a larger application cannot safely make (issue #23).

Both now accept options:

```go
acct, err := account.New(token, userAgent, account.WithClient(myClient))
conn := inet.NewConnection(vin, authHeader, host, userAgent, inet.WithClient(myClient))
```

Three properties are load-bearing, and there is a test for each:

- **`Account.GetVehicle` forwards the client into the connection it creates.** Without this, a
  caller could configure the `Account`'s transport and still have every vehicle command bypass
  it — the failure would be silent, because the commands would still work. `TestWithClientReachesVehicles`
  drives a real `Wakeup` through an injected transport to pin this down.
- **A `nil` client leaves the default in place** rather than producing a value that panics on
  first use.
- **The option stores the client, it does not copy it.** `http.Client` contains no lock and is
  documented as safe for concurrent use, so sharing is correct; mutating it afterwards is not.

The tests verify injection by pointing the `Account` at an `httptest.NewTLSServer` and relying on
the fact that the *default* client rejects its self-signed certificate. A request that succeeds
therefore proves the injected client carried it. `TestDefaultClientRejectsTestServer` asserts that
baseline explicitly, so if a future change makes the default client trust the test server, that
test fails loudly instead of the others quietly passing for the wrong reason.

Note that `New` takes options variadically. Adding a variadic parameter keeps every existing call
site compiling; it does change the function's type, which would affect code that assigns
`account.New` to a `func(string, string) (*Account, error)` variable. That form does not appear in
this repository or in its examples.

## Refreshing credentials

An `Account` built by `New` holds one bearer string for its whole lifetime, so it stops working
when that token expires — a few hours. The fix is not to re-create the `Account` on a timer but
to give it an `oauth2.TokenSource`, which is Go's standard shape for "hand me a valid token"
(issue #29):

```go
acct, err := account.NewFromTokenSource(config.TokenSource(ctx, token), userAgent)
```

The implementation is deliberately thin, and that is the thing to preserve. Rather than adding a
credential-refresh path of its own, `applyTokenSource` layers an `oauth2.Transport` over whatever
`http.Client` the options left in place, and clears `authHeader`:

- **Refresh lives in the transport**, so it applies to every request the SDK makes without each
  call site having to remember it. In particular, vehicles inherit it for free, because
  `GetVehicle` already forwards the client (see above). That inheritance is the part most likely
  to regress: an account-level refresh that vehicles do not share fails *later*, under load, in
  production, which is the worst place to find it. `TestTokenSourceReachesVehicles` covers it.
- **`authHeader` is emptied, not left stale.** A stored copy of the first token would be the value
  vehicle connections inherit, and would silently win over the fresh one if the ordering ever
  changed. Both `Account.Get` and `inet.SendFleetAPICommand` now skip an empty `Authorization`
  header, treating "empty" as "the Transport supplies it".
- **The client is copied, not mutated**, because it may be the caller's, shared with the rest of
  their program.
- **The wrapping happens after all options are applied**, so `WithClient` and `WithTokenSource`
  compose in either order. A test runs both orders.
- **`oauth2.ReuseTokenSource` caches** the token until it nears expiry, so enabling refresh does
  not turn one API call into two. `TestTokenSourceReusesValidToken` asserts the source is
  consulted twice for three requests — once to learn the region, once for the first request.

`NewFromTokenSource` spends one token during construction, because the Fleet API region is
encoded in the token's audience claim and has to be known before any request can be addressed.
The tests count tokens from there, which is why they expect `SUBJECT-2` on the first request.

`golang.org/x/oauth2` is pinned to **v0.24.0**: it is the newest release that still declares
`go 1.18`. From v0.27.0 onwards the module requires `go 1.23.0`, which would force this module's
`go` directive up and raise the floor for everyone who depends on it. Check that constraint before
bumping.

## OAuth scopes

Tesla's consent screen lets a user approve some of the scopes an application asked for and decline
the rest, and nothing in the redirect says which boxes were ticked. The application finds out
later, as an HTTP 403 or 412 from an endpoint that does not name the missing permission — see
issue #63, and issue #49, which is that failure with `user_data` specifically.

**The SDK cannot fix the root cause.** "Require the user to select the correct scopes" is a
property of Tesla's authorization server, and this repository never performs the grant:
`tesla-auth-token` reads a token that was obtained elsewhere. What the SDK can do, and now does,
is answer the question the issue asks next — *"what is the best way to detect the scopes they
selected?"*

`pkg/account/scopes.go` reads the token's `scp` claim and exposes it:

```go
if err := acct.RequireScopes(account.ScopeVehicleCmds, account.ScopeVehicleDeviceData); err != nil {
    return err // names the missing scopes and how to fix them
}
```

One rule governs this code and must not be weakened: **it fails open.** A token whose `scp` claim
is absent reports *nothing* missing, not *everything* missing. The SDK cannot distinguish "no
scopes granted" from "this token does not describe itself", and wrongly refusing a token that
would have worked is a worse failure than not warning about one that will not. Tesla's servers
remain the only authority on what a token may do; `RequireScopes` is an early warning, never an
authorization decision. `TestScopesUnstated` pins this, and `TestScopesStatedButEmpty` pins the
distinction from `"scp": []`, which *is* reported.

That distinction is why `Scopes()` does not use `append([]string(nil), ...)` — that idiom returns
nil for an empty slice and would quietly collapse the two cases. The comment in place says so.

The scope constants and their doc comments are Tesla's own list, from
https://developer.tesla.com/docs/fleet-api/authentication/overview. Note that the SDK does not
ship a mapping from commands to required scopes: which scopes a given command needs is Tesla's to
define and change, and a guess baked in here would be wrong quietly. Callers compose the set they
need from the constants.

## What cannot be added from this repository

Some feature requests here cannot be satisfied by any change to this code, and it saves a lot of
effort to recognise that shape early. The boundary is the **protocol**, not the SDK.

`pkg/vehicle` can only expose a command whose message the vehicle already understands. Commands
travel as a `VehicleAction` — a protobuf `oneof` in `pkg/protocol/protobuf/car_server.proto`,
currently 60 entries — signed with the application's private key and relayed by Tesla's servers,
which do not translate. A third party cannot invent a field number and have a car act on it. The
alternative route, the Fleet API REST surface, is likewise Tesla's to define; the proxy already
has an `ErrCommandUseRESTAPI` path for the handful of commands that live there instead
(`navigation_request`, the managed-charging trio).

There are **two** command domains, and a check that looks at only one is incomplete. Infotainment
commands are `VehicleAction`s in `car_server.proto`; body-control commands — doors, trunks, charge
port, tonneau — are `ClosureMoveRequest` and friends in `vcsec.proto`, which `tesla-control`
reaches over BLE even when infotainment is asleep. So for any "please add command X" request,
three checks settle it:

```sh
grep -rin "X" pkg/protocol/protobuf/*.proto        # any action, in either domain?
# and: is there a Fleet API endpoint for it?
```

If none exists, the request is a Tesla roadmap item and the issue cannot be closed by code here.
Say so, with the evidence, rather than building something that cannot work.

A refinement that is worth checking before concluding: **the protocol often reports more than it
can command.** A request to control some part of the car individually may be impossible while the
corresponding *state* is already available per-part, and saying so is much more useful than a flat
no. Issue #122 below is exactly that case.

### Worked example: Summon (issue #114)

Both checks come back empty:

- **No action in the command protocol.** Nothing matching `summon`, `autopark` or `auto_park`
  appears in any `.proto` file in `pkg/protocol/protobuf/`. The nearest neighbour,
  `VehicleControlTriggerHomelinkAction`, is Homelink.
- **No endpoint in the Fleet API.** The vehicle endpoints reference lists no summon or autopark
  endpoint. The legacy owner-API endpoints that third parties once used for this were shut down
  on 2024-03-26.

There is also a second, independent blocker that a new protobuf message alone would not remove:
Summon is a *continuously supervised* manoeuvre. It needs a live channel with a dead-man's switch
— the operator holds a control and the car stops when they let go — which is why the old
implementations used the streaming API. The command protocol here is request/response and
carries no such channel, so supporting Summon means designing one, not adding a method.

A collaborator's answer on the issue ("We currently do not have a roadmap for this") is therefore
the whole status: the work is Tesla's, on both counts. Do not fabricate an implementation. This
one physically moves a vehicle, and a plausible-looking method that cannot work — or worse, a
guessed field number aimed at a car — is far worse than an unimplemented feature.

### Worked example: per-window control (issue #122)

The request is a `window` parameter on `window_control`, naming one of the four windows. It cannot
be built, and both domains have to be checked to say so:

- **Infotainment.** `VehicleControlWindowAction` carries `unknown`, `vent` and `close` and nothing
  else. There is no selector, so the action is inherently all-windows. (Field 1 is `reserved` — it
  was the location, no longer required for vehicles on this protocol.)
- **Body control.** `ClosureMoveRequest` in `vcsec.proto` *does* address parts individually —
  `frontDriverDoor`, `frontPassengerDoor`, `rearDriverDoor`, `rearPassengerDoor`, `rearTrunk`,
  `frontTrunk`, `chargePort`, `tonneau` — which makes it the plausible place for this to live. It
  is not there. Windows are not closures in this protocol.

But the asymmetry is the useful part of the answer. `ClosuresState` in `vehicle.proto` reports each
window separately:

```
window_open_driver_front = 107     window_open_passenger_front = 108
window_open_driver_rear  = 109     window_open_passenger_rear  = 110
```

That is already exposed, through `StateCategoryClosures` and `tesla-control state closures`. So the
honest answer to #122 is that the car will *tell* you which window is open but will only *act* on
all four together — not simply "unsupported".

One separate thing that recurs on this issue: `window_control` failing for callers who use the
Fleet API directly is usually the `lat`/`lon` requirement, which Tesla enforces on `close` as an
anti-theft measure. Vehicles on the command protocol do not need it, which is why the proxy ignores
those parameters and why the protobuf field is `reserved`.

## Preconditioning, and the three things that word means here

Issue #115 asks for a `set_battery_preconditioning` command with `standard`/`fast`/`off`. Applying
the check above: there is no such action in `car_server.proto`, and no such Fleet API endpoint.
A collaborator has filed an internal ticket, so it is a Tesla roadmap item.

What makes this issue different from #114 is that the SDK *does* have preconditioning commands,
three separate ones, and the thread visibly conflates them. Anyone answering a preconditioning
question should keep them apart:

| What the user means | What it is | SDK |
| :--- | :--- | :--- |
| "warm the cabin fast" | Max defrost. **Cabin, not battery** — the name misleads. | `Vehicle.SetPreconditioningMax` |
| "be warm when I leave at 7am" | Scheduled departure preconditioning, cabin *and* battery. | `Vehicle.ScheduleDeparture`, `Vehicle.AddPreconditionSchedule` |
| "warm the battery now, for fast charging" | **Does not exist as a command.** | — |

The SDK already exposes every preconditioning action the protocol defines, so this is not a
coverage gap in `pkg/vehicle`; it is an absent capability.

The one thing that *does* precondition the battery on demand today is routing the car to a
Supercharger, which the vehicle responds to by preheating. That is `navigation_sc_request`, a
REST-only Fleet API command — which the proxy used to reject; see below.

## Fleet API commands the proxy does not implement

`ExtractCommandAction` in `pkg/proxy/command.go` maps a Fleet API command name to an action. Its
`default` case answers **HTTP 400 `invalid_command`**, and that is the trap: a command the proxy
does not recognise is not merely unimplemented, it is *blocked*. The request never reaches Tesla,
even though the proxy is forwarding everything else for that vehicle.

The escape hatch is `ErrCommandUseRESTAPI`. `Proxy.ServeHTTP` treats it as "forward this one
unchanged", which is right for any command that has no representation in the signed protocol —
usually because the vehicle is not the only participant. So there are three outcomes, and a new
command must be put in the correct one:

1. It has a `VehicleAction` → implement it, calling the `pkg/vehicle` method.
2. It has none → `return nil, ErrCommandUseRESTAPI`, so the proxy forwards it.
3. It is not a Fleet API command at all → fall through to the 400.

Getting (2) wrong looks exactly like Tesla rejecting the command, which is why it went unnoticed:
seven documented commands sat in outcome (3). Six were REST-only and are now forwarded; the
seventh, `sun_roof_control`, belonged in (1) and is implemented. `TestRESTOnlyCommandsAreForwarded`
covers the forwarding, `TestSunRoofControl` the implementation, and `TestUnknownCommandIsRejected`
guards the other direction so none of this can be generalised into forwarding anything at all.

The proxy currently implements or forwards **every** command in that reference. If the diff below
comes back non-empty, something has been added upstream.

To re-audit after a Fleet API release, diff the [vehicle commands
reference](https://developer.tesla.com/docs/fleet-api/endpoints/vehicle-commands) against the
`case` labels in `ExtractCommandAction`.

### `sun_roof_control`, and why it spans all three outcomes

`VehicleControlSunroofOpenCloseAction` carries two independent oneofs: a level
(`absolute_level`/`delta_level`) and a named action (`vent`, `close`, `open`). `ChangeSunroofState`
only ever set the level, so the named positions had nothing to call — which is why this command sat
unimplemented even though the protocol supported it.

`Vehicle.VentSunroof`, `CloseSunroof` and `OpenSunroof` now send the action variants, matching the
existing `VentWindows`/`CloseWindows` pair. The Fleet API's `state` parameter then maps onto all
three outcomes at once, which makes this command the clearest illustration of the rule above:

| `state` | Outcome | Why |
| :--- | :--- | :--- |
| `vent`, `close` | (1) implemented | The action message names them. |
| `stop` | (2) forwarded | The Fleet API defines it; the protobuf has no `stop`. |
| anything else | (3) rejected | Not a Fleet API state. |

Note the asymmetry in the last two rows, because it is deliberate. `open` **is** in the protobuf and
**is** exposed on `Vehicle` and in `tesla-control`, but the proxy rejects `state: "open"` rather
than accepting it. The proxy exists to be a drop-in Fleet API, so accepting a state Tesla does not
define would let code work against the proxy and then fail against Tesla. Capabilities the protocol
has but the Fleet API lacks belong on `Vehicle`, not in the proxy's command table.

Levels and named actions stay separate: do not map `vent` to a guessed percentage.
