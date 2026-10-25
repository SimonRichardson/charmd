package leadershipapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/canonical/tc"
)

type streamSuite struct{}

var _ = tc.InternalSuite(&streamSuite{})

// TestFrames verifies complete notifications only, including unrelated events,
// partial disconnects and malformed or oversized frames.
func (*streamSuite) TestFrames(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	frame := fmt.Sprintf("event: leadership-changed\ndata: {\"sha256\":%q}\n\n", checksum)
	var events []string
	err := readEvents(strings.NewReader(": heartbeat\n\nevent: unrelated\ndata: invalid\n\n"+frame+frame+"data: {"), func(sha string) { events = append(events, sha) })
	c.Check(err, tc.Equals, io.EOF)
	c.Check(events, tc.DeepEquals, []string{checksum, checksum})
	for _, input := range []string{"data: {}\n\n", "data: {\n\n", "data: " + strings.Repeat("x", 1024*1024+1) + "\n\n"} {
		err := readEvents(strings.NewReader(input), func(string) { c.Error("invalid event emitted") })
		c.Check(err, tc.NotNil)
	}
}

// TestWatchContract verifies the HTTP stream endpoint, media type and cancellation.
func (*streamSuite) TestWatchContract(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	mediaType := "text/event-stream"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.URL.Path, tc.Equals, "/v1/events")
		c.Check(r.Header.Get("Accept"), tc.Equals, "text/event-stream")
		w.Header().Set("Content-Type", mediaType)
		fmt.Fprintf(w, "data: {\"sha256\":%q}\n\n", checksum)
	}))
	defer server.Close()
	client, err := New(config.Endpoint{URL: server.URL})
	c.Assert(err, tc.IsNil)

	var events []string
	err = client.Watch(context.Background(), func(sha string) { events = append(events, sha) })
	c.Check(err, tc.Equals, io.EOF)
	c.Check(events, tc.DeepEquals, []string{checksum})
	mediaType = "application/json"
	c.Check(client.Watch(context.Background(), func(string) {}), tc.ErrorMatches, ".*SSE stream")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Check(client.Watch(ctx, func(string) {}), tc.ErrorIs, context.Canceled)
}
