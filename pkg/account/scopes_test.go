package account

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// accountWithScopes builds an Account from a JWT carrying the given scp claim.
// Pass nil for a token that does not state its scopes at all.
func accountWithScopes(t *testing.T, scopes []string) *Account {
	t.Helper()
	payload := &oauthPayload{
		Audiences: []string{"https://fleet-api.prd.na.vn.cloud.tesla.com"},
		Scopes:    scopes,
	}
	acct, err := New(makeTestJWT(payload), "")
	if err != nil {
		t.Fatalf("New failed: %s", err)
	}
	return acct
}

// TestScopes checks that the scp claim is read off the token.
func TestScopes(t *testing.T) {
	granted := []string{ScopeOpenID, ScopeVehicleDeviceData, ScopeVehicleCmds}
	acct := accountWithScopes(t, granted)

	if got := acct.Scopes(); !reflect.DeepEqual(got, granted) {
		t.Errorf("Scopes() = %v, want %v", got, granted)
	}
	if !acct.HasScope(ScopeVehicleCmds) {
		t.Error("HasScope(vehicle_cmds) = false, want true")
	}
	if acct.HasScope(ScopeVehicleChargingCmds) {
		t.Error("HasScope(vehicle_charging_cmds) = true, want false")
	}
}

// TestScopesReturnsACopy checks that a caller cannot corrupt the Account by
// modifying the slice it was handed.
func TestScopesReturnsACopy(t *testing.T) {
	acct := accountWithScopes(t, []string{ScopeVehicleCmds})
	scopes := acct.Scopes()
	scopes[0] = "tampered"
	if !acct.HasScope(ScopeVehicleCmds) {
		t.Error("modifying the returned slice changed the Account's scopes")
	}
}

// TestScopesUnstated pins the fail-open contract. A token that does not state
// its scopes must not be reported as missing all of them: doing so would lock
// an application out of a token that works.
func TestScopesUnstated(t *testing.T) {
	acct := accountWithScopes(t, nil)

	if got := acct.Scopes(); got != nil {
		t.Errorf("Scopes() = %v, want nil for a token with no scp claim", got)
	}
	if got := acct.MissingScopes(ScopeVehicleCmds, ScopeUserData); got != nil {
		t.Errorf("MissingScopes() = %v, want nil when the token does not state its scopes", got)
	}
	if err := acct.RequireScopes(ScopeVehicleCmds); err != nil {
		t.Errorf("RequireScopes() = %v, want nil when the token does not state its scopes", err)
	}
}

// TestScopesStatedButEmpty is the other side of TestScopesUnstated: a token
// that explicitly grants nothing is a different case from one that is silent,
// and it must be reported.
func TestScopesStatedButEmpty(t *testing.T) {
	acct := accountWithScopes(t, []string{})

	if got := acct.Scopes(); got == nil || len(got) != 0 {
		t.Errorf("Scopes() = %v, want a non-nil empty slice", got)
	}
	if got := acct.MissingScopes(ScopeVehicleCmds); !reflect.DeepEqual(got, []string{ScopeVehicleCmds}) {
		t.Errorf("MissingScopes() = %v, want [vehicle_cmds]", got)
	}
}

// TestMissingScopes checks that only the absent scopes are reported, in the
// order they were asked for.
func TestMissingScopes(t *testing.T) {
	acct := accountWithScopes(t, []string{ScopeOpenID, ScopeVehicleDeviceData})

	want := []string{ScopeVehicleCmds, ScopeUserData}
	got := acct.MissingScopes(ScopeVehicleCmds, ScopeOpenID, ScopeUserData, ScopeVehicleDeviceData)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MissingScopes() = %v, want %v", got, want)
	}
	if got := acct.MissingScopes(ScopeOpenID, ScopeVehicleDeviceData); got != nil {
		t.Errorf("MissingScopes() = %v, want nil when everything is granted", got)
	}
}

// TestRequireScopes checks the error a caller gets, since the whole point is
// that it names the problem instead of leaving a 403 to be guessed at.
func TestRequireScopes(t *testing.T) {
	acct := accountWithScopes(t, []string{ScopeOpenID, ScopeVehicleDeviceData})

	if err := acct.RequireScopes(ScopeOpenID); err != nil {
		t.Errorf("RequireScopes(openid) = %v, want nil", err)
	}

	err := acct.RequireScopes(ScopeVehicleCmds, ScopeOpenID, ScopeUserData)
	if err == nil {
		t.Fatal("RequireScopes returned nil for a token missing two scopes")
	}

	var scopeErr *ScopeError
	if !errors.As(err, &scopeErr) {
		t.Fatalf("error is %T, want *ScopeError", err)
	}
	if want := []string{ScopeVehicleCmds, ScopeUserData}; !reflect.DeepEqual(scopeErr.Missing, want) {
		t.Errorf("Missing = %v, want %v", scopeErr.Missing, want)
	}
	if want := []string{ScopeOpenID, ScopeVehicleDeviceData}; !reflect.DeepEqual(scopeErr.Granted, want) {
		t.Errorf("Granted = %v, want %v", scopeErr.Granted, want)
	}

	// The message has to be usable by whoever reads the log, so it names both
	// what is missing and what is present.
	message := err.Error()
	for _, substring := range []string{ScopeVehicleCmds, ScopeUserData, ScopeVehicleDeviceData, "Third-Party Apps"} {
		if !strings.Contains(message, substring) {
			t.Errorf("error message %q does not mention %q", message, substring)
		}
	}
	if strings.Contains(message, ScopeEnergyCmds) {
		t.Errorf("error message %q mentions a scope that was neither required nor granted", message)
	}
}

// TestScopeErrorWithNoGrants checks the wording when the token grants nothing,
// so that the message does not read "it grants ".
func TestScopeErrorWithNoGrants(t *testing.T) {
	acct := accountWithScopes(t, []string{})
	err := acct.RequireScopes(ScopeVehicleCmds)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "it grants none") {
		t.Errorf("error message %q should say the token grants none", err.Error())
	}
}

// TestScopesClaimAbsent uses a token with no scp key at all, rather than one
// whose scp is null, since that is the shape a real token without scopes has.
func TestScopesClaimAbsent(t *testing.T) {
	jwt := fmt.Sprintf("x.%s.y", b64Encode(`{"aud": ["https://fleet-api.prd.na.vn.cloud.tesla.com"]}`))
	acct, err := New(jwt, "")
	if err != nil {
		t.Fatalf("New failed: %s", err)
	}
	if got := acct.Scopes(); got != nil {
		t.Errorf("Scopes() = %v, want nil", got)
	}
	if err := acct.RequireScopes(ScopeVehicleCmds); err != nil {
		t.Errorf("RequireScopes() = %v, want nil", err)
	}
}
