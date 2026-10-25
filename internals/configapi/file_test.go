package configapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/canonical/tc"
)

type fileSuite struct{}

var _ = tc.InternalSuite(&fileSuite{})

// TestGetLocalConfig verifies source selection, default restoration, and independent snapshots.
func (*fileSuite) TestGetLocalConfig(c *tc.C) {
	checksum := strings.Repeat("a", 64)
	path := filepath.Join(c.MkDir(), "config.yaml")
	body := "charms:\n  " + checksum + ":\n    greeting: hello\n    count: 2\n"
	c.Assert(os.WriteFile(path, []byte(body), 0644), tc.IsNil)

	client, err := New(config.Endpoint{Path: path})
	c.Assert(err, tc.IsNil)

	first, err := client.Get(context.Background(), checksum)
	c.Assert(err, tc.IsNil)

	c.Check(first, tc.DeepEquals, map[string]any{"greeting": "hello", "count": float64(2)})
	first["greeting"] = "mutated"
	second, err := client.Get(context.Background(), checksum)
	c.Assert(err, tc.IsNil)

	c.Check(second["greeting"], tc.Equals, "hello")
	missing, err := client.Get(context.Background(), strings.Repeat("b", 64))
	c.Assert(err, tc.IsNil)

	c.Check(missing, tc.DeepEquals, map[string]any{})
	unchanged, err := os.ReadFile(path)
	c.Assert(err, tc.IsNil)

	c.Check(string(unchanged), tc.Equals, body)
}

// TestInvalidFiles verifies missing files, malformed identities, oversized content,
// unsupported values, and unknown schema fields cannot become config snapshots.
func (*fileSuite) TestInvalidFiles(c *tc.C) {
	path := filepath.Join(c.MkDir(), "config.yaml")
	client := &fileClient{path: path}
	_, err := client.Get(context.Background(), strings.Repeat("a", 64))
	c.Check(err, tc.NotNil)
	for _, body := range []string{"charms: [", "unknown: true\n", "charms:\n  invalid: {}\n", "charms: {}\n---\ncharms: {}\n", "charms:\n  " + strings.Repeat("a", 64) + ":\n    value: .nan\n", strings.Repeat(" ", 4*1024*1024+1)} {
		c.Assert(os.WriteFile(path, []byte(body), 0644), tc.IsNil)

		_, err := client.Get(context.Background(), strings.Repeat("a", 64))
		c.Check(err, tc.NotNil)
	}
	_, err = client.Get(context.Background(), "../bad")
	c.Check(err, tc.NotNil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Get(ctx, strings.Repeat("a", 64))
	c.Check(err, tc.Equals, context.Canceled)
	_, err = New(config.Endpoint{Path: path, URL: "http://example.test"})
	c.Check(err, tc.NotNil)
}

// TestEmptyOverrides verifies null charm entries and an empty mapping restore defaults.
func (*fileSuite) TestEmptyOverrides(c *tc.C) {
	path := filepath.Join(c.MkDir(), "config.yaml")
	checksum := strings.Repeat("a", 64)
	client := &fileClient{path: path}
	for _, body := range []string{"charms:\n  " + checksum + ": null\n", "charms: {}\n"} {
		c.Assert(os.WriteFile(path, []byte(body), 0644), tc.IsNil)

		values, err := client.Get(context.Background(), checksum)
		c.Assert(err, tc.IsNil)

		c.Check(values, tc.DeepEquals, map[string]any{})
	}
}
