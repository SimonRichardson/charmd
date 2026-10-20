// Copyright (c) 2023 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cli_test

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/cli"
)

func (s *CharmdSuite) TestNotices(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{})

		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": [{
				"id": "1",
				"user-id": 1000,
				"type": "custom",
				"key": "a.b/c",
				"first-occurred": "2023-09-05T17:18:00Z",
				"last-occurred": "2023-09-05T19:18:00Z",
				"last-repeated": "2023-09-05T18:18:00Z",
				"occurrences": 3
			}, {
				"id": "2",
				"user-id": null,
				"type": "warning",
				"key": "Ware!",
				"first-occurred": "2023-09-06T17:18:00Z",
				"last-occurred": "2023-09-06T19:18:00Z",
				"last-repeated": "2023-09-06T18:18:00Z",
				"occurrences": 1
			}
		]}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"notices", "--abs-time"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, `
ID   User    Type     Key    First                 Repeated              Occurrences
1    1000    custom   a.b/c  2023-09-05T17:18:00Z  2023-09-05T18:18:00Z  3
2    public  warning  Ware!  2023-09-06T17:18:00Z  2023-09-06T18:18:00Z  1
`[1:])
	c.Check(s.Stderr(), tc.Equals, "")

	cliState := s.readNoticesCLIState(c)
	c.Check(cliState, tc.DeepEquals, map[string]any{
		"notices-last-listed": "2023-09-06T18:18:00Z",
		"notices-last-okayed": "0001-01-01T00:00:00Z",
	})
}

func (s *CharmdSuite) readNoticesCLIState(c *tc.C) map[string]any {
	fullCLIState := s.readCLIState(c)
	cliState := map[string]any{
		"notices-last-listed": fullCLIState["notices-last-listed"],
		"notices-last-okayed": fullCLIState["notices-last-okayed"],
	}
	return cliState
}

func (s *CharmdSuite) TestNoticesFiltersUsers(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{
			"users": {"all"},
			"types": {"custom", "warning"},
			"keys":  {"a.b/c"},
		})

		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": [{
				"id": "1",
				"user-id": 1000,
				"type": "custom",
				"key": "a.b/c",
				"first-occurred": "2023-09-05T17:18:00Z",
				"last-occurred": "2023-09-05T19:18:00Z",
				"last-repeated": "2023-09-05T18:18:00Z",
				"occurrences": 3
			}
		]}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{
		"notices", "--abs-time", "--users", "all", "--type", "custom", "--key", "a.b/c", "--type", "warning"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, `
ID   User  Type    Key    First                 Repeated              Occurrences
1    1000  custom  a.b/c  2023-09-05T17:18:00Z  2023-09-05T18:18:00Z  3
`[1:])
	c.Check(s.Stderr(), tc.Equals, "")

	cliState := s.readNoticesCLIState(c)
	c.Check(cliState, tc.DeepEquals, map[string]any{
		"notices-last-listed": "2023-09-05T18:18:00Z",
		"notices-last-okayed": "0001-01-01T00:00:00Z",
	})
}

func (s *CharmdSuite) TestNoticesFiltersUID(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{
			"user-id": {"1000"},
			"types":   {"custom", "warning"},
			"keys":    {"a.b/c"},
		})

		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": [{
				"id": "1",
				"user-id": 1000,
				"type": "custom",
				"key": "a.b/c",
				"first-occurred": "2023-09-05T17:18:00Z",
				"last-occurred": "2023-09-05T19:18:00Z",
				"last-repeated": "2023-09-05T18:18:00Z",
				"occurrences": 3
			}
		]}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{
		"notices", "--abs-time", "--uid", "1000", "--type", "custom", "--key", "a.b/c", "--type", "warning"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, `
ID   User  Type    Key    First                 Repeated              Occurrences
1    1000  custom  a.b/c  2023-09-05T17:18:00Z  2023-09-05T18:18:00Z  3
`[1:])
	c.Check(s.Stderr(), tc.Equals, "")

	cliState := s.readNoticesCLIState(c)
	c.Check(cliState, tc.DeepEquals, map[string]any{
		"notices-last-listed": "2023-09-05T18:18:00Z",
		"notices-last-okayed": "0001-01-01T00:00:00Z",
	})
}

func (s *CharmdSuite) TestNoticesAfter(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{
			"after": {"2023-08-04T01:02:03Z"}, // from "notices-last-okayed" in notices.json
		})

		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": [{
				"id": "1",
				"user-id": 1000,
				"type": "custom",
				"key": "a.b/c",
				"first-occurred": "2023-09-05T17:18:00Z",
				"last-occurred": "2023-09-05T19:18:00Z",
				"last-repeated": "2023-09-07T18:18:00Z",
				"occurrences": 3
			}
		]}`)
	})

	s.writeCLIState(c, map[string]any{
		"notices-last-listed": time.Date(2023, 9, 6, 15, 6, 0, 0, time.UTC),
		"notices-last-okayed": time.Date(2023, 8, 4, 1, 2, 3, 0, time.UTC),
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"notices", "--abs-time"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, `
ID   User  Type    Key    First                 Repeated              Occurrences
1    1000  custom  a.b/c  2023-09-05T17:18:00Z  2023-09-07T18:18:00Z  3
`[1:])
	c.Check(s.Stderr(), tc.Equals, "")

	cliState := s.readNoticesCLIState(c)
	c.Check(cliState, tc.DeepEquals, map[string]any{
		"notices-last-listed": "2023-09-07T18:18:00Z",
		"notices-last-okayed": "2023-08-04T01:02:03Z",
	})
}

func (s *CharmdSuite) TestNoticesNoNotices(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{})
		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": []}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"notices"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, "")
	c.Check(s.Stderr(), tc.Equals, "No matching notices.\n")

	// Shouldn't have updated cli.json
	_, err = os.Stat(s.cliStatePath)
	c.Assert(errors.Is(err, fs.ErrNotExist), tc.Equals, true)
}

func (s *CharmdSuite) TestNoticesTimeout(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{"timeout": {"1s"}})

		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": [{
				"id": "1",
				"user-id": 1000,
				"type": "custom",
				"key": "a.b/c",
				"first-occurred": "2023-09-05T17:18:00Z",
				"last-occurred": "2023-09-05T19:18:00Z",
				"last-repeated": "2023-09-05T18:18:00Z",
				"occurrences": 3
			}
		]}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{
		"notices", "--abs-time", "--timeout", "1s"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, `
ID   User  Type    Key    First                 Repeated              Occurrences
1    1000  custom  a.b/c  2023-09-05T17:18:00Z  2023-09-05T18:18:00Z  3
`[1:])
	c.Check(s.Stderr(), tc.Equals, "")

	cliState := s.readNoticesCLIState(c)
	c.Check(cliState, tc.DeepEquals, map[string]any{
		"notices-last-listed": "2023-09-05T18:18:00Z",
		"notices-last-okayed": "0001-01-01T00:00:00Z",
	})
}

func (s *CharmdSuite) TestNoticesNoNoticesTimeout(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{"timeout": {"1s"}})
		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": []}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"notices", "--timeout", "1s"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, "")
	c.Check(s.Stderr(), tc.Equals, "No matching notices after waiting 1s.\n")

	// Shouldn't have updated cli.json
	_, err = os.Stat(s.cliStatePath)
	c.Assert(errors.Is(err, fs.ErrNotExist), tc.Equals, true)
}

func (s *CharmdSuite) TestNoticesJSON(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{})
		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": [{
				"id": "1",
				"user-id": 1000,
				"type": "custom",
				"key": "a.b/c",
				"first-occurred": "2023-09-05T17:18:00Z",
				"last-occurred": "2023-09-05T19:18:00Z",
				"last-repeated": "2023-09-05T18:18:00Z",
				"occurrences": 3,
				"last-data": {"k": "v"},
				"repeat-after": "1h0m0s",
				"expire-after": "168h0m0s"
			}
		]}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"notices", "--format", "json"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	expected := `{"notices":[{"id":"1","user-id":1000,"type":"custom","key":"a.b/c",` +
		`"first-occurred":"2023-09-05T17:18:00Z","last-occurred":"2023-09-05T19:18:00Z",` +
		`"last-repeated":"2023-09-05T18:18:00Z","occurrences":3,"last-data":{"k":"v"},` +
		`"repeat-after":"1h0m0s","expire-after":"168h0m0s"}]}` + "\n"
	c.Check(s.Stdout(), tc.Equals, expected)
	c.Check(s.Stderr(), tc.Equals, "")

	cliState := s.readNoticesCLIState(c)
	c.Check(cliState, tc.DeepEquals, map[string]any{
		"notices-last-listed": "2023-09-05T18:18:00Z",
		"notices-last-okayed": "0001-01-01T00:00:00Z",
	})
}

func (s *CharmdSuite) TestNoticesYAML(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		c.Check(r.URL.Query(), tc.DeepEquals, url.Values{})
		fmt.Fprint(w, `{
			"type": "sync",
			"status-code": 200,
			"result": [{
				"id": "1",
				"user-id": 1000,
				"type": "custom",
				"key": "a.b/c",
				"first-occurred": "2023-09-05T17:18:00Z",
				"last-occurred": "2023-09-05T19:18:00Z",
				"last-repeated": "2023-09-05T18:18:00Z",
				"occurrences": 3,
				"last-data": {"k": "v"},
				"repeat-after": "1h0m0s",
				"expire-after": "168h0m0s"
			}
		]}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"notices", "--format", "yaml"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, `
notices:
    - id: "1"
      user-id: 1000
      type: custom
      key: a.b/c
      first-occurred: 2023-09-05T17:18:00Z
      last-occurred: 2023-09-05T19:18:00Z
      last-repeated: 2023-09-05T18:18:00Z
      occurrences: 3
      last-data:
        k: v
      repeat-after: 1h0m0s
      expire-after: 168h0m0s
`[1:])
	c.Check(s.Stderr(), tc.Equals, "")

	cliState := s.readNoticesCLIState(c)
	c.Check(cliState, tc.DeepEquals, map[string]any{
		"notices-last-listed": "2023-09-05T18:18:00Z",
		"notices-last-okayed": "0001-01-01T00:00:00Z",
	})
}

func (s *CharmdSuite) TestNoNoticesJSON(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		fmt.Fprint(w, `{"type": "sync", "status-code": 200, "result": []}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"notices", "--format", "json"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, `{"notices":[]}`+"\n")
	c.Check(s.Stderr(), tc.Equals, "")
}

func (s *CharmdSuite) TestNoNoticesYAML(c *tc.C) {
	s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, "GET")
		c.Check(r.URL.Path, tc.Equals, "/v1/notices")
		fmt.Fprint(w, `{"type": "sync", "status-code": 200, "result": []}`)
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"notices", "--format", "yaml"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, "notices: []\n")
	c.Check(s.Stderr(), tc.Equals, "")
}

func (s *CharmdSuite) TestNoticesInvalidFormat(c *tc.C) {
	_, err := cli.ParserForTest().ParseArgs([]string{"notices", "--format", "foobar"})
	c.Assert(err, tc.ErrorMatches, "Invalid value.*for option.*--format.*")
}
