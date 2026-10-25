package cli_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/SimonRichardson/charmd/client"
	"github.com/SimonRichardson/charmd/internals/cli"
	"github.com/canonical/tc"
)

// TestTasksDisplaysHookFailure verifies command output includes task tracebacks
// and change errors while preserving the task status table.
func (*CharmdSuite) TestTasksDisplaysHookFailure(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"sync","status-code":200,"result":{"id":"2","status":"Error","err":"hook failed","tasks":[{"status":"Done","summary":"Fetch charm configuration"},{"status":"Error","summary":"Run charm config-changed hook","log":["Traceback: invalid application flag"]}]}}`)
	}))
	defer server.Close()
	api, err := client.New(&client.Config{BaseURL: server.URL})
	c.Assert(err, tc.IsNil)

	var stdout, stderr bytes.Buffer
	c.Assert(cli.RunWithClient([]string{"tasks", "2"}, api, &stdout, &stderr), tc.IsNil)

	for _, expected := range []string{"Fetch charm configuration", "Run charm config-changed hook", "Traceback: invalid application flag", "Error: hook failed"} {
		c.Check(strings.Contains(stdout.String(), expected), tc.Equals, true, tc.Commentf("output: %s", stdout.String()))
	}
	c.Check(stderr.String(), tc.Equals, "")
}
