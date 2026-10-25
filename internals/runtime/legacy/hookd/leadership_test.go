package hookd

import "github.com/canonical/tc"

type leadershipSuite struct{}

var _ = tc.InternalSuite(&leadershipSuite{})

// TestSnapshotAndFormats verifies true and false snapshots and rejects arguments
// without changing the context or falling back to a successful false response.
func (*leadershipSuite) TestSnapshotAndFormats(c *tc.C) {
	for _, leader := range []bool{false, true} {
		ctx := &testContext{leader: leader}
		command := newIsLeader(ctx)
		expected := "false\n"
		if leader {
			expected = "true\n"
		}
		for _, args := range [][]string{nil, {"--format=json"}, {"--format", "yaml"}} {
			output, err := command.Run(args)
			c.Assert(err, tc.IsNil)

			c.Check(string(output), tc.Equals, expected)
		}
		for _, args := range [][]string{{"extra"}, {"--invalid"}, {"--format"}, {"--format=invalid"}} {
			_, err := command.Run(args)
			c.Check(err, tc.NotNil)
			c.Check(ctx.IsLeader(), tc.Equals, leader)
		}
	}
}
