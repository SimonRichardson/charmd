package configapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/canonical/tc"
)

type streamSuite struct{}

var _ = tc.InternalSuite(&streamSuite{})

// TestFrames verifies comments, multiline data, CRLF, and unrelated or incomplete events.
func (*streamSuite) TestFrames(c *tc.C) {
	checksum := strings.Repeat("b", 64)
	input := ": heartbeat\r\n\r\nevent: other\ndata: ignored\n\nevent: config-changed\nid: 1\ndata: {\ndata: \"sha256\":\"" + checksum + "\"}\n\ndata: {\"sha256\":\"" + checksum + "\"}"
	var changes []string
	err := readEvents(strings.NewReader(input), func(checksum string) { changes = append(changes, checksum) })
	c.Check(err, tc.Equals, io.EOF)
	c.Check(changes, tc.DeepEquals, []string{checksum})
}

// TestInvalidFrames rejects invalid identities, malformed JSON, and unbounded frames.
func (*streamSuite) TestInvalidFrames(c *tc.C) {
	for _, input := range []string{"data: invalid\n\n", "data: {}\n\n", "data: " + strings.Repeat("x", 1024*1024) + "\n\n", strings.Repeat("data: "+strings.Repeat("x", 1024)+"\n", 1024)} {
		calls := 0
		err := readEvents(strings.NewReader(input), func(string) { calls++ })
		c.Check(err, tc.NotNil)
		c.Check(calls, tc.Equals, 0)
	}
}

// TestWatch delivers only notification identities supplied by the service.
func (*streamSuite) TestWatch(c *tc.C) {
	checksum := strings.Repeat("c", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.URL.Path, tc.Equals, "/v1/events")
		c.Check(r.Header.Get("Accept"), tc.Equals, "text/event-stream")
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprintf(w, "event: config-changed\ndata: {\"sha256\":%q}\n\n", checksum)
	}))
	defer server.Close()
	client, err := New(config.Endpoint{URL: server.URL})
	c.Assert(err, tc.IsNil)

	var got []string
	err = client.Watch(context.Background(), func(checksum string) { got = append(got, checksum) })
	c.Check(err, tc.Equals, io.EOF)
	c.Check(got, tc.DeepEquals, []string{checksum})
}

// TestWatchCancellation closes an idle stream promptly without dispatching partial events.
func (*streamSuite) TestWatchCancellation(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"sha256\":%q}\n\n", strings.Repeat("a", 64))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := New(config.Endpoint{URL: server.URL})
	c.Assert(err, tc.IsNil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Watch(ctx, func(string) { cancel() }) }()
	select {
	case err := <-done:
		c.Check(err, tc.NotNil)
	case <-time.After(5 * time.Second):
		c.Fatal("watch did not stop")
	}
}

// TestWatchRejectsNonStream verifies HTTP errors and unexpected content cannot trigger reconciliation.
func (*streamSuite) TestWatchRejectsNonStream(c *tc.C) {
	for _, code := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			fmt.Fprint(w, "{}")
		}))
		client, err := New(config.Endpoint{URL: server.URL})
		c.Assert(err, tc.IsNil)

		calls := 0
		err = client.Watch(context.Background(), func(string) { calls++ })
		c.Check(err, tc.NotNil)
		c.Check(calls, tc.Equals, 0)
		server.Close()
	}
}
