package overlord_test

import (
	"path/filepath"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/overlord"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

type overlordSuite struct{}

var _ = tc.InternalSuite(&overlordSuite{})

func (s *overlordSuite) TestNewConstructsCharmManager(c *tc.C) {
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	c.Assert(err, tc.IsNil)

	o, err := overlord.New(&overlord.Options{
		StatePath: filepath.Join(c.MkDir(), "state.json"),
		CharmAPI:  charmAPI,
	})
	c.Assert(err, tc.IsNil)

	defer o.Stop()
	c.Check(o.CharmManager(), tc.NotNil)
	c.Check(o.RuntimeManager(), tc.NotNil)
}

func (s *overlordSuite) TestNewRequiresOptions(c *tc.C) {
	_, err := overlord.New(nil)
	c.Check(err, tc.NotNil)
}

func (s *overlordSuite) TestCharmInstallChangeSurvivesRestart(c *tc.C) {
	statePath := filepath.Join(c.MkDir(), "state.json")
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	c.Assert(err, tc.IsNil)

	opts := &overlord.Options{StatePath: statePath, CharmAPI: charmAPI}
	o, err := overlord.New(opts)
	c.Assert(err, tc.IsNil)

	change, err := o.CharmManager().Install("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	c.Assert(err, tc.IsNil)

	changeID := change.ID()
	o.Stop()

	restored, err := overlord.New(opts)
	c.Assert(err, tc.IsNil)

	defer restored.Stop()

	st := restored.State()
	st.Lock()
	defer st.Unlock()

	change = st.Change(changeID)
	c.Assert(change, tc.NotNil)

	tasks := change.Tasks()
	c.Assert(tasks, tc.HasLen, 3)

	c.Check(tasks[1].WaitTasks(), tc.DeepEquals, []*state.Task{tasks[0]})
	c.Check(tasks[2].WaitTasks(), tc.DeepEquals, []*state.Task{tasks[1]})
	for _, task := range tasks[:2] {
		var checksum string
		c.Assert(task.Get("charm-sha256", &checksum), tc.IsNil)

		c.Check(checksum, tc.Equals, "0123456789abcdef0123456789abcdef"+"0123456789abcdef0123456789abcdef")
	}
}
