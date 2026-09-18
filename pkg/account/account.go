package account

import (
	"context"
	"crypto/ecdh"
	_ "embed" // Used to embed version for use with user agent
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"

	"golang.org/x/oauth2"

	"github.com/teslamotors/vehicle-command/internal/authentication"
	"github.com/teslamotors/vehicle-command/internal/log"
	"github.com/teslamotors/vehicle-command/pkg/cache"
	"github.com/teslamotors/vehicle-command/pkg/connector"
	"github.com/teslamotors/vehicle-command/pkg/connector/inet"
	"github.com/teslamotors/vehicle-command/pkg/vehicle"
)

var (
	//go:embed version.txt
	libraryVersion string
)

func buildUserAgent(app string) string {
	library := strings.TrimSpace("tesla-sdk/" + libraryVersion)
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return library
	}
	path := strings.Split(build.Path, "/")
	if len(path) == 0 {
		return library
	}

	if app == "" {
		app = path[len(path)-1]
		var version string
		if build.Main.Version != "(devel)" && build.Main.Version != "" {
			version = build.Main.Version
		} else {
			for _, info := range build.Settings {
				if info.Key == "vcs.revision" {
					if len(info.Value) > 8 {
						version = info.Value[0:8]
					}
					break
				}
			}
		}

		if version != "" {
			app = fmt.Sprintf("%s/%s", app, version)
		}
	}

	return fmt.Sprintf("%s %s", app, library)
}

// Account allows interaction with a Tesla account.
type Account struct {
	// The default UserAgent is constructed from the global UserAgent, but can be overridden.
	UserAgent  string
	authHeader string
	Host       string
	Subject    string
	scopes     []string
	client     *http.Client

	tokenSource oauth2.TokenSource
}

// An Option modifies an Account returned by [New].
type Option func(*Account)

// WithClient makes the Account send its requests with client instead of with a
// default [http.Client]. Vehicles returned by [Account.GetVehicle] inherit the
// client, so a single call covers every Fleet API request the Account is
// responsible for.
//
// This is the hook for behavior that lives in the transport: an
// [http.RoundTripper] that logs or instruments requests, a proxy, a custom TLS
// configuration, or a client-wide timeout. Previously the only way to influence
// any of that was to modify [http.DefaultClient], which is not an option for a
// program that also makes unrelated HTTP requests.
//
// The Account keeps a reference to client rather than a copy of it, so do not
// modify client after passing it here. A nil client leaves the default in
// place.
func WithClient(client *http.Client) Option {
	return func(a *Account) {
		if client != nil {
			a.client = client
		}
	}
}

// WithTokenSource makes the Account obtain an OAuth token from source for each
// request, rather than reusing the single token it was constructed with.
//
// An [Account] built from a token string holds that token for its whole
// lifetime, so a long-running program has to discard the Account and build a
// new one every time the token expires -- and must notice the expiry itself.
// An [oauth2.TokenSource] is the idiomatic Go answer: it hands out a valid
// token on demand and refreshes in the background. Vehicles returned by
// [Account.GetVehicle] share the refreshed credentials, so commands sent hours
// later keep working.
//
// The token source is consulted through [oauth2.ReuseTokenSource], so a token
// is fetched again only once the previous one is close to expiring, not on
// every request.
//
// WithTokenSource composes with [WithClient] in either order: the token source
// is layered over whichever client is in place once all options have been
// applied, leaving that client's Transport to carry the request.
//
// See [NewFromTokenSource] for the usual way to build such an Account.
func WithTokenSource(source oauth2.TokenSource) Option {
	return func(a *Account) {
		a.tokenSource = source
	}
}

// applyTokenSource layers a token source, if one was configured, over the
// client the options left in place. It runs after all options so that the
// result does not depend on the order they were given in.
func (a *Account) applyTokenSource() {
	if a.tokenSource == nil {
		return
	}
	// Copy the client rather than modify it. It may have come from the caller
	// through WithClient, and may be shared with the rest of their program.
	client := *a.client
	client.Transport = &oauth2.Transport{
		Source: oauth2.ReuseTokenSource(nil, a.tokenSource),
		Base:   a.client.Transport,
	}
	a.client = &client
	// The Authorization header is now set per request from the token source.
	// A copy stored here could only ever go stale, and would be the value the
	// vehicle connections inherited.
	a.authHeader = ""
}

// We don't parse JWTs beyond what's required to extract the API server domain name
type oauthPayload struct {
	Audiences []string `json:"aud"`
	OUCode    string   `json:"ou_code"`
	Subject   string   `json:"sub"`
	Scopes    []string `json:"scp"`
}

var domainRegEx = regexp.MustCompile(`^[A-Za-z0-9-.]+$`) // We're mostly interested in stopping paths; the http package handles the rest.
var remappedDomains = map[string]string{}                // For use during development; populate in an init() function.

const defaultDomain = "fleet-api.prd.na.vn.cloud.tesla.com"

func (p *oauthPayload) domain() string {
	if len(remappedDomains) > 0 {
		for _, a := range p.Audiences {
			if d, ok := remappedDomains[a]; ok {
				return d
			}
		}
	}
	domain := defaultDomain
	ouCodeMatch := fmt.Sprintf(".%s.", strings.ToLower(p.OUCode))
	for _, u := range p.Audiences {
		if strings.HasPrefix(u, "https://auth.tesla.") {
			continue
		}
		d, _ := strings.CutPrefix(u, "https://")
		d, _ = strings.CutSuffix(d, "/")
		if !domainRegEx.MatchString(d) {
			continue
		}

		if inet.ValidTeslaDomainSuffix(d) && strings.HasPrefix(d, "fleet-api.") {
			domain = d
			// Prefer domains that contain the ou_code (region)
			if strings.Contains(domain, ouCodeMatch) {
				return domain
			}
		}
	}
	return domain
}

// New returns an [Account] that can be used to fetch a [vehicle.Vehicle].
// Optional userAgent can be passed in - otherwise it will be generated from code.
// Zero or more [Option] values may be passed to configure the Account; see
// [WithClient].
func New(oauthToken, userAgent string, options ...Option) (*Account, error) {
	parts := strings.Split(oauthToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("client provided malformed OAuth token")
	}
	payloadJSON, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("client provided malformed OAuth token: %s (%s)", err, parts[1])
	}
	var payload oauthPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, fmt.Errorf("client provided malformed OAuth token: %s", err)
	}

	domain := payload.domain()
	if domain == "" {
		return nil, fmt.Errorf("client provided OAuth token with invalid audiences")
	}
	account := &Account{
		UserAgent:  buildUserAgent(userAgent),
		authHeader: "Bearer " + strings.TrimSpace(oauthToken),
		Host:       domain,
		Subject:    payload.Subject,
		scopes:     payload.Scopes,
		client:     &http.Client{},
	}
	for _, option := range options {
		option(account)
	}
	account.applyTokenSource()
	return account, nil
}

// NewFromTokenSource returns an [Account] that takes its OAuth token from
// source, refreshing it as needed, and that can be used to fetch a
// [vehicle.Vehicle].
//
// This is the constructor to use in a long-running program. Unlike [New], the
// returned Account does not stop working when the token it started with
// expires; see [WithTokenSource].
//
// One token is fetched immediately, because the Fleet API region this account
// belongs to is encoded in the token's audience claim and must be known before
// any request can be addressed. A source that cannot produce a token yet
// therefore fails here rather than at first use.
//
// Building a source is the caller's job, and is usually a matter of
//
//	config.TokenSource(ctx, token)
//
// on an [oauth2.Config]. Note that the context given there governs the refresh
// requests for the lifetime of the Account, so it should not be a short-lived
// per-request context.
func NewFromTokenSource(source oauth2.TokenSource, userAgent string, options ...Option) (*Account, error) {
	if source == nil {
		return nil, fmt.Errorf("client provided a nil TokenSource")
	}
	token, err := source.Token()
	if err != nil {
		return nil, fmt.Errorf("could not obtain an initial OAuth token: %w", err)
	}
	// WithTokenSource goes first so that it can still be overridden by an
	// explicit option from the caller.
	options = append([]Option{WithTokenSource(source)}, options...)
	return New(token.AccessToken, userAgent, options...)
}

// GetVehicle returns the Vehicle belonging to the account with the provided vin.
//
// Providing a nil privateKey is allowed, but a privateKey is required for most Vehicle
// interactions. Typically, the privateKey will only be nil when connecting to the Vehicle to send
// an AddKeyRequest; see documentation in [pkg/github.com/teslamotors/vehicle-command/pkg/vehicle]. The
// sessions parameter may also be nil, but providing a cache.SessionCache avoids a round-trip
// handshake with the Vehicle in subsequent connections.
func (a *Account) GetVehicle(_ context.Context, vin string, privateKey authentication.ECDHPrivateKey, sessions *cache.SessionCache) (*vehicle.Vehicle, error) {
	conn := inet.NewConnection(vin, a.authHeader, a.Host, a.UserAgent, inet.WithClient(a.client))
	car, err := vehicle.NewVehicle(conn, privateKey, sessions)
	if err != nil {
		conn.Close()
	}
	return car, err
}

// Get sends an HTTP GET request to endpoint.
//
// The endpoint should contain only the path (e.g., "api/1/vehicles/foo"); the domain is determined
// by the a.Host.
func (a *Account) Get(ctx context.Context, endpoint string) ([]byte, error) {
	url := fmt.Sprintf("https://%s/%s", a.Host, endpoint)
	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error constructing request to %s: %w", endpoint, err)
	}
	log.Debug("Requesting %s...", url)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", a.UserAgent)
	// An empty header means the Authorization header is supplied by the
	// client's Transport; see Account.applyTokenSource.
	if a.authHeader != "" {
		request.Header.Set("Authorization", a.authHeader)
	}
	response, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("error fetching %s: %w", endpoint, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		err := fmt.Errorf("http error when sending command to %s: %s", url, response.Status)
		return nil, err
	}
	reader := io.LimitedReader{R: response.Body, N: connector.MaxResponseLength}
	body, err := io.ReadAll(&reader)
	if err != nil {
		return nil, err
	}
	log.Debug("Received: %s\n", body)
	return body, err
}

func (a *Account) sendFleetAPICommand(ctx context.Context, endpoint string, command interface{}) ([]byte, error) {
	return inet.SendFleetAPICommand(ctx, a.client, a.UserAgent, a.authHeader, fmt.Sprintf("https://%s/%s", a.Host, endpoint), command)
}

// Post sends an HTTP POST request to endpoint.
//
// The endpoint should contain only the path (e.g., "api/1/vehicles/foo"); the domain is determined
// by the ServerConfig used to create the Account. Returns the HTTP body of the response.
func (a *Account) Post(ctx context.Context, endpoint string, data []byte) ([]byte, error) {
	return a.sendFleetAPICommand(ctx, endpoint, data)
}

// SendVehicleFleetAPICommand sends a command to a vehicle through the REST API.
//
// The command must support JSON serialization.
func (a *Account) SendVehicleFleetAPICommand(ctx context.Context, vin, endpoint string, command interface{}) ([]byte, error) {
	endpoint = fmt.Sprintf("api/1/vehicles/%s/%s", vin, endpoint)
	return a.sendFleetAPICommand(ctx, endpoint, command)
}

// UpdateKey sends metadata about a public key to Tesla's servers.
//
// Vehicles query this information when displaying the list of paired mobile devices and NFC cards
// in the vehicle's Locks screen. Only the account that first registers a public key can modify its
// metadata.
func (a *Account) UpdateKey(ctx context.Context, publicKey *ecdh.PublicKey, name string) error {
	params := map[string]string{
		"public_key": fmt.Sprintf("%02x", publicKey.Bytes()),
		"kind":       "mobile_device",
		"model":      "3rd Party Application",
		"name":       name,
		"tag":        a.UserAgent,
	}
	_, err := a.sendFleetAPICommand(ctx, "api/1/users/keys", &params)
	return err
}
