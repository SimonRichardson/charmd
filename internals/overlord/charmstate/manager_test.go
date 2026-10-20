package charmstate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

type managerSuite struct{}

var _ = tc.InternalSuite(&managerSuite{})

func (s *managerSuite) TestNewManagerRequiresCharmAPI(c *tc.C) {
	defer func() {
		c.Check(recover(), tc.NotNil)
	}()
	NewManager(state.New(nil), state.NewTaskRunner(state.New(nil)), nil, c.MkDir())
}

func (suite *managerSuite) TestNewManager(c *tc.C) {
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	c.Check(NewManager(st, r, newCharmAPI(), c.MkDir()), tc.NotNil)
	got := r.KnownTaskKinds()
	knownKinds := reflect.DeepEqual(got, []string{FetchCharmTask, InstallCharmTask}) ||
		reflect.DeepEqual(got, []string{InstallCharmTask, FetchCharmTask})
	c.Check(knownKinds, tc.Equals, true)
}

func (suite *managerSuite) TestInstallCreatesDependentTasks(c *tc.C) {
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	manager := NewManager(st, r, newCharmAPI(), c.MkDir())

	change, err := manager.Install(charmapi.DownloadRequest{Name: "postgresql-k8s"})
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
	manager := NewManager(st, state.NewTaskRunner(st), newCharmAPI(), c.MkDir())
	for _, charm := range []string{"", " \t", "\n"} {
		_, err := manager.Install(charmapi.DownloadRequest{Name: charm})
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
	charmDir := c.MkDir()
	api := newCharmAPI()
	manager := NewManager(st, r, api, charmDir)
	request := charmapi.DownloadRequest{
		Name: "postgresql-k8s", Revision: intPtr(42), Channel: "14/stable",
	}
	change, err := manager.Install(request)
	c.Assert(err, tc.IsNil)

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	tasks := change.Tasks()
	c.Check(tasks[0].Status(), tc.Equals, state.DoneStatus)
	c.Check(tasks[1].Status(), tc.Equals, state.DoStatus)
	var info charmapi.DownloadInfo
	c.Assert(tasks[0].Get(charmDownloadInfoKey, &info), tc.IsNil)
	c.Check(info, tc.DeepEquals, api.info)
	var archivePath string
	c.Assert(tasks[0].Get(charmArchivePathKey, &archivePath), tc.IsNil)
	st.Unlock()
	c.Check(api.request, tc.DeepEquals, request)
	c.Check(api.downloadURL, tc.Equals, api.info.URL)
	c.Check(api.checksum, tc.Equals, api.info.SHA256)
	c.Check(archivePath, tc.Equals, filepath.Join(charmDir, api.info.SHA256+".charm"))
	data, readErr := os.ReadFile(archivePath)
	c.Assert(readErr, tc.IsNil)
	c.Check(data, tc.DeepEquals, []byte("charm archive"))

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(change.Status().Ready(), tc.Equals, true)
	c.Check(change.Err(), tc.IsNil)
}

func (suite *managerSuite) TestFetchCharmFailsWithoutDownloadOnResolutionError(c *tc.C) {
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	defer r.Stop()
	api := newCharmAPI()
	api.getErr = errors.New("unavailable")
	manager := NewManager(st, r, api, c.MkDir())
	change, err := manager.Install(charmapi.DownloadRequest{Name: "postgresql-k8s"})
	c.Assert(err, tc.IsNil)

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	defer st.Unlock()
	tasks := change.Tasks()
	c.Check(tasks[0].Status(), tc.Equals, state.ErrorStatus)
	c.Check(tasks[1].Status(), tc.Equals, state.HoldStatus)
	c.Check(api.downloadURL, tc.Equals, "")
}

func (suite *managerSuite) TestFetchCharmRejectsInvalidDownloadInfoWithoutSideEffects(c *tc.C) {
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	defer r.Stop()
	charmDir := filepath.Join(c.MkDir(), "charms")
	api := newCharmAPI()
	api.info.SHA256 = "bad"
	manager := NewManager(st, r, api, charmDir)
	change, err := manager.Install(charmapi.DownloadRequest{Name: "postgresql-k8s"})
	c.Assert(err, tc.IsNil)

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	tasks := change.Tasks()
	c.Check(tasks[0].Status(), tc.Equals, state.ErrorStatus)
	st.Unlock()
	c.Check(api.downloadURL, tc.Equals, "")
	_, err = os.Stat(charmDir)
	c.Check(os.IsNotExist(err), tc.Equals, true)
}

type fakeCharmAPI struct {
	info        charmapi.DownloadInfo
	request     charmapi.DownloadRequest
	downloadURL string
	checksum    string
	destination string
	getErr      error
}

// GetCharmDownloadInfo records request and returns the configured archive.
func (f *fakeCharmAPI) GetCharmDownloadInfo(
	_ context.Context, request charmapi.DownloadRequest,
) (charmapi.DownloadInfo, error) {
	f.request = request
	if f.getErr != nil {
		return charmapi.DownloadInfo{}, f.getErr
	}
	return f.info, nil
}

// DownloadCharm records the archive details and writes a test archive.
func (f *fakeCharmAPI) DownloadCharm(
	_ context.Context, downloadURL, checksum, destination string,
) error {
	f.downloadURL = downloadURL
	f.checksum = checksum
	f.destination = destination
	return os.WriteFile(destination, []byte("charm archive"), 0644)
}

func newCharmAPI() *fakeCharmAPI {
	return &fakeCharmAPI{info: charmapi.DownloadInfo{
		URL:    "https://downloads.example.test/postgresql-k8s.charm",
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}}
}

func intPtr(value int) *int { return &value }
