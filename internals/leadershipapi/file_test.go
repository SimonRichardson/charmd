package leadershipapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/canonical/tc"
)

type fileSuite struct{}

var _ = tc.InternalSuite(&fileSuite{})

// TestSnapshots verifies explicit true, false and removal, independent reads,
// cancellation and malformed snapshots. Invalid data never grants leadership.
func (*fileSuite) TestSnapshots(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	path := filepath.Join(c.MkDir(), "leadership.yaml")
	client := &fileClient{path: path}
	for _, test := range []struct {
		body   string
		leader bool
	}{
		{"charms:\n  " + checksum + ": true\n", true},
		{"charms:\n  " + checksum + ": false\n", false},
		{"charms: {}\n", false},
	} {
		c.Assert(os.WriteFile(path, []byte(test.body), 0644), tc.IsNil)

		leader, err := client.Get(context.Background(), checksum)
		c.Assert(err, tc.IsNil)

		c.Check(leader, tc.Equals, test.leader)
	}
	for _, body := range []string{"charms: [", "charms: {invalid: true}", "unknown: true", "charms: {}\n---\ncharms: {}", "charms: {" + checksum + ": null}", "charms: {" + checksum + ": maybe}", strings.Repeat("x", 4*1024*1024+1)} {
		c.Assert(os.WriteFile(path, []byte(body), 0644), tc.IsNil)

		_, err := client.Get(context.Background(), checksum)
		c.Check(err, tc.NotNil)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Get(ctx, checksum)
	c.Check(err, tc.Equals, context.Canceled)
	_, err = client.Get(context.Background(), "invalid")
	c.Check(err, tc.NotNil)
	c.Assert(os.Remove(path), tc.IsNil)

	_, err = client.Get(context.Background(), checksum)
	c.Check(err, tc.NotNil)
}
