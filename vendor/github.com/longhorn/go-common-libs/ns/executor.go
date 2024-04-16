package ns

import (
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/longhorn/go-common-libs/exec"
	"github.com/longhorn/go-common-libs/proc"
	"github.com/longhorn/go-common-libs/types"
)

// Executor is a struct resonpsible for executing commands in a specific
// namespace using nsenter.
type Executor struct {
	namespaces  []types.Namespace // The namespaces to enter.
	nsDirectory string            // The directory of the namespace.

	executor exec.ExecuteInterface // An interface for executing commands. This allows mocking for unit tests.
}

// NewNamespaceExecutor creates a new namespace executor for the given process name,
// namespaces and proc directory. If the process name is not empty, it will try to
// use the process namespace directory. Otherwise, it will use the host namespace
// directory. The namespaces are the namespaces to enter. The proc directory is
// the directory where the process information is stored. It will also verify the
// existence of the nsenter binary.
func NewNamespaceExecutor(processName, procDirectory string, namespaces []types.Namespace) (*Executor, error) {
	nsDir, err := proc.GetProcessNamespaceDirectory(processName, procDirectory)
	if err != nil {
		return nil, err
	}

	NamespaceExecutor := &Executor{
		namespaces:  namespaces,
		nsDirectory: nsDir,
		executor:    exec.NewExecutor(),
	}

	if _, err := NamespaceExecutor.executor.Execute(nil, types.NsBinary, []string{"-V"}, types.ExecuteDefaultTimeout); err != nil {
		return nil, errors.Wrap(err, "cannot find nsenter for namespace switching")
	}

	return NamespaceExecutor, nil
}

// prepareCommandArgs prepares the nsenter command arguments.
func (nsexec *Executor) prepareCommandArgs(binary string, args, envs []string) []string {
	cmdArgs := []string{}
	for _, ns := range nsexec.namespaces {
		nsPath := filepath.Join(nsexec.nsDirectory, ns.String())
		switch ns {
		case types.NamespaceIpc:
			cmdArgs = append(cmdArgs, "--ipc="+nsPath)
		case types.NamespaceMnt:
			cmdArgs = append(cmdArgs, "--mount="+nsPath)
		case types.NamespaceNet:
			cmdArgs = append(cmdArgs, "--net="+nsPath)
		}
	}
	if len(envs) > 0 {
		cmdArgs = append(cmdArgs, "env", "-i")
		cmdArgs = append(cmdArgs, envs...)
	}

	cmdArgs = append(cmdArgs, binary)
	return append(cmdArgs, args...)
}

// Execute executes the command in the namespace. If NsDirectory is empty,
// it will execute the command in the current namespace.
func (nsexec *Executor) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return nsexec.executor.Execute(nil, types.NsBinary, nsexec.prepareCommandArgs(binary, args, envs), timeout)
}

// ExecuteWithStdin executes the command in the namespace with stdin.
// If NsDirectory is empty, it will execute the command in the current namespace.
func (nsexec *Executor) ExecuteWithStdin(envs []string, binary string, args []string, stdinString string, timeout time.Duration) (string, error) {
	return nsexec.executor.ExecuteWithStdin(types.NsBinary, nsexec.prepareCommandArgs(binary, args, envs), stdinString, timeout)
}

// ExecuteWithStdinPipe executes the command in the namespace with stdin pipe.
// If NsDirectory is empty, it will execute the command in the current namespace.
func (nsexec *Executor) ExecuteWithStdinPipe(envs []string, binary string, args []string, stdinString string, timeout time.Duration) (string, error) {
	return nsexec.executor.ExecuteWithStdinPipe(types.NsBinary, nsexec.prepareCommandArgs(binary, args, envs), stdinString, timeout)
}
