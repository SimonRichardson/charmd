package client_test

import (
	"io"

	"github.com/canonical/tc"
)

func (cs *clientSuite) TestInstallCharm(c *tc.C) {
	cs.rsp = `{"status-code":202,"type":"async","change":"1"}`

	changeID, err := cs.cli.InstallCharm("postgresql-k8s")
	c.Assert(err, tc.IsNil)
	c.Check(changeID, tc.Equals, "1")
	c.Check(cs.req.Method, tc.Equals, "POST")
	c.Check(cs.req.URL.String(), tc.Equals, "http://localhost/v1/charms")

	body, err := io.ReadAll(cs.req.Body)
	c.Assert(err, tc.IsNil)
	c.Check(string(body), tc.Equals, `{"action":"install","charm":"postgresql-k8s"}`)
}

func (cs *clientSuite) TestInstallCharmError(c *tc.C) {
	cs.err = io.EOF
	_, err := cs.cli.InstallCharm("postgresql-k8s")
	c.Check(err, tc.ErrorMatches, `cannot communicate with server: EOF`)
	c.Check(cs.req.URL.String(), tc.Equals, "http://localhost/v1/charms")
}

func (cs *clientSuite) TestInstallCharmRejectsInvalidAsyncResponse(c *tc.C) {
	for _, test := range []struct {
		response string
		err      string
	}{
		{`{"status-code":200,"type":"sync"}`, `expected async response.*got "sync"`},
		{`{"status-code":200,"type":"async","change":"1"}`, `operation not accepted`},
		{`{"status-code":202,"type":"async"}`, `async response without change reference`},
	} {
		cs.rsp = test.response
		_, err := cs.cli.InstallCharm("postgresql-k8s")
		c.Check(err, tc.ErrorMatches, test.err)
	}
}
