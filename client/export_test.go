// Copyright (c) 2023 Canonical Ltd
// Licensed under the GNU General Public License version 3.

package client

import (
	"net/http"
	"time"
)

// SetDoer replaces the HTTP transport for upstream client tests.
func (client *Client) SetDoer(doer interface {
	Do(*http.Request) (*http.Response, error)
}) {
	client.requester.(*defaultRequester).doer = doer
}

// FakeDoRetry fakes the delays used by the retry loop for tests.
func FakeDoRetry(retry, timeout time.Duration) func() {
	oldRetry, oldTimeout := doRetry, doTimeout
	doRetry, doTimeout = retry, timeout
	return func() { doRetry, doTimeout = oldRetry, oldTimeout }
}
