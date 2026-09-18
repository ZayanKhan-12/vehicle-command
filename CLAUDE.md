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
