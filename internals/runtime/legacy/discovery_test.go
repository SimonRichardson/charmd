package legacy

import (
	"os"
	"path/filepath"

	"github.com/canonical/tc"
)

type discoverySuite struct{}

var _ = tc.InternalSuite(&discoverySuite{})

// TestDiscoveryPrecedence checks dispatch priority, hook fallback, and missing-hook behavior without executing handlers.
func (*discoverySuite) TestDiscoveryPrecedence(c *tc.C) {
	dir := charm(c, "exit 42", "hooks/install")
	path, err := discoverHook(dir, "install")
	c.Assert(err, tc.IsNil)

	c.Check(path, tc.Equals, filepath.Join(dir, "hooks/install"))
	c.Assert(os.WriteFile(filepath.Join(dir, "dispatch"), []byte("#!/bin/sh\nexit 17\n"), 0755), tc.IsNil)

	path, err = discoverHook(dir, "install")
	c.Assert(err, tc.IsNil)

	c.Check(path, tc.Equals, filepath.Join(dir, "dispatch"))
	c.Assert(os.Remove(filepath.Join(dir, "dispatch")), tc.IsNil)

	path, err = discoverHook(dir, "start")
	c.Assert(err, tc.IsNil)

	c.Check(path, tc.Equals, "")
	// Discovery must leave an existing hook's contents and permissions unchanged.
	data, err := os.ReadFile(filepath.Join(dir, "hooks/install"))
	c.Assert(err, tc.IsNil)

	c.Check(string(data), tc.Equals, "#!/bin/sh\nset -eu\nexit 42")
}

// TestInvalidHandlerNeverFallsBack checks that invalid dispatch files fail instead of silently running another handler.
func (*discoverySuite) TestInvalidHandlerNeverFallsBack(c *tc.C) {
	for _, directory := range []bool{false, true} {
		dir := charm(c, "exit 0", "hooks/install")
		if directory {
			c.Assert(os.Mkdir(filepath.Join(dir, "dispatch"), 0755), tc.IsNil)
		} else {
			c.Assert(os.WriteFile(filepath.Join(dir, "dispatch"), []byte("not executable"), 0644), tc.IsNil)
		}

		path, err := discoverHook(dir, "install")
		c.Check(err, tc.ErrorMatches, `hook handler "dispatch" is not executable`)
		c.Check(path, tc.Equals, "")
	}
}

// TestNonExecutableHook checks that a present hook without execute permission is an error.
func (*discoverySuite) TestNonExecutableHook(c *tc.C) {
	dir := charm(c, "exit 0", "hooks/install")
	c.Assert(os.Chmod(filepath.Join(dir, "hooks/install"), 0644), tc.IsNil)

	path, err := discoverHook(dir, "install")
	c.Check(err, tc.ErrorMatches, `hook handler "hooks/install" is not executable`)
	c.Check(path, tc.Equals, "")
}
