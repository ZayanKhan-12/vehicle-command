package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teslamotors/vehicle-command/pkg/connector/inet"
	"github.com/teslamotors/vehicle-command/pkg/protocol"
)

func TestHTTPStatusCode(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected int
	}{
		{"vehicle asleep", inet.ErrVehicleNotAwake, http.StatusRequestTimeout},
		{"vehicle asleep wrapped", fmt.Errorf("connect: %w", inet.ErrVehicleNotAwake), http.StatusRequestTimeout},
		{"key not paired", protocol.ErrKeyNotPaired, http.StatusPreconditionFailed},
		{"key not paired wrapped", fmt.Errorf("handshake: %w", protocol.ErrKeyNotPaired), http.StatusPreconditionFailed},
		{"account disabled", inet.ErrAccountDisabled, http.StatusForbidden},
		{"account disabled wrapped", fmt.Errorf("fleet: %w", inet.ErrAccountDisabled), http.StatusForbidden},
		{"account disabled classified", &inet.AccountDisabledError{Reason: "EXCEEDED_LIMIT"}, http.StatusForbidden},
		{"generic error", errors.New("boom"), http.StatusInternalServerError},
		{"deadline exceeded", context.DeadlineExceeded, http.StatusInternalServerError},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if code := httpStatusCode(testCase.err); code != testCase.expected {
				t.Errorf("httpStatusCode(%v) = %d, want %d", testCase.err, code, testCase.expected)
			}
		})
	}
}

func TestWriteJSONErrorStatus(t *testing.T) {
	// Mapped errors reach the client with the mapped status code and the
	// error message in the JSON error field.
	recorder := httptest.NewRecorder()
	writeJSONError(recorder, httpStatusCode(inet.ErrVehicleNotAwake), inet.ErrVehicleNotAwake)
	if recorder.Code != http.StatusRequestTimeout {
		t.Errorf("vehicle-asleep status = %d, want %d", recorder.Code, http.StatusRequestTimeout)
	}
	var reply struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &reply); err != nil {
		t.Fatalf("invalid JSON body %q: %s", recorder.Body.String(), err)
	}
	if reply.Error != inet.ErrVehicleNotAwake.Error() {
		t.Errorf("error field = %q, want %q", reply.Error, inet.ErrVehicleNotAwake.Error())
	}

	// An inet.HTTPError still passes its own status code through, regardless
	// of the code computed by the caller.
	httpErr := &inet.HTTPError{Code: http.StatusTooManyRequests, Message: `{"error":"slow down"}`}
	recorder = httptest.NewRecorder()
	writeJSONError(recorder, httpStatusCode(httpErr), httpErr)
	if recorder.Code != http.StatusTooManyRequests {
		t.Errorf("HTTPError status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}

	// Account-disabled 403s keep Tesla's JSON body so proxy clients can match
	// on "EXCEEDED_LIMIT", while errors.Is still classifies the hold.
	disabledBody := `{"error":"account disabled: EXCEEDED_LIMIT"}`
	disabledErr := &inet.HTTPError{
		Code:    http.StatusForbidden,
		Message: disabledBody,
		Cause:   &inet.AccountDisabledError{Reason: "EXCEEDED_LIMIT", Body: disabledBody},
	}
	recorder = httptest.NewRecorder()
	writeJSONError(recorder, httpStatusCode(disabledErr), disabledErr)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("account-disabled status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if recorder.Body.String() != disabledBody+"\n" {
		t.Errorf("account-disabled body = %q, want Tesla payload", recorder.Body.String())
	}

	// Nominal errors still return 200 with the reason in the response body.
	recorder = httptest.NewRecorder()
	writeJSONError(recorder, http.StatusOK, &protocol.NominalError{Details: errors.New("could not execute command")})
	if recorder.Code != http.StatusOK {
		t.Errorf("nominal error status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
