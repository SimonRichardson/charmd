package client_test

import (
	"io"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/client"
)

func (cs *clientSuite) TestInstallCharm(c *tc.C) {
	cs.rsp = `{"status-code":202,"type":"async","change":"1"}`

	changeID, err := cs.cli.InstallCharm(client.InstallCharmOptions{
		Name:     "postgresql-k8s",
		SHA256:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Revision: clientIntPtr(42),
		Channel:  "14/stable",
	})
	c.Assert(err, tc.IsNil)
	c.Check(changeID, tc.Equals, "1")
	c.Check(cs.req.Method, tc.Equals, "POST")
	c.Check(cs.req.URL.String(), tc.Equals, "http://localhost/v1/charms")

	body, err := io.ReadAll(cs.req.Body)
	c.Assert(err, tc.IsNil)
	expected := `{"action":"install","name":"postgresql-k8s",` +
		`"sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",` +
		`"revision":42,"channel":"14/stable"}`
	c.Check(string(body), tc.Equals, expected)
}

func clientIntPtr(value int) *int { return &value }

func (cs *clientSuite) TestInstallCharmError(c *tc.C) {
	cs.err = io.EOF
	_, err := cs.cli.InstallCharm(client.InstallCharmOptions{Name: "postgresql-k8s"})
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
		_, err := cs.cli.InstallCharm(client.InstallCharmOptions{Name: "postgresql-k8s"})
		c.Check(err, tc.ErrorMatches, test.err)
	}
}
