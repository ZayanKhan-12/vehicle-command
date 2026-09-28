package inet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/teslamotors/vehicle-command/internal/log"
	"github.com/teslamotors/vehicle-command/pkg/connector"
	"github.com/teslamotors/vehicle-command/pkg/protocol"
)

// MaxLatency is the default maximum latency permitted when updating the vehicle clock estimate.
var MaxLatency = 10 * time.Second

func ReadWithContext(ctx context.Context, r io.Reader, p []byte) ([]byte, error) {
	bytesRead := 0
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		n, err := r.Read(p[bytesRead:])
		bytesRead += n
		if err == io.EOF {
			return p[:bytesRead], nil
		}
		if err != nil {
			return p[:bytesRead], err
		}
		if bytesRead == len(p) {
			return p[:bytesRead], nil
		}
	}
}

var ErrVehicleNotAwake = protocol.NewError("vehicle unavailable: vehicle is offline or asleep", false, false)

/*
The regular expression below extracts domains from HTTP bodies:

	{
	  "response": null,
	  "error": "user out of region, use base URL: https://fleet-api.prd.na.vn.cloud.tesla.com, see https://...",
	  "error_description": ""
	}
*/
var baseDomainRE = regexp.MustCompile(`use base URL: https://([-a-z0-9.]*)`)

type HTTPError struct {
	Code    int
	Message string
	// Cause is an optional classified error unwrapped by errors.As / errors.Is,
	// for example [*AccountDisabledError].
	Cause error
}

func (e *HTTPError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Message == "" {
		return http.StatusText(e.Code)
	}
	return e.Message
}

func (e *HTTPError) MayHaveSucceeded() bool {
	if e.Code >= 400 && e.Code < 500 {
		return false
	}
	return e.Code != http.StatusServiceUnavailable
}

func (e *HTTPError) Temporary() bool {
	return e.Code == http.StatusServiceUnavailable ||
		e.Code == http.StatusGatewayTimeout ||
		e.Code == http.StatusRequestTimeout ||
		e.Code == http.StatusMisdirectedRequest
}

func (e *HTTPError) Unwrap() error {
	return e.Cause
}

// ErrAccountDisabled is returned when Fleet API rejects a request because Tesla
// has disabled the developer account. This is an account/billing condition,
// not a vehicle or protocol failure. The client cannot recover except by Tesla
// restoring the account (typically after a payment method is added at
// https://developer.tesla.com). See teslamotors/vehicle-command#403.
var ErrAccountDisabled = protocol.NewError("fleet API account disabled", false, false)

// AccountDisabledError is a classified Fleet API 403 whose body reports
// "account disabled", optionally with a reason such as EXCEEDED_LIMIT.
type AccountDisabledError struct {
	// Reason is the Tesla-supplied suffix, for example "EXCEEDED_LIMIT".
	Reason string
	// Body is the raw HTTP response body.
	Body string
}

func (e *AccountDisabledError) Error() string {
	msg := "fleet API account disabled"
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg + "; Tesla has placed a billing hold on the developer account. " +
		"This library cannot lift it. Add a supported payment method at https://developer.tesla.com " +
		"or contact Tesla developer support. Retrying the command will keep returning 403"
}

func (e *AccountDisabledError) Unwrap() error {
	return ErrAccountDisabled
}

func (e *AccountDisabledError) MayHaveSucceeded() bool {
	return false
}

func (e *AccountDisabledError) Temporary() bool {
	return false
}

func parseAccountDisabled(body []byte) *AccountDisabledError {
	var payload struct {
		Error string `json:"error"`
	}
	text := strings.TrimSpace(string(body))
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		text = payload.Error
	}
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "account disabled") {
		return nil
	}
	reason := ""
	if _, suffix, ok := strings.Cut(text, ":"); ok {
		reason = strings.TrimSpace(suffix)
	}
	return &AccountDisabledError{Reason: reason, Body: string(body)}
}

func SendFleetAPICommand(ctx context.Context, client *http.Client, userAgent, authHeader string, url string, command interface{}) ([]byte, error) {
	var body []byte
	var ok bool
	if body, ok = command.([]byte); !ok {
		var err error
		body, err = json.Marshal(command)
		if err != nil {
			return nil, err
		}
	}
	log.Debug("Sending request to %s: %s", url, body)
	request, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &protocol.CommandError{Err: err, PossibleSuccess: false, PossibleTemporary: true}
	}

	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Content-type", "application/json")
	request.Header.Set("Authorization", authHeader)
	request.Header.Set("Accept", "*/*")

	result, err := client.Do(request)
	if err != nil {
		return nil, &protocol.CommandError{Err: err, PossibleSuccess: false, PossibleTemporary: true}
	}
	defer func() {
		_ = result.Body.Close()
	}()

	body = make([]byte, connector.MaxResponseLength+1)
	body, err = ReadWithContext(ctx, result.Body, body)
	if err != nil {
		return nil, &protocol.CommandError{Err: err, PossibleSuccess: true, PossibleTemporary: false}
	}

	if len(body) == connector.MaxResponseLength+1 {
		return nil, protocol.NewError("response exceeds maximum length", true, true)
	}

	log.Debug("Server returned %d: %s: %s", result.StatusCode, http.StatusText(result.StatusCode), body)
	switch result.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnprocessableEntity: // HTTP: 422 on commands endpoint means protocol is not supported (fallback to regular commands)
		return nil, protocol.ErrProtocolNotSupported
	case http.StatusServiceUnavailable:
		return nil, ErrVehicleNotAwake
	case http.StatusRequestTimeout:
		if bytes.Contains(body, []byte("vehicle is offline")) {
			return nil, ErrVehicleNotAwake
		}
	}
	httpErr := &HTTPError{Code: result.StatusCode, Message: string(body)}
	if result.StatusCode == http.StatusForbidden {
		if acc := parseAccountDisabled(body); acc != nil {
			httpErr.Cause = acc
		}
	}
	return nil, httpErr
}

func ValidTeslaDomainSuffix(domain string) bool {
	return strings.HasSuffix(domain, ".tesla.com") || strings.HasSuffix(domain, ".tesla.cn") || strings.HasSuffix(domain, ".teslamotors.com")
}

// Sends a command to a Fleet API REST endpoint. Returns the response body and an error. The
// response body is not necessarily nil if the error is set.
func (c *Connection) SendFleetAPICommand(ctx context.Context, endpoint string, command interface{}) ([]byte, error) {
	url := fmt.Sprintf("https://%s/%s", c.serverURL, endpoint)
	rsp, err := SendFleetAPICommand(ctx, c.client, c.UserAgent, c.authHeader, url, command)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.Code == http.StatusMisdirectedRequest {
			matches := baseDomainRE.FindStringSubmatch(httpErr.Message)
			if len(matches) == 2 && ValidTeslaDomainSuffix(matches[1]) {
				log.Debug("Received HTTP Status 421. Updating server URL.")
				c.serverURL = matches[1]
			}
		}
	}
	return rsp, err
}

// Connection implements the connector.Connector interface by POSTing commands to a server.
type Connection struct {
	UserAgent  string
	vin        string
	client     *http.Client
	serverURL  string
	inbox      chan []byte
	authHeader string

	lock     sync.Mutex
	lastPoke time.Time
}

// NewConnection creates a Connection.
func NewConnection(vin string, authHeader, serverURL, userAgent string) *Connection {
	conn := Connection{
		UserAgent:  userAgent,
		vin:        vin,
		client:     &http.Client{},
		serverURL:  serverURL,
		authHeader: authHeader,
		inbox:      make(chan []byte, connector.BufferSize),
	}
	return &conn
}

func (c *Connection) PreferredAuthMethod() connector.AuthMethod {
	return connector.AuthMethodHMAC
}

func (c *Connection) AllowedLatency() time.Duration {
	return MaxLatency
}

func (c *Connection) RetryInterval() time.Duration {
	return time.Second
}

func (c *Connection) Receive() <-chan []byte {
	return c.inbox
}

func (c *Connection) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.inbox != nil {
		close(c.inbox)
		c.inbox = nil
	}
}

func (c *Connection) VIN() string {
	return c.vin
}

func (c *Connection) Wakeup(ctx context.Context) error {
	type wakeResponse struct {
		State string `json:"state"`
	}
	var response struct {
		WakeResponse wakeResponse `json:"response"`
	}

	for {
		c.lock.Lock()
		c.lastPoke = time.Now()
		c.lock.Unlock()
		endpoint := fmt.Sprintf("api/1/vehicles/%s/wake_up", c.vin)
		respJSON, err := c.SendFleetAPICommand(ctx, endpoint, nil)
		if err == nil {
			err = json.Unmarshal(respJSON, &response)
			if err == nil && response.WakeResponse.State == "online" {
				return nil
			}
		}

		if !protocol.Temporary(err) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			continue
		}
	}
}

func (c *Connection) Send(ctx context.Context, buffer []byte) error {
	type cmd struct {
		Payload []byte `json:"routable_message"`
	}

	endpoint := fmt.Sprintf("api/1/vehicles/%s/signed_command", c.vin)
	body, err := c.SendFleetAPICommand(ctx, endpoint, cmd{Payload: buffer})
	if err != nil {
		return err
	}

	type jsonResponse struct {
		Payload []byte `json:"response"`
	}

	var rsp jsonResponse
	if err := json.Unmarshal(body, &rsp); err != nil {
		log.Debug("Invalid server response (%d bytes): %s", len(body), body)
		return &protocol.CommandError{Err: fmt.Errorf("unable to parse server response: %w", err), PossibleSuccess: true, PossibleTemporary: false}
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.inbox == nil {
		return protocol.ErrNotConnected
	}
	select {
	case c.inbox <- rsp.Payload:
		return nil
	default:
		return protocol.NewError("dropped response because inbox is full", true, false)
	}
}
