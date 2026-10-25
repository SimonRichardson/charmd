package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/configapi"
	"github.com/SimonRichardson/charmd/internals/leadershipapi"
	"github.com/SimonRichardson/charmd/internals/overlord"
	"github.com/SimonRichardson/charmd/internals/overlord/charmstate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type daemonSuite struct{}

var _ = tc.InternalSuite(&daemonSuite{})

func (s *daemonSuite) TestHealth(c *tc.C) {
	d := New(nil)
	recorder := httptest.NewRecorder()
	d.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	c.Check(recorder.Code, tc.Equals, http.StatusOK)
	c.Check(recorder.Body.Len(), tc.Equals, 0)
}

func (s *daemonSuite) TestReadinessLifecycle(c *tc.C) {
	d := New(nil)

	assertReadinessStatus(c, d, http.StatusServiceUnavailable)
	d.Start()
	assertReadinessStatus(c, d, http.StatusOK)
	d.Stop()
	assertReadinessStatus(c, d, http.StatusServiceUnavailable)
}

func (s *daemonSuite) TestPostCharmsCreatesInstallChange(c *tc.C) {
	o := newOverlord(c)
	defer o.Stop()
	d := New(o)
	recorder := httptest.NewRecorder()
	body := `{"action":"install",` +
		`"sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/charms", bytes.NewBufferString(body))
	d.Handler().ServeHTTP(recorder, req)

	c.Assert(recorder.Code, tc.Equals, http.StatusAccepted)

	var response struct {
		Type   string `json:"type"`
		Change string `json:"change"`
	}
	c.Assert(json.NewDecoder(recorder.Body).Decode(&response), tc.IsNil)

	c.Check(response.Type, tc.Equals, "async")
	c.Assert(response.Change, tc.Not(tc.Equals), "")

	st := o.State()
	st.Lock()
	defer st.Unlock()
	change := st.Change(response.Change)
	c.Assert(change, tc.NotNil)

	c.Check(change.Kind(), tc.Equals, "install-charm")
	tasks := change.Tasks()
	c.Assert(tasks, tc.HasLen, 3)

	c.Check(tasks[0].Kind(), tc.Equals, charmstate.FetchCharmTask)
	c.Check(tasks[1].Kind(), tc.Equals, charmstate.InstallCharmTask)
	c.Check(tasks[1].WaitTasks(), tc.DeepEquals, []*state.Task{tasks[0]})
	c.Check(tasks[2].WaitTasks(), tc.DeepEquals, []*state.Task{tasks[1]})
	for _, task := range tasks[:2] {
		var checksum string
		c.Assert(task.Get("charm-sha256", &checksum), tc.IsNil)

		c.Check(checksum, tc.Equals, "0123456789abcdef0123456789abcdef"+"0123456789abcdef0123456789abcdef")
	}
}

func (s *daemonSuite) TestPostCharmsRejectsInvalidRequest(c *tc.C) {
	o := newOverlord(c)
	defer o.Stop()
	d := New(o)
	for _, body := range []string{
		`{"action":`,
		`{"action":"remove","sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`,
		`{"action":"install","sha256":"bad"}`,
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/charms", bytes.NewBufferString(body))
		d.Handler().ServeHTTP(recorder, req)
		c.Check(recorder.Code, tc.Equals, http.StatusBadRequest, tc.Commentf("body %s", body))
		var response struct {
			StatusCode int    `json:"status-code"`
			Type       string `json:"type"`
		}
		c.Assert(json.NewDecoder(recorder.Body).Decode(&response), tc.IsNil, tc.Commentf("body %s", body))

		c.Check(response.StatusCode, tc.Equals, http.StatusBadRequest, tc.Commentf("body %s", body))
		c.Check(response.Type, tc.Equals, "error", tc.Commentf("body %s", body))
	}

	st := o.State()
	st.Lock()
	defer st.Unlock()
	c.Check(st.Changes(), tc.HasLen, 0)
	c.Check(st.TaskCount(), tc.Equals, 0)
}

func newOverlord(c *tc.C) *overlord.Overlord {
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	c.Assert(err, tc.IsNil)

	configAPI, err := configapi.New(config.Endpoint{URL: "https://config.example.test"})
	c.Assert(err, tc.IsNil)

	o, err := overlord.New(&overlord.Options{
		StatePath:     filepath.Join(c.MkDir(), "state.json"),
		CharmAPI:      charmAPI,
		LeadershipAPI: newLeadershipClient(c), ConfigAPI: configAPI,
	})
	c.Assert(err, tc.IsNil)

	return o
}

func assertReadinessStatus(c *tc.C, d *Daemon, want int) {
	recorder := httptest.NewRecorder()
	d.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	c.Check(recorder.Code, tc.Equals, want)
	c.Check(recorder.Body.Len(), tc.Equals, 0)
}

// newLeadershipClient supplies a required service without starting network activity.
func newLeadershipClient(c *tc.C) leadershipapi.Client {
	client, err := leadershipapi.New(config.Endpoint{URL: "https://leadership.example.test"})
	c.Assert(err, tc.IsNil)

	return client
}
