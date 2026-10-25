package daemon

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type userKey struct{}

// UserState describes the identity associated with a daemon connection.
type UserState struct {
	UID   *uint32
	Admin bool
}

// requestUser retrieves the authenticated identity attached to a request.
func requestUser(r *http.Request) *UserState {
	user, _ := r.Context().Value(userKey{}).(*UserState)
	return user
}

// WithUser attaches an authenticated identity to a request context.
func WithUser(ctx context.Context, user *UserState) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

// parseOptionalTime parses an RFC3339 timestamp or returns zero for an empty value.
func parseOptionalTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// parseOptionalDuration parses a duration or returns zero for an empty value.
func parseOptionalDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

// multiCommaSeparatedList flattens repeated comma-separated query values and removes empty entries.
func multiCommaSeparatedList(values []string) []string {
	var result []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}
