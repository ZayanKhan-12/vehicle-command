// Package account implements functions for managing a Tesla account.
//
// An [Account] is created from an OAuth token and hands out vehicles:
//
//	acct, err := account.New(token, "my-app/1.0")
//	car, err := acct.GetVehicle(ctx, vin, privateKey, sessions)
//
// A token expires, so an Account built this way stops working after a few
// hours. A program that runs longer than one token's lifetime should supply an
// oauth2.TokenSource instead and let the Account refresh for itself:
//
//	source := config.TokenSource(ctx, token) // config is an *oauth2.Config
//	acct, err := account.NewFromTokenSource(source, "my-app/1.0")
//
// Vehicles obtained from that Account share its credentials, so commands sent
// long after the original token expired still authenticate. See
// [NewFromTokenSource] and [WithTokenSource].
//
// Both constructors accept options. [WithClient] replaces the http.Client used
// for every request the Account and its vehicles make, which is the hook for
// request logging, an outbound proxy, a custom TLS configuration or a timeout.
package account
