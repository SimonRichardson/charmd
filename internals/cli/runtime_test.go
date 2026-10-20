package cli

import (
	"os"
	"testing"

	"github.com/canonical/tc"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type runtimeSuite struct {
	oldCharmd string
}

var _ = tc.InternalSuite(&runtimeSuite{})

func (s *runtimeSuite) SetUpTest(c *tc.C) {
	s.oldCharmd = os.Getenv(charmdEnv)
}

func (s *runtimeSuite) TearDownTest(c *tc.C) {
	c.Assert(os.Setenv(charmdEnv, s.oldCharmd), tc.IsNil)
}

func (s *runtimeSuite) TestRuntimePathsUseCharmdDirectory(c *tc.C) {
	c.Assert(os.Setenv(charmdEnv, "/var/lib/charmd/custom"), tc.IsNil)
	c.Check(runtimePaths(), tc.DeepEquals, runtimePathSet{
		dir:    "/var/lib/charmd/custom",
		config: "/var/lib/charmd/custom/config.yaml",
		socket: "/var/lib/charmd/custom/charmd.sock",
		state:  "/var/lib/charmd/custom/state.json",
	})
}

func (s *runtimeSuite) TestRuntimePathsUseDefaultDirectory(c *tc.C) {
	c.Assert(os.Setenv(charmdEnv, ""), tc.IsNil)
	c.Check(runtimePaths(), tc.DeepEquals, runtimePathSet{
		dir:    defaultCharmdDir,
		config: defaultCharmdDir + "/config.yaml",
		socket: defaultCharmdDir + "/charmd.sock",
		state:  defaultCharmdDir + "/state.json",
	})
}
