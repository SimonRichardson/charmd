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
	"time"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/cli"
)

func (s *CharmdSuite) TestOkay(c *tc.C) {
	s.writeCLIState(c, map[string]any{
		"notices-last-listed": time.Date(2023, 9, 6, 15, 6, 0, 0, time.UTC),
		"notices-last-okayed": time.Time{},
	})

	rest, err := cli.ParserForTest().ParseArgs([]string{"okay"})
	c.Assert(err, tc.IsNil)
	c.Check(rest, tc.HasLen, 0)
	c.Check(s.Stdout(), tc.Equals, "")
	c.Check(s.Stderr(), tc.Equals, "")

	cliState := s.readNoticesCLIState(c)
	c.Check(cliState, tc.DeepEquals, map[string]any{
		"notices-last-listed": "2023-09-06T15:06:00Z",
		"notices-last-okayed": "2023-09-06T15:06:00Z",
	})
}

func (s *CharmdSuite) TestOkayNoNotices(c *tc.C) {
	_, err := cli.ParserForTest().ParseArgs([]string{"okay"})
	c.Assert(err, tc.ErrorMatches, "no notices.* have been listed.*")
	c.Check(s.Stdout(), tc.Equals, "")
	c.Check(s.Stderr(), tc.Equals, "")
}
