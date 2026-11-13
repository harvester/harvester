package backup

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const ExecSubcommand = "repository-exec"

type execOptions struct {
	repositoryID string
	engine       string
	command      []string
}

// JobCommand wraps the existing shell script. Only public configuration is
// recorded in the Job; credentials are fetched when each container starts.
func JobCommand(repositoryID, engine string) []string {
	return []string{
		"/usr/bin/harvester", ExecSubcommand,
		"--repository-id", repositoryID, "--engine", engine,
		"--", "/bin/sh", "-c",
	}
}

func RunCommand(args []string) error {
	opts, err := parseExec(args)
	if err != nil {
		return err
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("loading repository Job API configuration: %w", err)
	}
	config.Timeout = 30 * time.Second
	client, err := coreclient.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating repository Job API client: %w", err)
	}
	return execWithCredentials(context.Background(), client, opts, os.Environ(), syscall.Exec)
}

func parseExec(args []string) (*execOptions, error) {
	opts := &execOptions{}
	flags := flag.NewFlagSet(ExecSubcommand, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.repositoryID, "repository-id", "", "expected repository ID")
	flags.StringVar(&opts.engine, "engine", "", "restic or kopia")
	if err := flags.Parse(args); err != nil {
		return nil, fmt.Errorf("invalid repository-exec arguments")
	}
	if opts.repositoryID == "" {
		return nil, fmt.Errorf("repository-id is required")
	}
	if opts.engine != string(harvesterv1.Restic) && opts.engine != string(harvesterv1.Kopia) {
		return nil, fmt.Errorf("engine must be restic or kopia")
	}
	opts.command = flags.Args()
	if len(opts.command) == 0 {
		return nil, fmt.Errorf("repository-exec requires a command")
	}
	return opts, nil
}

func execWithCredentials(
	ctx context.Context, client coreclient.CoreV1Interface, opts *execOptions,
	env []string, replace func(string, []string, []string) error,
) error {
	source, err := client.Secrets(util.LonghornSystemNamespaceName).Get(
		ctx, util.BackupTargetSecretName, metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("reading backup target Secret: %w", err)
	}
	passwordKeys := map[string]string{
		string(harvesterv1.Restic): ResticPasswordKey,
		string(harvesterv1.Kopia):  KopiaPasswordKey,
	}
	credentials, err := CredentialEnv(source.Data, opts.repositoryID, passwordKeys[opts.engine])
	if err != nil {
		return err
	}
	path, err := exec.LookPath(opts.command[0])
	if err != nil {
		return fmt.Errorf("locating repository command: %w", err)
	}
	// Replace the wrapper so signals and exit codes (including snapshot-not-found)
	// belong to the original script. Never persist credentials in Kubernetes.
	return replace(path, opts.command, append(cleanEnv(env), credentials...))
}

func cleanEnv(env []string) []string {
	result := make([]string, 0, len(env))
	for _, value := range env {
		key, _, _ := strings.Cut(value, "=")
		switch key {
		case util.AWSAccessKey, util.AWSSecretKey, ResticPasswordKey, KopiaPasswordKey:
			continue
		}
		result = append(result, value)
	}
	return result
}
