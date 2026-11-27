//go:build linux

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
	resticutil "github.com/harvester/harvester/pkg/util/restic"
)

// This test creates and deletes its own unique bucket. The endpoint must be an
// isolated test server; production backup-target configuration is never read.
func TestS3Preparation(t *testing.T) {
	endpoint := os.Getenv("HARVESTER_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("HARVESTER_TEST_S3_ENDPOINT is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	target := &settings.BackupTarget{
		Type: settings.S3BackupType, Endpoint: endpoint, BucketRegion: "us-east-1",
		BucketName: fmt.Sprintf("harvester-init-test-%d", time.Now().UnixNano()),
	}
	data := repoData()
	data[util.AWSAccessKey] = []byte(os.Getenv("HARVESTER_TEST_S3_ACCESS_KEY"))
	data[util.AWSSecretKey] = []byte(os.Getenv("HARVESTER_TEST_S3_SECRET_KEY"))
	require.NotEmpty(t, data[util.AWSAccessKey])
	require.NotEmpty(t, data[util.AWSSecretKey])
	backend, err := newStore(target, data)
	require.NoError(t, err)
	defer backend.close()
	store := backend.(*s3Store)
	_, err = store.client.CreateBucketWithContext(ctx, &s3.CreateBucketInput{Bucket: aws.String(target.BucketName)})
	require.NoError(t, err)
	t.Cleanup(func() { deleteTestBucket(t, store) })

	var mu sync.Mutex
	creates := map[string]int{}
	manager := &Manager{run: func(ctx context.Context, spec command) error {
		if spec.action == "create" || spec.action == "initialize" {
			mu.Lock()
			creates[spec.engine]++
			mu.Unlock()
		}
		return measuredRun(t, ctx, spec)
	}}
	// Concurrent submissions/volumes share one initializer. Later requests open
	// the repositories created by the first request, including empty repositories.
	results := make(chan error, 3)
	for range 3 {
		go func() { results <- manager.Init(ctx, target, data) }()
	}
	for range 3 {
		require.NoError(t, <-results)
	}
	require.Equal(t, map[string]int{"restic": 1, "kopia": 1}, creates)
	configs := readConfigs(t, ctx, store)

	dir := t.TempDir()
	env, err := processEnv(data, dir)
	require.NoError(t, err)
	rrepo, err := resticutil.Repository(target)
	require.NoError(t, err)
	env = append(env, "RESTIC_REPOSITORY="+rrepo.URL)
	resticData(t, ctx, env)
	krepo, err := kopiautil.S3Repository(target)
	require.NoError(t, err)
	require.NoError(t, runCommand(ctx, kopiaCommand(krepo, "connect", dir, env)))
	ids := kopiaData(t, ctx, dir, env)

	// Reset/reconnect uses the same password; credentials are not re-derived.
	require.NoError(t, manager.Connect(ctx, target, data))
	require.Equal(t, configs, readConfigs(t, ctx, store))
	checkKopiaCache(t, ctx, target, data)
	verifyData(t, ctx, dir, env, ids)
	wrong := maps.Clone(data)
	wrong[backuputil.ResticPasswordKey] = []byte("wrong")
	require.Error(t, manager.Connect(ctx, target, wrong))
	wrong = maps.Clone(data)
	wrong[backuputil.KopiaPasswordKey] = []byte("wrong")
	require.Error(t, manager.Connect(ctx, target, wrong))
	require.Equal(t, configs, readConfigs(t, ctx, store))
	require.Equal(t, map[string]int{"restic": 1, "kopia": 1}, creates)

	// An independent S3 principal can use the same repositories and passwords.
	if access := os.Getenv("HARVESTER_TEST_S3_ROTATED_ACCESS_KEY"); access != "" {
		rotated := maps.Clone(data)
		rotated[util.AWSAccessKey] = []byte(access)
		rotated[util.AWSSecretKey] = []byte(os.Getenv("HARVESTER_TEST_S3_ROTATED_SECRET_KEY"))
		require.NoError(t, manager.Connect(ctx, target, rotated))
		require.Equal(t, configs, readConfigs(t, ctx, store))
		rotatedDir := t.TempDir()
		rotatedEnv, err := processEnv(rotated, rotatedDir)
		require.NoError(t, err)
		rotatedEnv = append(rotatedEnv, "RESTIC_REPOSITORY="+rrepo.URL)
		require.NoError(t, runCommand(ctx, kopiaCommand(krepo, "connect", rotatedDir, rotatedEnv)))
		verifyData(t, ctx, rotatedDir, rotatedEnv, ids)
	}

	// Losing configuration cannot turn leftover data into a new repository.
	require.NoError(t, store.remove(ctx, "restic/config"))
	require.Error(t, manager.Connect(ctx, target, data))
	require.ErrorContains(t, manager.Init(ctx, target, data), "existing data")
	require.Equal(t, map[string]int{"restic": 1, "kopia": 1}, creates)
}

// Exercise a cold connection with real CLI flags and existing repository indexes.
func checkKopiaCache(t *testing.T, ctx context.Context, target *settings.BackupTarget, data map[string][]byte) {
	t.Helper()
	dir := t.TempDir()
	env, err := processEnv(data, dir)
	require.NoError(t, err)
	manager := &Manager{}
	require.NoError(t, manager.connectKopia(ctx, target, env, dir))

	config, err := os.ReadFile(filepath.Join(dir, "kopia.config"))
	require.NoError(t, err)
	var saved struct {
		Cache struct {
			Directory    string `json:"cacheDirectory"`
			ContentSoft  int64  `json:"maxCacheSize"`
			ContentHard  int64  `json:"contentCacheSizeLimitBytes"`
			MetadataSoft int64  `json:"maxMetadataCacheSize"`
			MetadataHard int64  `json:"metadataCacheSizeLimitBytes"`
		} `json:"caching"`
	}
	require.NoError(t, json.Unmarshal(config, &saved))
	cacheDir := saved.Cache.Directory
	if !filepath.IsAbs(cacheDir) {
		cacheDir = filepath.Join(dir, cacheDir)
	}
	require.Equal(t, filepath.Join(dir, "cache"), cacheDir)
	require.Equal(t, int64(32<<20), saved.Cache.ContentSoft)
	require.Equal(t, int64(64<<20), saved.Cache.ContentHard)
	require.Equal(t, int64(32<<20), saved.Cache.MetadataSoft)
	require.Equal(t, int64(64<<20), saved.Cache.MetadataHard)
	indexes, err := filepath.Glob(filepath.Join(cacheDir, "indexes", "*.sndx"))
	require.NoError(t, err)
	require.NotEmpty(t, indexes, "cold connect should cache repository indexes on disk")
	require.NoDirExists(t, filepath.Join(dir, "cli-logs"))
	require.NoDirExists(t, filepath.Join(dir, "content-logs"))
}

func measuredRun(t *testing.T, ctx context.Context, spec command) error {
	t.Helper()
	// #nosec G204 -- fixed test commands use the packaged binaries on PATH.
	cmd := exec.CommandContext(ctx, spec.engine, spec.args...)
	cmd.Env = spec.env
	err := cmd.Run()
	if cmd.ProcessState != nil {
		usage := cmd.ProcessState.SysUsage().(*syscall.Rusage)
		t.Logf("%s %s: peak RSS %d MiB", spec.engine, spec.action, usage.Maxrss/1024)
	}
	if err == nil {
		return nil
	}
	if failed, ok := err.(*exec.ExitError); ok {
		return &commandError{engine: spec.engine, action: spec.action, code: failed.ExitCode()}
	}
	return err
}

func cliOutput(t *testing.T, ctx context.Context, env []string, engine string, args ...string) []byte {
	t.Helper()
	// #nosec G204 -- fixed test commands use the packaged binaries on PATH.
	cmd := exec.CommandContext(ctx, engine, args...)
	cmd.Env = env
	out, err := cmd.Output()
	require.NoError(t, err, "%s %s failed", engine, args[0])
	return out
}

func resticData(t *testing.T, ctx context.Context, env []string) {
	t.Helper()
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			name := fmt.Sprintf("volume-%d", i)
			// Use the operational script: it must not contain an initializer.
			script := resticutil.BackupCommand("restic", "printf "+name, name, []string{name})
			cliOutput(t, ctx, env, "bash", "-c", script)
		})
	}
	wg.Wait()
	for i := range 2 {
		name := fmt.Sprintf("volume-%d", i)
		out := cliOutput(t, ctx, env, "restic", "dump", "--tag", name, "latest", "/"+name)
		require.Equal(t, name, string(out))
	}
}

func kopiaData(t *testing.T, ctx context.Context, dir string, env []string) []string {
	t.Helper()
	ids := make([]string, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			name := fmt.Sprintf("volume-%d", i)
			path := filepath.Join(dir, name)
			require.NoError(t, os.WriteFile(path, []byte(name), 0600))
			args := []string{"--no-auto-maintenance", "--config-file=" + filepath.Join(dir, "kopia.config")}
			out := cliOutput(t, ctx, env, "kopia", append(args, "snapshot", "create", "--json", path)...)
			var snapshot struct {
				Root struct {
					ObjectID string `json:"obj"`
				} `json:"rootEntry"`
			}
			require.NoError(t, json.Unmarshal(out, &snapshot))
			require.NotEmpty(t, snapshot.Root.ObjectID)
			ids[i] = snapshot.Root.ObjectID
			dest := filepath.Join(dir, name+"-restored")
			cliOutput(t, ctx, env, "kopia", append(args, "snapshot", "restore", snapshot.Root.ObjectID, dest)...)
			got, err := os.ReadFile(dest)
			require.NoError(t, err)
			require.Equal(t, name, string(got))
		})
	}
	wg.Wait()
	return ids
}

func readConfigs(t *testing.T, ctx context.Context, store *s3Store) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, key := range []string{"restic/config", "kopia/kopia.repository"} {
		object, err := store.client.GetObjectWithContext(ctx, &s3.GetObjectInput{Bucket: &store.bucket, Key: &key})
		require.NoError(t, err)
		data, err := io.ReadAll(object.Body)
		require.NoError(t, err)
		require.NoError(t, object.Body.Close())
		result[key] = string(data)
	}
	return result
}

func deleteTestBucket(t *testing.T, store *s3Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := store.client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{Bucket: &store.bucket})
	require.NoError(t, err)
	for _, object := range result.Contents {
		require.NoError(t, store.remove(ctx, aws.StringValue(object.Key)))
	}
	_, err = store.client.DeleteBucketWithContext(ctx, &s3.DeleteBucketInput{Bucket: &store.bucket})
	require.NoError(t, err)
}

func verifyData(t *testing.T, ctx context.Context, dir string, env, ids []string) {
	t.Helper()
	for i, id := range ids {
		name := fmt.Sprintf("volume-%d", i)
		out := cliOutput(t, ctx, env, "restic", "dump", "--tag", name, "latest", "/"+name)
		require.Equal(t, name, string(out))
		out = cliOutput(t, ctx, env, "kopia", "--no-auto-maintenance",
			"--config-file="+filepath.Join(dir, "kopia.config"), "show", id)
		require.Equal(t, name, string(out))
	}
}
