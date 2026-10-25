package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/client"
	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/cli"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/configapi"
	"github.com/SimonRichardson/charmd/internals/daemon"
	"github.com/SimonRichardson/charmd/internals/leadershipapi"
	"github.com/SimonRichardson/charmd/internals/overlord"
)

func (s *CharmdSuite) TestInstall(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, request *http.Request) {
		c.Check(request.Method, tc.Equals, http.MethodPost)
		c.Check(request.URL.Path, tc.Equals, "/v1/charms")
		var body map[string]any
		c.Assert(json.NewDecoder(request.Body).Decode(&body), tc.IsNil)

		c.Check(body, tc.DeepEquals, map[string]any{
			"action": "install",
			"sha256": "0123456789abcdef0123456789abcdef" +
				"0123456789abcdef0123456789abcdef",
		})
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, `{"status-code":202,"type":"async","change":"1"}`)
		c.Check(err, tc.IsNil)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{
		"install", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	c.Assert(err, tc.IsNil)

	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, "Change 1 created.\n")
	c.Check(s.Stderr(), tc.Equals, "")
}

func (s *CharmdSuite) TestInstallRejectsMissingOrInvalidChecksum(c *tc.C) {
	for _, args := range [][]string{
		{"install"},
		{"install", "bad"},
	} {
		_, err := cli.ParserForTest().ParseArgs(args)
		c.Check(err, tc.NotNil, tc.Commentf("args %q", args))
	}
}

func (s *CharmdSuite) TestInstallCreatesDaemonChange(c *tc.C) {
	charmClient, err := charmapi.New(config.Endpoint{Path: c.MkDir()})
	c.Assert(err, tc.IsNil)

	statePath := filepath.Join(c.MkDir(), "state.json")
	configAPI, err := configapi.New(config.Endpoint{URL: "https://config.example.test"})
	c.Assert(err, tc.IsNil)

	o, err := overlord.New(&overlord.Options{
		StatePath:     statePath,
		CharmAPI:      charmClient,
		LeadershipAPI: newLeadershipClient(c), ConfigAPI: configAPI,
	})
	c.Assert(err, tc.IsNil)

	defer o.Stop()

	server := httptest.NewServer(daemon.New(o).Handler())
	defer server.Close()
	api, err := client.New(&client.Config{BaseURL: server.URL})
	c.Assert(err, tc.IsNil)

	var stdout, stderr bytes.Buffer
	err = cli.RunWithClient(
		[]string{"install", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		api, &stdout, &stderr,
	)
	c.Assert(err, tc.IsNil)

	c.Check(stderr.String(), tc.Equals, "")
	c.Check(stdout.String(), tc.Matches, `Change [0-9]+ created\.\n`)

	st := o.State()
	st.Lock()
	defer st.Unlock()
	changes := st.Changes()
	c.Assert(changes, tc.HasLen, 1)

	c.Check(changes[0].Kind(), tc.Equals, "install-charm")
}

// newLeadershipClient supplies a required service without starting network activity.
func newLeadershipClient(c *tc.C) leadershipapi.Client {
	client, err := leadershipapi.New(config.Endpoint{URL: "https://leadership.example.test"})
	c.Assert(err, tc.IsNil)

	return client
}
