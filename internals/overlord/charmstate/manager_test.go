package charmstate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

	checksum := newCharmAPI().info.SHA256
	change, err := manager.Install(checksum)
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
		var taskChecksum string
		c.Assert(task.Get(charmSHA256Key, &taskChecksum), tc.IsNil)
		c.Check(taskChecksum, tc.Equals, checksum)
	}
}

func (suite *managerSuite) TestInstallRequiresValidChecksum(c *tc.C) {
	st := state.New(nil)
	manager := NewManager(st, state.NewTaskRunner(st), newCharmAPI(), c.MkDir())
	for _, checksum := range []string{"", "bad", strings.Repeat("0", 63)} {
		_, err := manager.Install(checksum)
		c.Check(err, tc.NotNil, tc.Commentf("checksum %q", checksum))
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
	change, err := manager.Install(api.info.SHA256)
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
	c.Check(api.checksum, tc.Equals, api.info.SHA256)
	c.Check(archivePath, tc.Equals, filepath.Join(charmDir, api.info.SHA256+".charm"))

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	defer st.Unlock()
	c.Check(change.Status().Ready(), tc.Equals, true)
	c.Check(change.Err(), tc.IsNil)
	var installPath string
	c.Assert(tasks[1].Get(charmInstallPathKey, &installPath), tc.IsNil)
	_, err = os.Stat(filepath.Join(installPath, "metadata.yaml"))
	c.Check(err, tc.IsNil)
}

func (suite *managerSuite) TestFetchCharmFailsWithoutDownloadOnResolutionError(c *tc.C) {
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	defer r.Stop()
	api := newCharmAPI()
	api.getErr = errors.New("unavailable")
	manager := NewManager(st, r, api, c.MkDir())
	change, err := manager.Install(api.info.SHA256)
	c.Assert(err, tc.IsNil)

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	defer st.Unlock()
	tasks := change.Tasks()
	c.Check(tasks[0].Status(), tc.Equals, state.ErrorStatus)
	c.Check(tasks[1].Status(), tc.Equals, state.HoldStatus)
	c.Check(api.destination, tc.Equals, "")
}

func (suite *managerSuite) TestFetchCharmRejectsInvalidDownloadInfoWithoutSideEffects(c *tc.C) {
	st := state.New(nil)
	r := state.NewTaskRunner(st)
	defer r.Stop()
	charmDir := filepath.Join(c.MkDir(), "charms")
	api := newCharmAPI()
	checksum := api.info.SHA256
	api.info.SHA256 = strings.Repeat("f", 64)
	manager := NewManager(st, r, api, charmDir)
	change, err := manager.Install(checksum)
	c.Assert(err, tc.IsNil)

	c.Assert(r.Ensure(), tc.IsNil)
	r.Wait()

	st.Lock()
	tasks := change.Tasks()
	c.Check(tasks[0].Status(), tc.Equals, state.ErrorStatus)
	st.Unlock()
	c.Check(api.destination, tc.Equals, "")
	_, err = os.Stat(charmDir)
	c.Check(os.IsNotExist(err), tc.Equals, true)
}

func (suite *managerSuite) TestUnpackArchiveRejectsPathTraversal(c *tc.C) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	file, err := archive.Create("../outside")
	c.Assert(err, tc.IsNil)
	_, err = file.Write([]byte("unexpected"))
	c.Assert(err, tc.IsNil)
	c.Assert(archive.Close(), tc.IsNil)

	root := c.MkDir()
	archivePath := filepath.Join(root, "charm")
	c.Assert(os.WriteFile(archivePath, buffer.Bytes(), 0644), tc.IsNil)
	err = unpackArchive(archivePath, filepath.Join(root, "installed"))
	c.Check(err, tc.ErrorMatches, `charm archive contains invalid path "\.\./outside"`)
	_, err = os.Stat(filepath.Join(root, "outside"))
	c.Check(os.IsNotExist(err), tc.Equals, true)
}

type fakeCharmAPI struct {
	info        charmapi.DownloadInfo
	checksum    string
	destination string
	getErr      error
	archive     []byte
}

// GetCharmDownloadInfo records checksum and returns the configured archive.
func (f *fakeCharmAPI) GetCharmDownloadInfo(
	_ context.Context, checksum string,
) (charmapi.DownloadInfo, error) {
	f.checksum = checksum
	if f.getErr != nil {
		return charmapi.DownloadInfo{}, f.getErr
	}
	return f.info, nil
}

// DownloadCharm records the archive details and writes a test archive.
func (f *fakeCharmAPI) DownloadCharm(
	_ context.Context, info charmapi.DownloadInfo, destination string,
) error {
	f.checksum = info.SHA256
	f.destination = destination
	return os.WriteFile(destination, f.archive, 0644)
}

func newCharmAPI() *fakeCharmAPI {
	archive := charmArchive()
	digest := sha256.Sum256(archive)
	return &fakeCharmAPI{info: charmapi.DownloadInfo{
		Name: "postgresql-k8s", SHA256: hex.EncodeToString(digest[:]),
	}, archive: archive}
}

func charmArchive() []byte {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	file, err := archive.Create("metadata.yaml")
	if err != nil {
		panic(err)
	}
	if _, err := file.Write([]byte("name: postgresql-k8s\n")); err != nil {
		panic(err)
	}
	if err := archive.Close(); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}
