package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type commandError struct {
	engine, action string
	code           int
}

func (e *commandError) Error() string {
	return fmt.Sprintf("%s repository %s failed (exit %d)", e.engine, e.action, e.code)
}

func exitCode(err error) int {
	var failed *commandError
	if errors.As(err, &failed) {
		return failed.code
	}
	return -1
}

// Commands use fixed binaries and argument arrays, never a shell. CLI output
// can contain credentials or decrypted config, so it is not logged or returned.
func runCommand(ctx context.Context, spec command) error {
	// #nosec G204 -- the caller selects only the bundled restic or kopia binary.
	cmd := exec.CommandContext(ctx, spec.engine, spec.args...)
	cmd.Env = spec.env
	err := cmd.Run() // Run waits for the child even when cancellation kills it.
	if ctx.Err() != nil {
		return fmt.Errorf("%s repository %s: %w", spec.engine, spec.action, ctx.Err())
	}
	if err == nil {
		return nil
	}
	var failed *exec.ExitError
	if errors.As(err, &failed) {
		return &commandError{engine: spec.engine, action: spec.action, code: failed.ExitCode()}
	}
	return fmt.Errorf("cannot start %s repository %s", spec.engine, spec.action)
}

func processEnv(data map[string][]byte, dir string) ([]string, error) {
	// Avoid inheriting repository overrides, ambient AWS credentials or another
	// goroutine's backupstore environment. Match the current Jobs: default system
	// CA roots, no target-specific proxy or certificate overrides.
	// GOMEMLIMIT is a soft Go memory target, not an RSS or container limit.
	credentials := credentialEnv(data)
	env := make([]string, 0, 8+len(credentials))
	env = append(env,
		"PATH="+os.Getenv("PATH"), "HOME="+dir, "TMPDIR="+dir,
		"GOMEMLIMIT=128MiB", "GOMAXPROCS=2", "AWS_EC2_METADATA_DISABLED=true",
		"KOPIA_LOG_DIR="+dir, "KOPIA_CHECK_FOR_UPDATES=false",
	)
	for _, value := range credentials {
		key, _, _ := strings.Cut(value, "=")
		if strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("invalid repository credential field %s", key)
		}
	}
	env = append(env, credentials...)
	return env, nil
}
