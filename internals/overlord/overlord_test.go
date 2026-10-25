package overlord_test

import (
	"path/filepath"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/configapi"
	"github.com/SimonRichardson/charmd/internals/leadershipapi"
	"github.com/SimonRichardson/charmd/internals/overlord"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

type overlordSuite struct{}

var _ = tc.InternalSuite(&overlordSuite{})

func (s *overlordSuite) TestNewConstructsCharmManager(c *tc.C) {
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	c.Assert(err, tc.IsNil)

	o, err := overlord.New(&overlord.Options{
		StatePath:     filepath.Join(c.MkDir(), "state.json"),
		CharmAPI:      charmAPI,
		LeadershipAPI: newLeadershipClient(c), ConfigAPI: newConfigClient(c),
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

	opts := &overlord.Options{StatePath: statePath, CharmAPI: charmAPI, LeadershipAPI: newLeadershipClient(c), ConfigAPI: newConfigClient(c)}
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

// TestConfigManagerWiring verifies the configured service participates in overlord lifecycle management.
func (*overlordSuite) TestConfigManagerWiring(c *tc.C) {
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	c.Assert(err, tc.IsNil)

	configAPI, err := configapi.New(config.Endpoint{URL: "https://config.example.test"})
	c.Assert(err, tc.IsNil)

	o, err := overlord.New(&overlord.Options{StatePath: filepath.Join(c.MkDir(), "state.json"), CharmAPI: charmAPI, LeadershipAPI: newLeadershipClient(c), ConfigAPI: configAPI})
	c.Assert(err, tc.IsNil)

	defer o.Stop()
	c.Check(o.ConfigManager(), tc.NotNil)
}

// newConfigClient provides the required service dependency without starting network activity.
func newConfigClient(c *tc.C) configapi.Client {
	client, err := configapi.New(config.Endpoint{URL: "https://config.example.test"})
	c.Assert(err, tc.IsNil)

	return client
}

// TestNewRequiresConfigAPI verifies incomplete service wiring fails before state is opened.
func (*overlordSuite) TestNewRequiresConfigAPI(c *tc.C) {
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	c.Assert(err, tc.IsNil)

	_, err = overlord.New(&overlord.Options{StatePath: filepath.Join(c.MkDir(), "state.json"), CharmAPI: charmAPI})
	c.Check(err, tc.ErrorMatches, "config API client is required")
}

// newLeadershipClient supplies a required service without starting network activity.
func newLeadershipClient(c *tc.C) leadershipapi.Client {
	client, err := leadershipapi.New(config.Endpoint{URL: "https://leadership.example.test"})
	c.Assert(err, tc.IsNil)

	return client
}

// TestLeadershipManagerWiring verifies required leadership service construction
// and registration participate in the same daemon lifecycle as config changes.
func (*overlordSuite) TestLeadershipManagerWiring(c *tc.C) {
	charmAPI, err := charmapi.New(config.Endpoint{URL: "https://charm.example.test"})
	c.Assert(err, tc.IsNil)

	opts := &overlord.Options{StatePath: filepath.Join(c.MkDir(), "state.json"), CharmAPI: charmAPI, ConfigAPI: newConfigClient(c)}
	_, err = overlord.New(opts)
	c.Check(err, tc.ErrorMatches, "leadership API client is required")
	opts.LeadershipAPI = newLeadershipClient(c)
	o, err := overlord.New(opts)
	c.Assert(err, tc.IsNil)

	defer o.Stop()
	c.Check(o.LeadershipManager(), tc.NotNil)
}
