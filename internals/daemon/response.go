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
	// ResponseTypeError indicates an API error.
	ResponseTypeError ResponseType = "error"
)

// Response writes itself to an HTTP response.
type Response interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}
type response struct {
	Status     int          `json:"status-code"`
	Type       ResponseType `json:"type"`
	StatusText string       `json:"status,omitempty"`
	Result     any          `json:"result,omitempty"`
}

func (r *response) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.Status)
	_ = json.NewEncoder(w).Encode(r)
}

// SyncResponse creates a successful synchronous API response.
func SyncResponse(result any) Response {
	return &response{Status: http.StatusOK, Type: ResponseTypeSync, Result: result}
}

type errorResponder func(string, ...any) Response

func (e errorResponder) ServeHTTP(http.ResponseWriter, *http.Request) {}
func (e errorResponder) call(format string, args ...any) Response     { return e(format, args...) }
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
