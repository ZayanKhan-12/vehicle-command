package account

import (
	"fmt"
	"strings"
)

// OAuth scopes defined by the Fleet API. A token carries the subset the user
// approved on Tesla's consent screen; see [Account.Scopes].
//
// The descriptions are Tesla's own, from
// https://developer.tesla.com/docs/fleet-api/authentication/overview.
const (
	// ScopeOpenID allows Tesla customers to sign in to the application with
	// their Tesla credentials.
	ScopeOpenID = "openid"

	// ScopeOfflineAccess allows getting a refresh token without needing the
	// user to log in again. A long-running application wants this one; see
	// [NewFromTokenSource].
	ScopeOfflineAccess = "offline_access"

	// ScopeUserData grants contact information, home address, profile picture,
	// and referral information.
	ScopeUserData = "user_data"

	// ScopeVehicleDeviceData grants access to the vehicle's live data, service
	// history, service scheduling data, service communications, eligible
	// upgrades, nearby Superchargers and ownership details.
	ScopeVehicleDeviceData = "vehicle_device_data"

	// ScopeVehicleLocation grants access to vehicle location information.
	ScopeVehicleLocation = "vehicle_location"

	// ScopeVehicleCmds grants commands like add/remove driver, access Live
	// Camera, unlock, wake up, remote start, and schedule software updates.
	ScopeVehicleCmds = "vehicle_cmds"

	// ScopeVehicleChargingCmds grants vehicle charging history, billed amount,
	// charging location, commands to schedule, and start/stop charging.
	ScopeVehicleChargingCmds = "vehicle_charging_cmds"

	// ScopeVehicleSpecs grants detailed vehicle specifications. Partner tokens
	// only.
	ScopeVehicleSpecs = "vehicle_specs"

	// ScopeVehiclePricingInfo grants vehicle pricing details. Partner tokens
	// only.
	ScopeVehiclePricingInfo = "vehicle_pricing_info"

	// ScopeEnergyDeviceData grants energy live status, site info, backup
	// history, energy history, and charge history.
	ScopeEnergyDeviceData = "energy_device_data"

	// ScopeEnergyCmds grants updates to settings like backup reserve percent,
	// operation mode, and storm mode.
	ScopeEnergyCmds = "energy_cmds"

	// ScopeEnterpriseManagement grants access to enterprise management
	// functions for businesses.
	ScopeEnterpriseManagement = "enterprise_management"
)

// A ScopeError reports that an OAuth token does not carry every scope an
// operation needs. It is returned by [Account.RequireScopes].
type ScopeError struct {
	// Missing lists the required scopes the token does not carry, in the order
	// they were requested.
	Missing []string
	// Granted lists every scope the token does carry.
	Granted []string
}

func (e *ScopeError) Error() string {
	granted := "none"
	if len(e.Granted) > 0 {
		granted = strings.Join(e.Granted, ", ")
	}
	return fmt.Sprintf(
		"OAuth token is missing required scope(s) %s (it grants %s); "+
			"the user must repeat the authorization flow and approve those permissions, "+
			"which they can also update under Third-Party Apps in their Tesla account settings",
		strings.Join(e.Missing, ", "), granted)
}

// Scopes returns the OAuth scopes the token states it was granted, taken from
// the token's scp claim.
//
// It returns nil if the token does not state its scopes at all, which is not
// the same as stating that none were granted. Callers must not treat nil as an
// empty set; see [Account.RequireScopes].
//
// The returned slice is a copy and may be modified freely.
func (a *Account) Scopes() []string {
	if a.scopes == nil {
		return nil
	}
	// Not append([]string(nil), ...): that yields nil for an empty slice, which
	// would erase the distinction between a token that grants no scopes and one
	// that does not state them.
	return append(make([]string, 0, len(a.scopes)), a.scopes...)
}

// HasScope reports whether the token states it was granted scope.
//
// It reports false for a token that does not state its scopes. Use
// [Account.Scopes] to tell the two cases apart.
func (a *Account) HasScope(scope string) bool {
	for _, granted := range a.scopes {
		if granted == scope {
			return true
		}
	}
	return false
}

// MissingScopes returns the subset of required that the token does not carry,
// preserving the order given.
//
// It returns nil if the token does not state its scopes. This is deliberate:
// an application must not be locked out of a token that would in fact work,
// so an unstated scope set is reported as "nothing missing" rather than as
// "everything missing". The check is an early warning, not an authorization
// decision -- Tesla's servers remain the only authority on what a token may do.
func (a *Account) MissingScopes(required ...string) []string {
	if a.scopes == nil {
		return nil
	}
	var missing []string
	for _, scope := range required {
		if !a.HasScope(scope) {
			missing = append(missing, scope)
		}
	}
	return missing
}

// RequireScopes returns a *[ScopeError] naming every scope in required that the
// token does not carry, or nil if it carries them all.
//
// The point is to fail immediately, at a place that can name the problem,
// rather than several calls later with an HTTP 403 or 412 that does not say
// which permission was withheld. The consent screen lets a user approve some of
// the requested scopes and decline others, and nothing in the flow tells the
// application which boxes were ticked.
//
// Like [Account.MissingScopes], this returns nil for a token that does not
// state its scopes.
func (a *Account) RequireScopes(required ...string) error {
	missing := a.MissingScopes(required...)
	if len(missing) == 0 {
		return nil
	}
	return &ScopeError{Missing: missing, Granted: a.Scopes()}
}
