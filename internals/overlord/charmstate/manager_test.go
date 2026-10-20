package charmstate

import (
	"reflect"
	"testing"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/config"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type managerSuite struct{}

var _ = tc.InternalSuite(&managerSuite{})

func (s *managerSuite) TestNewManagerRequiresCharmAPI(c *tc.C) {
	defer func() {
		c.Check(recover(), tc.NotNil)
	}()
	NewManager(state.New(nil), state.NewTaskRunner(state.New(nil)), nil)
}

func (suite *managerSuite) TestNewManager(c *tc.C) {
	client, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	c.Assert(err, tc.IsNil)
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	c.Check(NewManager(st, r, client), tc.NotNil)
	got := r.KnownTaskKinds()
	c.Check(reflect.DeepEqual(got, []string{FetchCharmTask, InstallCharmTask}) || reflect.DeepEqual(got, []string{InstallCharmTask, FetchCharmTask}), tc.Equals, true)
}

func (suite *managerSuite) TestInstallCreatesDependentTasks(c *tc.C) {
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	manager := NewManager(st, r, newCharmAPI(c))

	change, err := manager.Install("postgresql-k8s")
	c.Assert(err, tc.IsNil)

	st.Lock()
	defer st.Unlock()
	c.Check(change.Kind(), tc.Equals, "install-charm")
	tasks := change.Tasks()
	c.Assert(tasks, tc.HasLen, 2)
	c.Check(tasks[0].Kind(), tc.Equals, FetchCharmTask)
	c.Check(tasks[1].Kind(), tc.Equals, InstallCharmTask)
	c.Check(tasks[1].WaitTasks(), tc.DeepEquals, []*state.Task{tasks[0]})
	c.Check(change.CheckTaskDependencies(), tc.IsNil)
	for _, task := range tasks {
		var charm string
		c.Assert(task.Get(charmNameKey, &charm), tc.IsNil)
		c.Check(charm, tc.Equals, "postgresql-k8s")
	}
}

func (suite *managerSuite) TestInstallRequiresCharmName(c *tc.C) {
	st := state.New(nil)
	manager := NewManager(st, state.NewTaskRunner(st), newCharmAPI(c))
	for _, charm := range []string{"", " \t", "\n"} {
		_, err := manager.Install(charm)
		c.Check(err, tc.NotNil, tc.Commentf("charm %q", charm))
	}

	st.Lock()
	defer st.Unlock()
	c.Check(st.Changes(), tc.HasLen, 0)
	c.Check(st.TaskCount(), tc.Equals, 0)
}

func (suite *managerSuite) TestInstallTasksExecuteInOrder(c *tc.C) {
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	defer r.Stop()
	manager := NewManager(st, r, newCharmAPI(c))
	change, err := manager.Install("postgresql-k8s")
	c.Assert(err, tc.IsNil)

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	tasks := change.Tasks()
	c.Check(tasks[0].Status(), tc.Equals, state.DoneStatus)
	c.Check(tasks[1].Status(), tc.Equals, state.DoStatus)
	st.Unlock()

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(change.Status().Ready(), tc.Equals, true)
	c.Check(change.Err(), tc.IsNil)
}

func newCharmAPI(c *tc.C) *charmapi.Client {
	client, err := charmapi.New(config.Endpoint{URL: "https://agent.example.test"})
	c.Assert(err, tc.IsNil)
	return client
}
