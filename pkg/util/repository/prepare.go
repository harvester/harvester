// Package repository prepares third-party repositories before credentials are published.
package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
	resticutil "github.com/harvester/harvester/pkg/util/restic"
)

// Both engines share this budget, including cold Kopia index downloads.
const prepareTimeout = 10 * time.Minute

// Manager serializes local initializers. The leader context cancels and reaps
// each child before releasing the mutex. Independent clusters must coordinate
// their first initialization separately; this is not storage-level fencing.
type Manager struct {
	mu    sync.Mutex
	run   func(context.Context, command) error
	store func(*settings.BackupTarget, map[string][]byte) (objectStore, error)
}

type command struct {
	engine string
	action string
	args   []string
	env    []string
}

// Init opens existing repositories or initializes them after inspecting their prefixes.
func (m *Manager) Init(ctx context.Context, target *settings.BackupTarget, data map[string][]byte) error {
	return m.withWorkspace(ctx, data, func(ctx context.Context, env []string, dir string) error {
		store, err := m.openStore(target, data)
		if err != nil {
			return err
		}
		defer store.close()
		if err := m.initRestic(ctx, target, env, store); err != nil {
			return err
		}
		return m.initKopia(ctx, target, env, dir, store)
	})
}

// Connect only opens existing repositories. Missing repositories return an error.
func (m *Manager) Connect(ctx context.Context, target *settings.BackupTarget, data map[string][]byte) error {
	return m.withWorkspace(ctx, data, func(ctx context.Context, env []string, dir string) error {
		if err := m.connectRestic(ctx, target, env); err != nil {
			return err
		}
		return m.connectKopia(ctx, target, env, dir)
	})
}

func (m *Manager) withWorkspace(
	ctx context.Context, data map[string][]byte, run func(context.Context, []string, string) error,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, prepareTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "harvester-repository-")
	if err != nil {
		return fmt.Errorf("creating repository workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	env, err := processEnv(data, dir)
	if err != nil {
		return err
	}
	return run(ctx, env, dir)
}

func (m *Manager) openStore(target *settings.BackupTarget, data map[string][]byte) (objectStore, error) {
	if m.store != nil {
		return m.store(target, data)
	}
	return newStore(target, data)
}

func (m *Manager) execute(ctx context.Context, cmd command) error {
	if m.run != nil {
		return m.run(ctx, cmd)
	}
	return runCommand(ctx, cmd)
}

func (m *Manager) connectRestic(ctx context.Context, target *settings.BackupTarget, env []string) error {
	repo, err := resticutil.Repository(target)
	if err != nil {
		return err
	}
	return m.execute(ctx, resticCommand(repo, "open", env, "cat", "config"))
}

func (m *Manager) initRestic(
	ctx context.Context, target *settings.BackupTarget, env []string, store objectStore,
) error {
	repo, err := resticutil.Repository(target)
	if err != nil {
		return err
	}
	probe := resticCommand(repo, "open", env, "cat", "config")
	err = m.execute(ctx, probe)
	if err == nil || exitCode(err) != 10 {
		return err
	}
	if err := emptyPrefix(ctx, store, "restic/", "config"); err != nil {
		return fmt.Errorf("preparing Restic: %w", err)
	}
	init := resticCommand(repo, "initialize", env, "init")
	if err := m.execute(ctx, init); err != nil {
		return err
	}
	return m.execute(ctx, probe)
}

func resticCommand(repo *resticutil.RepositoryConfig, action string, env []string, args ...string) command {
	return command{
		engine: "restic",
		action: action,
		args:   append([]string{"--no-cache", "-o", "s3.region=" + repo.Region}, args...),
		env:    append(env, "RESTIC_REPOSITORY="+repo.URL),
	}
}

func (m *Manager) connectKopia(ctx context.Context, target *settings.BackupTarget, env []string, dir string) error {
	repo, err := kopiautil.S3Repository(target)
	if err != nil {
		return err
	}
	return m.execute(ctx, kopiaCommand(repo, "connect", dir, env))
}

func (m *Manager) initKopia(
	ctx context.Context, target *settings.BackupTarget, env []string, dir string, store objectStore,
) error {
	repo, err := kopiautil.S3Repository(target)
	if err != nil {
		return err
	}
	probe := kopiaCommand(repo, "connect", dir, env)
	err = m.execute(ctx, probe)
	if err == nil || exitCode(err) != 1 {
		return err
	}
	if err := emptyPrefix(ctx, store, "kopia/", "kopia.repository"); err != nil {
		return fmt.Errorf("preparing Kopia: %w", err)
	}
	if err := m.execute(ctx, kopiaCommand(repo, "create", dir, env)); err != nil {
		return err
	}
	// Always verify through a fresh local connection after creation.
	if err := resetKopiaConfig(dir); err != nil {
		return err
	}
	return m.execute(ctx, probe)
}

func resetKopiaConfig(dir string) error {
	if err := os.Remove(filepath.Join(dir, "kopia.config")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("resetting Kopia connection: %w", err)
	}
	return nil
}

func kopiaCommand(repo *kopiautil.Repository, action, dir string, env []string) command {
	// A positive content cache selects Kopia's disk-backed index cache.
	// Content and metadata each use 32 MiB soft / 64 MiB hard limits. Indexes
	// are separate, so these limits do not bound the whole workspace or RSS.
	args := []string{
		"--no-auto-maintenance", "--disable-file-logging",
		"--config-file=" + filepath.Join(dir, "kopia.config"),
		"repository", action, "s3", "--bucket=" + repo.Bucket, "--endpoint=" + repo.Endpoint,
		"--region=" + repo.Region, "--prefix=" + repo.Prefix,
		"--cache-directory=" + filepath.Join(dir, "cache"),
		"--content-cache-size-mb=32", "--content-cache-size-limit-mb=64",
		"--metadata-cache-size-mb=32", "--metadata-cache-size-limit-mb=64",
	}
	if repo.DisableTLS {
		args = append(args, "--disable-tls")
	}
	return command{engine: "kopia", action: action, args: args, env: env}
}

func credentialEnv(data map[string][]byte) []string {
	env := make([]string, 0, 4)
	for _, key := range []string{
		util.AWSAccessKey, util.AWSSecretKey, backuputil.ResticPasswordKey, backuputil.KopiaPasswordKey,
	} {
		env = append(env, key+"="+string(data[key]))
	}
	return env
}
