package backup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/harvester/harvester/pkg/util"
)

func execSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName},
		Data: map[string][]byte{
			RepositoryIDKey:   []byte("repository-id"),
			util.AWSAccessKey: []byte("access"),
			util.AWSSecretKey: []byte("secret"),
			"RESTIC_PASSWORD": []byte("restic-password"),
			"KOPIA_PASSWORD":  []byte("kopia-password"),
			"UNRELATED":       []byte("private"),
		},
	}
}

func TestExecUsesCurrentCredentials(t *testing.T) {
	for _, engine := range []string{"restic", "kopia"} {
		t.Run(engine, func(t *testing.T) {
			client := fake.NewSimpleClientset(execSecret())
			opts, err := parseExec(append(JobCommand("repository-id", engine)[2:], "existing script"))
			require.NoError(t, err)
			inherited := []string{
				"PATH=/bin", "RESTIC_REPOSITORY=s3:example", "AWS_ACCESS_KEY_ID=old",
				"AWS_SECRET_ACCESS_KEY=old", "RESTIC_PASSWORD=old", "KOPIA_PASSWORD=old",
			}
			var captured [][]string
			for _, suffix := range []string{"", "-rotated"} {
				source := execSecret()
				source.Data[util.AWSAccessKey] = []byte("access" + suffix)
				source.Data[util.AWSSecretKey] = []byte("secret" + suffix)
				_, err = client.CoreV1().Secrets(source.Namespace).Update(
					context.Background(), source, metav1.UpdateOptions{},
				)
				require.NoError(t, err)
				client.ClearActions()
				err = execWithCredentials(context.Background(), client.CoreV1(), opts, inherited,
					func(path string, args, env []string) error {
						require.Equal(t, "/bin/sh", path)
						require.Equal(t, []string{"/bin/sh", "-c", "existing script"}, args)
						require.ElementsMatch(t, []string{
							"PATH=/bin", "RESTIC_REPOSITORY=s3:example",
							"AWS_ACCESS_KEY_ID=access" + suffix, "AWS_SECRET_ACCESS_KEY=secret" + suffix,
							strings.ToUpper(engine) + "_PASSWORD=" + engine + "-password",
						}, env)
						captured = append(captured, env)
						return nil
					})
				require.NoError(t, err)
				require.Len(t, client.Actions(), 1)
				action := client.Actions()[0].(clienttesting.GetAction)
				require.True(t, action.Matches("get", "secrets"))
				require.Equal(t, util.LonghornSystemNamespaceName, action.GetNamespace())
				require.Equal(t, util.BackupTargetSecretName, action.GetName())
			}
			require.Contains(t, captured[0], "AWS_ACCESS_KEY_ID=access")
			require.Contains(t, captured[1], "AWS_ACCESS_KEY_ID=access-rotated")
		})
	}
}

func TestExecRejectsInvalidCredentials(t *testing.T) {
	for _, engine := range []string{"restic", "kopia"} {
		for _, invalid := range []string{
			RepositoryIDKey, util.AWSAccessKey, util.AWSSecretKey, strings.ToUpper(engine) + "_PASSWORD",
			"mismatch", "missing-secret", "forbidden",
		} {
			t.Run(engine+"/"+invalid, func(t *testing.T) {
				source := execSecret()
				delete(source.Data, invalid)
				if invalid == "mismatch" {
					source.Data[RepositoryIDKey] = []byte("different-repository")
				}
				client := fake.NewSimpleClientset(source)
				if invalid == "missing-secret" {
					client = fake.NewSimpleClientset()
				}
				if invalid == "forbidden" {
					client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
						return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"},
							util.BackupTargetSecretName, errors.New("denied"))
					})
				}
				opts := &execOptions{repositoryID: "repository-id", engine: engine, command: []string{"/bin/sh"}}
				err := execWithCredentials(context.Background(), client.CoreV1(), opts, nil,
					func(string, []string, []string) error {
						t.Fatal("must not execute a command with invalid credentials")
						return nil
					})
				require.Error(t, err)
				require.NotContains(t, err.Error(), "restic-password")
				require.NotContains(t, err.Error(), "kopia-password")
			})
		}
	}
}

func TestExecArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--repository-id", "repo", "--engine", "other", "--", "/bin/sh"},
		{"--repository-id", "repo", "--engine", "restic"},
		{"--repository-id", "repo", "--password=private"},
	} {
		_, err := parseExec(args)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private")
	}
}

// Exercise the real exec boundary: credentials reach the script, its exit code
// survives, and neither credential values nor wrapper output are printed.
func TestExecPreservesExitCode(t *testing.T) {
	if os.Getenv("HARVESTER_REPOSITORY_EXEC_TEST") == "1" {
		client := fake.NewSimpleClientset(execSecret())
		opts := &execOptions{
			repositoryID: "repository-id", engine: "restic",
			command: []string{"/bin/sh", "-c",
				`test "$AWS_ACCESS_KEY_ID" = access && test "$AWS_SECRET_ACCESS_KEY" = secret && ` +
					`test "$RESTIC_PASSWORD" = restic-password && test -z "${KOPIA_PASSWORD:-}" && exit 3`,
			},
		}
		require.NoError(t, execWithCredentials(context.Background(), client.CoreV1(), opts, os.Environ(), syscall.Exec))
		t.Fatal("exec must replace this process")
	}
	binary, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(binary, "-test.run=^TestExecPreservesExitCode$")
	command.Env = append(os.Environ(), "HARVESTER_REPOSITORY_EXEC_TEST=1", "KOPIA_PASSWORD=stale")
	output, err := command.CombinedOutput()
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.Equal(t, 3, exitError.ExitCode())
	require.Empty(t, output)
}
