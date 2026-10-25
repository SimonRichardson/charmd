package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ResponseType identifies the kind of API response.
type ResponseType string

const (
	// ResponseTypeSync indicates a successful synchronous result.
	ResponseTypeSync ResponseType = "sync"
	// ResponseTypeAsync indicates a successfully accepted asynchronous operation.
	ResponseTypeAsync ResponseType = "async"
	// ResponseTypeError indicates an API error.
	ResponseTypeError ResponseType = "error"
)

// Response writes itself to an HTTP response.
type Response interface {
	// ServeHTTP writes the response status, headers, and body.
	ServeHTTP(http.ResponseWriter, *http.Request)
}
type response struct {
	Status     int          `json:"status-code"`
	Type       ResponseType `json:"type"`
	StatusText string       `json:"status,omitempty"`
	Result     any          `json:"result,omitempty"`
	Change     string       `json:"change,omitempty"`
}

// ServeHTTP implements the HTTP response contract for this responder.
func (r *response) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.Status)
	_ = json.NewEncoder(w).Encode(r)
}

// SyncResponse creates a successful synchronous API response.
func SyncResponse(result any) Response {
	return &response{Status: http.StatusOK, Type: ResponseTypeSync, Result: result}
}

// AsyncResponse creates a response for an accepted asynchronous change.
func AsyncResponse(change string) Response {
	return &response{Status: http.StatusAccepted, Type: ResponseTypeAsync, Change: change}
}

type errorResponder func(string, ...any) Response

// ServeHTTP satisfies Response for the error factory; call constructs the actual response.
func (e errorResponder) ServeHTTP(http.ResponseWriter, *http.Request) {}

// call formats an error through the response factory.
func (e errorResponder) call(format string, args ...any) Response { return e(format, args...) }

// makeErrorResponder creates a response factory for one HTTP error status.
func makeErrorResponder(status int) errorResponder {
	return func(format string, args ...any) Response {
		return &response{
			Status:     status,
			Type:       ResponseTypeError,
			StatusText: http.StatusText(status),
			Result:     map[string]string{"message": fmt.Sprintf(format, args...)},
		}
	}
}

var (
	BadRequest       = makeErrorResponder(http.StatusBadRequest)
	Forbidden        = makeErrorResponder(http.StatusForbidden)
	NotFound         = makeErrorResponder(http.StatusNotFound)
	MethodNotAllowed = makeErrorResponder(http.StatusMethodNotAllowed)
	GatewayTimeout   = makeErrorResponder(http.StatusGatewayTimeout)
	ServerError      = makeErrorResponder(http.StatusInternalServerError)
)
