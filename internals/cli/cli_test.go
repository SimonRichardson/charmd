// Copyright (c) 2023 Canonical Ltd
// Licensed under the GNU General Public License version 3.

package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	. "gopkg.in/check.v1"

	"github.com/SimonRichardson/charmd/client"
	"github.com/SimonRichardson/charmd/internals/cli"
)

func Test(t *testing.T) { TestingT(t) }

type CharmdSuite struct {
	stdout, stderr *bytes.Buffer
	cliStatePath   string
	socket         string
	server         *httptest.Server
	oldConfig      string
}

var _ = Suite(&CharmdSuite{})

func (s *CharmdSuite) SetUpTest(c *C) {
	s.stdout, s.stderr = &bytes.Buffer{}, &bytes.Buffer{}
	cli.Stdout, cli.Stderr = s.stdout, s.stderr
	s.oldConfig = os.Getenv("XDG_CONFIG_HOME")
	config := c.MkDir()
	c.Assert(os.Setenv("XDG_CONFIG_HOME", config), IsNil)
	s.cliStatePath = filepath.Join(config, "charmd", "cli.json")
	s.socket = filepath.Join(c.MkDir(), ".charmd.socket")
	cli.SetNoticeTestClient(nil, s.socket)
}

func (s *CharmdSuite) TearDownTest(c *C) {
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
	cli.Stdout, cli.Stderr = os.Stdout, os.Stderr
	c.Assert(os.Setenv("XDG_CONFIG_HOME", s.oldConfig), IsNil)
}

func (s *CharmdSuite) Stdout() string { return s.stdout.String() }

func (s *CharmdSuite) Stderr() string { return s.stderr.String() }

func (s *CharmdSuite) RedirectClientToTestServer(handler func(http.ResponseWriter, *http.Request)) {
	s.server = httptest.NewServer(http.HandlerFunc(handler))
	api, err := client.New(&client.Config{BaseURL: s.server.URL})
	if err != nil {
		panic(err)
	}
	cli.SetNoticeTestClient(api, s.socket)
}

func (s *CharmdSuite) readCLIState(c *C) map[string]any {
	data, err := os.ReadFile(s.cliStatePath)
	c.Assert(err, IsNil)
	var all struct {
		Charmd map[string]map[string]any `json:"charmd"`
	}
	c.Assert(json.Unmarshal(data, &all), IsNil)
	return all.Charmd[s.socket]
}

func (s *CharmdSuite) writeCLIState(c *C, state map[string]any) {
	data, err := json.Marshal(map[string]any{"charmd": map[string]any{s.socket: state}})
	c.Assert(err, IsNil)
	c.Assert(os.MkdirAll(filepath.Dir(s.cliStatePath), 0700), IsNil)
	c.Assert(os.WriteFile(s.cliStatePath, data, 0600), IsNil)
}
