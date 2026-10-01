/*
Package proxy implements a REST API for sending commands to Tesla vehicles.

See the [Fleet API documentation] for available endpoints.

POST virtual_key_return, ak_return_uri, or set_virtual_key_return returns
HTTP 400 with [protocol.ErrVirtualKeyReturnURI] before opening a vehicle
session. https://tesla.com/_ak/<domain> is Tesla's hosted enrollment page.
This proxy does not append return_uri. An off-domain return URL is an
open redirect; Tesla said any redirect must stay on the registered
partner domain and be configured in advance. See
teslamotors/vehicle-command#444.

[Fleet API documentation]: https://developer.tesla.com/docs/fleet-api/getting-started/what-is-fleet-api
*/
package proxy
