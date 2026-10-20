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

func requestUser(r *http.Request) *UserState {
	user, _ := r.Context().Value(userKey{}).(*UserState)
	return user
}

// WithUser attaches an authenticated identity to a request context.
func WithUser(ctx context.Context, user *UserState) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

func parseOptionalTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func parseOptionalDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

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
