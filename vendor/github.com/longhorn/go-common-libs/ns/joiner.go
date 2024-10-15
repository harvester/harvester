package ns

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/longhorn/go-common-libs/proc"
	"github.com/longhorn/go-common-libs/types"
	"github.com/longhorn/go-common-libs/utils"
)

type Joiners []*Joiner

// Joiner is a context with information about a namespace.
type Joiner struct {
	namespace types.Namespace // The namespace to join (e.g. net, mnt)
	fd        int             // The file descriptor of the namespace.
	flags     uintptr         // The flags to use when joining the namespace.

	isJoined bool // A boolean to indicate if the namespace has been joined.
}

// ReverseOrder returns a reversed copy of the Joiners.
func (joiners *Joiners) ReverseOrder() Joiners {
	joinerCount := len(*joiners)

	reversed := make(Joiners, joinerCount)
	for i, j := joinerCount-1, 0; i >= 0; i, j = i-1, j+1 {
		reversed[j] = (*joiners)[i]
	}
	return reversed
}

// JoinReverse joins all the namespaces in the Joiners in reverse order.
func (joiners *Joiners) JoinReverse() (err error) {
	*joiners = joiners.ReverseOrder()
	return joiners.Join()
}

// Reset resets all the Joiners.
func (joiners *Joiners) Reset() (err error) {
	for _, joiner := range *joiners {
		logrus.Tracef("Resetting namespace: %+v", joiner)
		joiner = &Joiner{} // nolint:ineffassign
	}
	return nil
}

// OpenFile opens a file in the Joiners.
func (joiners *Joiners) OpenFile(path string) (fd int, err error) {
	return unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC, 0)
}

// CloseFiles closes all the files in the Joiners.
func (joiners *Joiners) CloseFiles() {
	for _, joiner := range *joiners {
		if joiner.fd == -1 {
			continue
		}

		if err := unix.Close(joiner.fd); err != nil {
			logrus.WithError(err).Warnf("Failed to close %v namespace file", joiner.namespace)
			continue
		}

		joiner.fd = -1
	}
}

// JoinerDescriptor is a struct that holds information about the namespaces to join.
type JoinerDescriptor struct {
	namespaces []types.Namespace // List of namespaces to be joined.

	directory string // The target directory where the namespace files are located.
	pid       uint64 // Process ID associated with the Joiner.

	origin Joiners // Contexts of the original namespaces.
	target Joiners // Contexts of the namespaces to be joined.

	stop    chan struct{} // Channel to signal stopping the execution of the function within namespaces.
	timeout time.Duration // Timeout duration for the execution of the function within namespaces.
}

// RunFunc runs the given function in the host namespace.
// Returns the result of the function and any error that occurred.
func RunFunc(fn func() (interface{}, error), timeout time.Duration) (interface{}, error) {
	joiner, err := NewJoiner(types.HostProcDirectory, 0)
	if err != nil {
		return nil, err
	}

	return joiner.Run(fn)
}

type JoinerInterface interface {
	Revert() error
	Run(fn func() (interface{}, error)) (interface{}, error)
}

type NewJoinerFunc func(string, time.Duration) (JoinerInterface, error)

// NewJoiner is a variable holding the function responsible for creating
// a new JoinerInterface.
// By using a variable for the creation function, it allows for easier unit testing
// by substituting a mock implementation.
var NewJoiner NewJoinerFunc = newJoiner

// newJoiner creates a new JoinerInterface.
func newJoiner(procDirectory string, timeout time.Duration) (nsjoin JoinerInterface, err error) {
	log := logrus.WithFields(logrus.Fields{
		"procDirectory": procDirectory,
		"timeout":       timeout,
	})
	log.Trace("Initializing new namespace joiner")

	if timeout == 0 {
		timeout = types.NsJoinerDefaultTimeout
	}

	if procDirectory == "" {
		return &JoinerDescriptor{
			stop:    make(chan struct{}),
			timeout: timeout,
		}, nil
	}

	nsDir := proc.GetHostNamespaceDirectory(procDirectory)
	procPid := proc.GetHostNamespacePID(procDirectory)

	return &JoinerDescriptor{
		directory: nsDir,
		pid:       procPid,

		namespaces: []types.Namespace{
			types.NamespaceMnt,
			types.NamespaceNet,
		},

		origin: Joiners{},
		target: Joiners{},

		stop:    make(chan struct{}),
		timeout: timeout,
	}, err
}

// OpenNamespaceFiles opens required namespace files.
func (jd *JoinerDescriptor) OpenNamespaceFiles() (err error) {
	defer func() {
		err = errors.Wrapf(err, "failed to open namespace files")
		if err != nil {
			_ = jd.Revert()
		}
	}()

	for _, namespace := range jd.namespaces {
		err = jd.openAndRecordNamespaceFiles(namespace)
		if err != nil {
			break
		}
	}
	return err
}

// openAndRecordNamespaceFiles opens and records current and target namespace files.
func (jd *JoinerDescriptor) openAndRecordNamespaceFiles(namespace types.Namespace) error {
	logrus.Tracef("Opening %s namespace file", namespace)

	ns := namespace.String()

	if err := jd.openAndRecordOriginalNamespaceFile(ns, namespace); err != nil {
		return err
	}

	return jd.openAndRecordTargetNamespaceFile(ns, namespace)
}

// openAndRecordOriginalNamespaceFile opens the original namespace file and records
// the file descriptor and namespace information.
// The original namespace file is the namespace file of the process thread that is
// executing the joiner (e.g. /proc/1/task/2/ns/mnt)
func (jd *JoinerDescriptor) openAndRecordOriginalNamespaceFile(ns string, namespace types.Namespace) error {
	pthreadFile := filepath.Join("/proc", fmt.Sprint(os.Getpid()), "task", fmt.Sprint(Gettid()), "ns", ns)
	originFd, err := jd.origin.OpenFile(pthreadFile)
	if err != nil {
		return errors.Wrapf(err, "failed to open process thread file %v", pthreadFile)
	}
	jd.origin = append(jd.origin, &Joiner{
		namespace: namespace,
		fd:        originFd,
		flags:     namespace.Flag(),
	})
	return nil
}

// openAndRecordTargetNamespaceFile opens the target namespace file and records
// the file descriptor and namespace information.
// The target namespace file is the namespace file of the process that is being
// joined (e.g. /host/proc/123/ns/mnt)
func (jd *JoinerDescriptor) openAndRecordTargetNamespaceFile(ns string, namespace types.Namespace) error {
	namespaceFile := filepath.Join(jd.directory, ns)
	targetFd, err := jd.target.OpenFile(namespaceFile)
	if err != nil {
		return errors.Wrapf(err, "failed to open namespace file %v", namespaceFile)
	}
	jd.target = append(jd.target, &Joiner{
		namespace: namespace,
		fd:        targetFd,
		flags:     namespace.Flag(),
	})
	return nil
}

// Join joins the target namespaces.
func (jd *JoinerDescriptor) Join() (err error) {
	defer func() {
		err = errors.Wrap(err, "failed to join namespaces")
	}()
	return jd.target.Join()
}

// Revert reverts to the original namespaces.
func (jd *JoinerDescriptor) Revert() (err error) {
	if jd.target == nil {
		return nil
	}

	defer func() {
		err = errors.Wrap(err, "failed to revert namespaces")
		if err == nil {
			logrus.Tracef("Reverted to %v namespace", jd.directory)
		}

		jd.target.CloseFiles()
		_ = jd.target.Reset()

		jd.origin.CloseFiles()
		_ = jd.origin.Reset()
	}()

	logrus.Trace("Reverting namespaces")
	if err := jd.origin.JoinReverse(); err != nil {
		return err
	}

	_, err = os.Stat(jd.directory)
	return err
}

// Run executes the function in the target namespace.
// The function is executed in a goroutine with a locked OS thread to ensure
// namespace isolation.
func (jd *JoinerDescriptor) Run(fn func() (interface{}, error)) (interface{}, error) {
	errCh := make(chan error)
	resultCh := make(chan interface{})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a wait group to wait for the goroutine to finish
	var wg sync.WaitGroup
	wg.Add(1)

	// Goroutine to run the function in the target namespace.
	go func() {
		defer wg.Done()

		defer close(errCh)
		defer close(resultCh)
		defer cancel()

		// The goroutine runs with a locked OS thread to ensure namespace isolation.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Open the namespace files and revert them after execution.
		err := jd.OpenNamespaceFiles()
		if err != nil {
			errCh <- err
			return
		}
		defer func() {
			_ = jd.Revert()
		}()

		// Join the target namespaces.
		logrus.Trace("Joining target namespaces")
		err = jd.Join()
		if err != nil {
			errCh <- err
			return
		}

		// Execute the given function and send the result or error to the channels.
		select {
		case <-jd.stop:
			return // The stop signal was received, exit without running the function.
		default:
			output, err := fn()
			if err != nil {
				errCh <- err
				return
			}

			resultCh <- output
		}
	}()

	// Goroutine to wait for the function to finish and stop the joiner.
	var stopOnce sync.Once
	go func() {
		wg.Wait()

		// Stop the joiner once to avoid race conditions.
		stopOnce.Do(func() {
			close(jd.stop)
		})
	}()

	// Get the function name for logging.
	fnName := utils.GetFunctionName(fn)

	select {
	case err := <-errCh:
		return nil, errors.Wrapf(err, types.ErrNamespaceFuncFmt, fnName)

	case result := <-resultCh:
		logrus.Tracef("Completed function %v in namespace: %+v", fnName, result)
		return result, nil

	case <-jd.stop:
		logrus.Tracef("Received stop signal while running function: %v", fnName)
		stopOnce.Do(func() {
			close(jd.stop)
		})
		return nil, nil

	case <-time.After(jd.timeout):
		// The function execution timed out, clean up and return an error.
		stopOnce.Do(func() {
			close(jd.stop)
		})
		return nil, errors.Errorf("timeout running function: %v", fnName)
	}
}
