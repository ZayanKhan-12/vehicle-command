/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

# Partner OAuth audience / policy

POST partner_token, register_partner, oauth_audience, or
invalid_audience returns HTTP 400 with
[protocol.ErrPartnerOAuthNotProvisioned] before opening a vehicle
session. Tesla Fleet Auth invalid_audience on client_credentials and
/authorize "No policy rules" mean Tesla has not bound OAuth
policy/audience to the application. tesla-http-proxy consumes an
already-issued OAuth token; it does not mint partner tokens or POST
/api/1/partner_accounts. Cycling regional fleet-api audience URLs
does not provision Tesla's IdP. Use Tesla developer dashboard Support
Inquiry. See teslamotors/vehicle-command#460.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
