package legacy

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/canonical/tc"

	charmRuntime "github.com/SimonRichardson/charmd/internals/runtime"
)

type contextSuite struct{}

var _ = tc.InternalSuite(&contextSuite{})

// TestConfigurationIsolation checks that contexts load independent config defaults without modifying the charm.
func (*contextSuite) TestConfigurationIsolation(c *tc.C) {
	dir := charm(c, "", "")
	data := []byte("options:\n  greeting:\n    default: hello\n  unset:\n    type: string\n")
	c.Assert(os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0644), tc.IsNil)

	first, err := NewHookContext(dir, nil)
	c.Assert(err, tc.IsNil)

	second, err := NewHookContext(dir, nil)
	c.Assert(err, tc.IsNil)

	c.Check(first.ID() != second.ID(), tc.Equals, true)
	c.Check(first.ID(), tc.HasLen, 64)
	c.Check(first.Config(), tc.DeepEquals, map[string]any{"greeting": "hello", "unset": nil})
	c.Check(first.Status(), tc.Equals, charmRuntime.Status{Status: "unknown"})
	c.Check(first.Result().Status, tc.IsNil)

	first.SetStatus(charmRuntime.Status{Status: "active"})
	c.Check(second.Status().Status, tc.Equals, "unknown")

	stored, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	c.Assert(err, tc.IsNil)

	c.Check(stored, tc.DeepEquals, data)
}

// TestResultSnapshots checks that returned results and status values cannot mutate the live context or older snapshots.
func (*contextSuite) TestResultSnapshots(c *tc.C) {
	ctx, err := NewHookContext(charm(c, "", ""), nil)
	c.Assert(err, tc.IsNil)

	original := charmRuntime.Status{Status: "maintenance", Message: "installing"}
	ctx.SetStatus(original)
	first := ctx.Result()
	second := ctx.Result()
	first.Status.Message = "changed by caller"
	status := ctx.Status()
	status.Message = "changed status copy"

	c.Check(ctx.Status(), tc.Equals, original)
	c.Check(*second.Status, tc.Equals, original)

	ctx.SetStatus(charmRuntime.Status{Status: "active", Message: "ready"})
	c.Check(*second.Status, tc.Equals, original)
	c.Check(ctx.Status().Status, tc.Equals, "active")
}

// TestInvalidCharmData checks missing, malformed, and unreadable inputs without requiring privilege-dependent permissions.
func (*contextSuite) TestInvalidCharmData(c *tc.C) {
	_, err := NewHookContext(c.MkDir(), nil)
	c.Check(os.IsNotExist(errors.Unwrap(err)), tc.Equals, true)

	for _, metadata := range []string{"name: [", "summary: missing-name\n"} {
		dir := charm(c, "", "")
		c.Assert(os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte(metadata), 0644), tc.IsNil)

		ctx, err := NewHookContext(dir, nil)
		c.Check(err, tc.NotNil)
		c.Check(ctx, tc.IsNil)
	}

	dir := charm(c, "", "")
	c.Assert(os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("options: ["), 0644), tc.IsNil)

	ctx, err := NewHookContext(dir, nil)
	c.Check(err, tc.ErrorMatches, "read charm config: .*")
	c.Check(ctx, tc.IsNil)

	c.Assert(os.Remove(filepath.Join(dir, "config.yaml")), tc.IsNil)

	c.Assert(os.Mkdir(filepath.Join(dir, "config.yaml"), 0755), tc.IsNil)

	ctx, err = NewHookContext(dir, nil)
	c.Check(err, tc.NotNil)
	c.Check(ctx, tc.IsNil)
}

type errorWriter struct{ err error }

// Write reports a failed log or process output destination without consuming bytes.
func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

// TestLog checks message delivery, absent destinations, and propagation of write failures.
func (*contextSuite) TestLog(c *tc.C) {
	var output bytes.Buffer
	ctx, err := NewHookContext(charm(c, "", ""), &output)
	c.Assert(err, tc.IsNil)

	c.Check(ctx.Log("installing"), tc.IsNil)
	c.Check(output.String(), tc.Equals, "installing\n")
	c.Check(ctx.Result().Status, tc.IsNil)

	ctx.log = nil
	c.Check(ctx.Log("discarded"), tc.IsNil)

	failure := errors.New("log unavailable")
	ctx.log = errorWriter{failure}
	c.Check(ctx.Log("failed"), tc.ErrorIs, failure)
	c.Check(ctx.Result().Status, tc.IsNil)
}

// TestConfigOverridesAreIsolated verifies nested service values are copied and defaults survive.
func (*contextSuite) TestConfigOverridesAreIsolated(c *tc.C) {
	dir := charm(c, "", "")
	c.Assert(os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("options:\n  greeting:\n    default: default\n  untouched:\n    default: kept\n"), 0644), tc.IsNil)

	ctx, err := NewHookContext(dir, nil)
	c.Assert(err, tc.IsNil)

	nested := map[string]any{"value": "snapshot"}
	values := map[string]any{"greeting": nested}
	c.Assert(ctx.applyConfig(values), tc.IsNil)

	nested["value"] = "changed"
	values["greeting"] = "replacement"
	c.Check(ctx.Config(), tc.DeepEquals, map[string]any{"greeting": map[string]any{"value": "snapshot"}, "untouched": "kept"})
	other, err := NewHookContext(dir, nil)
	c.Assert(err, tc.IsNil)

	c.Check(other.Config()["greeting"], tc.Equals, "default")
}

// TestInvalidConfigLeavesDefaultsUnchanged verifies validation is atomic and rejects unsupported values.
func (*contextSuite) TestInvalidConfigLeavesDefaultsUnchanged(c *tc.C) {
	dir := charm(c, "", "")
	c.Assert(os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("options:\n  greeting:\n    default: default\n"), 0644), tc.IsNil)

	ctx, err := NewHookContext(dir, nil)
	c.Assert(err, tc.IsNil)

	for _, values := range []map[string]any{{"greeting": "changed", "unknown": "bad"}, {"greeting": make(chan int)}} {
		c.Check(ctx.applyConfig(values), tc.NotNil)
		c.Check(ctx.Config()["greeting"], tc.Equals, "default")
	}
}
