// Package charmstate owns charm changes and their task handlers.
package charmstate

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/tomb.v2"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

const (
	// FetchCharmTask resolves and downloads the requested charm.
	FetchCharmTask = "fetch-charm"
	// InstallCharmTask installs a charm fetched by FetchCharmTask.
	InstallCharmTask = "install-charm"
)

const (
	charmNameKey            = "charm-name"
	charmDownloadRequestKey = "charm-download-request"
	charmDownloadInfoKey    = "charm-download-info"
	charmArchivePathKey     = "charm-archive-path"
)

// Manager will create and execute charm fetch and installation tasks.
type Manager struct {
	state    *state.State
	runner   *state.TaskRunner
	charmAPI charmapi.CharmClient
	charmDir string
}

// NewManager constructs the manager, storing downloaded archives in charmDir,
// and registers its task handlers with r.
func NewManager(
	s *state.State, r *state.TaskRunner, charmAPI charmapi.CharmClient, charmDir string,
) *Manager {
	if s == nil {
		panic("state is required")
	}
	if r == nil {
		panic("task runner is required")
	}
	if charmAPI == nil {
		panic("charm API client is required")
	}
	if charmDir == "" {
		panic("charm directory is required")
	}
	m := &Manager{state: s, runner: r, charmAPI: charmAPI, charmDir: charmDir}
	r.AddHandler(FetchCharmTask, m.fetchCharm, nil)
	r.AddHandler(InstallCharmTask, m.installCharm, nil)
	return m
}

// Ensure implements overlord.StateManager. Charm tasks will schedule their own
// work through the shared task runner.
func (m *Manager) Ensure() error { return nil }

// Install creates the change that fetches and then installs charm.
func (m *Manager) Install(request charmapi.DownloadRequest) (*state.Change, error) {
	if strings.TrimSpace(request.Name) == "" {
		return nil, fmt.Errorf("charm name is required")
	}

	m.state.Lock()
	defer m.state.Unlock()

	change := m.state.NewChange("install-charm", fmt.Sprintf("Install charm %q", request.Name))
	fetch := m.state.NewTask(FetchCharmTask, fmt.Sprintf("Fetch charm %q", request.Name))
	install := m.state.NewTask(InstallCharmTask, fmt.Sprintf("Install charm %q", request.Name))

	fetch.Set(charmNameKey, request.Name)
	install.Set(charmNameKey, request.Name)
	fetch.Set(charmDownloadRequestKey, request)
	install.Set(charmDownloadRequestKey, request)

	install.WaitFor(fetch)

	change.AddTask(fetch)
	change.AddTask(install)

	m.state.EnsureBefore(0)

	return change, nil
}

// fetchCharm resolves a charm, downloads its verified archive, and persists the
// result needed by subsequent install tasks.
func (m *Manager) fetchCharm(task *state.Task, tomb *tomb.Tomb) error {
	m.state.Lock()
	var request charmapi.DownloadRequest
	err := task.Get(charmDownloadRequestKey, &request)
	m.state.Unlock()
	if err != nil {
		return fmt.Errorf("get charm download request: %w", err)
	}

	ctx := tomb.Context(context.Background())
	info, err := m.charmAPI.GetCharmDownloadInfo(ctx, request)
	if err != nil {
		return fmt.Errorf("get charm download info: %w", err)
	}
	if err := validateChecksum(info.SHA256); err != nil {
		return err
	}
	if err := os.MkdirAll(m.charmDir, 0755); err != nil {
		return fmt.Errorf("create charm directory: %w", err)
	}
	archivePath := filepath.Join(m.charmDir, info.SHA256+".charm")
	if err := m.charmAPI.DownloadCharm(
		ctx, info.URL, info.SHA256, archivePath,
	); err != nil {
		return fmt.Errorf("download charm: %w", err)
	}

	m.state.Lock()
	defer m.state.Unlock()
	task.Set(charmDownloadInfoKey, info)
	task.Set(charmArchivePathKey, archivePath)
	return nil
}

func validateChecksum(checksum string) error {
	if len(checksum) != 64 {
		return fmt.Errorf("invalid charm download checksum %q", checksum)
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return fmt.Errorf("invalid charm download checksum %q", checksum)
	}
	return nil
}

// installCharm is a placeholder until fetched charm installation is defined.
func (m *Manager) installCharm(*state.Task, *tomb.Tomb) error { return nil }
