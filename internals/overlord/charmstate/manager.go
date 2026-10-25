// Package charmstate owns charm changes and their task handlers.
package charmstate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/tomb.v2"

	"github.com/SimonRichardson/charmd/internals/charmapi"
	"github.com/SimonRichardson/charmd/internals/overlord/runtimestate"
	"github.com/SimonRichardson/charmd/internals/overlord/state"
)

const (
	// FetchCharmTask resolves and downloads the requested charm.
	FetchCharmTask = "fetch-charm"
	// InstallCharmTask installs a charm fetched by FetchCharmTask.
	InstallCharmTask = "install-charm"
)

const (
	charmSHA256Key       = "charm-sha256"
	charmDownloadInfoKey = "charm-download-info"
	charmArchivePathKey  = "charm-archive-path"
	charmInstallPathKey  = "charm-install-path"
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

// Install creates the change that fetches and then installs a SHA-addressed charm.
func (m *Manager) Install(checksum string) (*state.Change, error) {
	if err := validateChecksum(checksum); err != nil {
		return nil, err
	}

	m.state.Lock()
	defer m.state.Unlock()

	change := m.state.NewChange("install-charm", fmt.Sprintf("Install charm %q", checksum))
	fetch := m.state.NewTask(FetchCharmTask, fmt.Sprintf("Fetch charm %q", checksum))
	install := m.state.NewTask(InstallCharmTask, fmt.Sprintf("Install charm %q", checksum))

	fetch.Set(charmSHA256Key, checksum)
	install.Set(charmSHA256Key, checksum)

	install.WaitFor(fetch)

	change.AddTask(fetch)
	change.AddTask(install)
	hook := runtimestate.InstallTask(m.state, install)
	hook.Set(charmSHA256Key, checksum)
	change.AddTask(hook)

	m.state.EnsureBefore(0)

	return change, nil
}

// fetchCharm resolves a charm, downloads its verified archive, and persists the
// result needed by subsequent install tasks.
func (m *Manager) fetchCharm(task *state.Task, tomb *tomb.Tomb) error {
	m.state.Lock()
	var checksum string
	err := task.Get(charmSHA256Key, &checksum)
	m.state.Unlock()
	if err != nil {
		return fmt.Errorf("get charm checksum: %w", err)
	}

	ctx := tomb.Context(context.Background())
	info, err := m.charmAPI.GetCharmDownloadInfo(ctx, checksum)
	if err != nil {
		return fmt.Errorf("get charm download info: %w", err)
	}
	if info.SHA256 != checksum {
		return fmt.Errorf("discovered checksum %q does not match %q", info.SHA256, checksum)
	}
	if err := validateChecksum(info.SHA256); err != nil {
		return err
	}
	if err := os.MkdirAll(m.charmDir, 0755); err != nil {
		return fmt.Errorf("create charm directory: %w", err)
	}
	archivePath := filepath.Join(m.charmDir, info.SHA256+".charm")
	if err := verifyArchive(archivePath, checksum); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := m.charmAPI.DownloadCharm(ctx, info, archivePath); err != nil {
			return fmt.Errorf("download charm: %w", err)
		}
		if err := verifyArchive(archivePath, checksum); err != nil {
			return err
		}
	}

	m.state.Lock()
	defer m.state.Unlock()
	task.Set(charmDownloadInfoKey, info)
	task.Set(charmArchivePathKey, archivePath)
	return nil
}

// validateChecksum rejects invalid SHA-addressed charm selectors.
func validateChecksum(checksum string) error {
	if len(checksum) != 64 {
		return fmt.Errorf("invalid charm download checksum %q", checksum)
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return fmt.Errorf("invalid charm download checksum %q", checksum)
	}
	return nil
}

// verifyArchive hashes the cached archive and checks it against the requested digest.
func verifyArchive(archivePath, checksum string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, archive); err != nil {
		return fmt.Errorf("hash charm archive: %w", err)
	}
	if hex.EncodeToString(digest.Sum(nil)) != checksum {
		return fmt.Errorf("charm archive checksum does not match %q", checksum)
	}
	return nil
}

// installCharm unpacks the verified archive fetched by its prerequisite task.
func (m *Manager) installCharm(task *state.Task, _ *tomb.Tomb) error {
	m.state.Lock()
	dependencies := task.WaitTasks()
	if len(dependencies) != 1 {
		m.state.Unlock()
		return fmt.Errorf("install charm task requires one fetch task")
	}
	var info charmapi.DownloadInfo
	if err := dependencies[0].Get(charmDownloadInfoKey, &info); err != nil {
		m.state.Unlock()
		return fmt.Errorf("get fetched charm info: %w", err)
	}
	var archivePath string
	if err := dependencies[0].Get(charmArchivePathKey, &archivePath); err != nil {
		m.state.Unlock()
		return fmt.Errorf("get fetched charm archive: %w", err)
	}
	m.state.Unlock()

	installPath := filepath.Join(m.charmDir, info.SHA256)
	if info, err := os.Stat(installPath); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("charm installation is not a directory")
		}
		m.state.Lock()
		task.Set(charmInstallPathKey, installPath)
		m.state.Unlock()
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect charm installation: %w", err)
	}
	if err := unpackArchive(archivePath, installPath); err != nil {
		return err
	}

	m.state.Lock()
	defer m.state.Unlock()
	task.Set(charmInstallPathKey, installPath)
	return nil
}

// unpackArchive extracts into a temporary directory and publishes it atomically after validating entry paths.
func unpackArchive(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open charm archive: %w", err)
	}
	defer archive.Close()
	temporary, err := os.MkdirTemp(filepath.Dir(destination), "."+filepath.Base(destination)+"-")
	if err != nil {
		return fmt.Errorf("create temporary charm installation: %w", err)
	}
	defer os.RemoveAll(temporary)
	for _, file := range archive.File {
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("charm archive contains symbolic link %q", file.Name)
		}
		target := filepath.Join(temporary, file.Name)
		relative, err := filepath.Rel(temporary, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("charm archive contains invalid path %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, file.Mode().Perm()); err != nil {
				return fmt.Errorf("create charm directory: %w", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("create charm parent directory: %w", err)
		}
		reader, err := file.Open()
		if err != nil {
			return fmt.Errorf("open charm archive file: %w", err)
		}
		writer, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, file.Mode().Perm())
		if err == nil {
			_, err = io.Copy(writer, reader)
			closeErr := writer.Close()
			if err == nil {
				err = closeErr
			}
		}
		reader.Close()
		if err != nil {
			return fmt.Errorf("extract charm archive file: %w", err)
		}
	}
	if err := os.Rename(temporary, destination); err != nil {
		return fmt.Errorf("store charm installation: %w", err)
	}
	return nil
}
