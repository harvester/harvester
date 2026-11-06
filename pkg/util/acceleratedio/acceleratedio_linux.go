package acceleratedio

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	blkGetSize64     = 0x80081272
	progressTick     = time.Second
	defaultBlockSize = 64 * 1024
	defaultWorkers   = 4
)

type readTask struct {
	index  int
	offset int64
	data   []byte
}

type readResult struct {
	index int
	data  []byte
	err   error
}

type writeTask struct {
	index  int
	offset int64
	data   []byte
}

type writeState struct {
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
	err  error
}

func newWriteState() *writeState {
	return &writeState{done: make(chan struct{})}
}

func (s *writeState) fail(err error) {
	s.once.Do(func() {
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		close(s.done)
	})
}

func (s *writeState) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("io-mode", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var devicePath string
	var blockSize int
	var workers int
	var mode string

	fs.StringVar(&devicePath, "device", "", "Path to block device")
	fs.IntVar(&blockSize, "bs", defaultBlockSize, "Block size in bytes")
	fs.IntVar(&workers, "workers", defaultWorkers, "Number of concurrent workers")
	fs.StringVar(&mode, "mode", "", "Mode: read or write")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if devicePath == "" {
		return errors.New("no device specified, use -device")
	}
	if blockSize <= 0 {
		return errors.New("block size must be greater than zero")
	}
	if workers <= 0 {
		return errors.New("workers must be greater than zero")
	}

	switch mode {
	case "read":
		return readBlockDevice(devicePath, blockSize, workers, stdout, stderr)
	case "write":
		return writeBlockDevice(devicePath, blockSize, workers, stdin, stderr)
	default:
		return errors.New("invalid mode, use -mode=read or -mode=write")
	}
}

func blockDeviceSize(f *os.File) (int64, error) {
	var size int64
	_, _, errNo := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		blkGetSize64,
		uintptr(unsafe.Pointer(&size)),
	)
	if errNo != 0 {
		return 0, errNo
	}
	return size, nil
}

func readWorker(ctx context.Context, device io.ReaderAt, tasks <-chan readTask, results chan<- readResult) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-tasks:
			if !ok || ctx.Err() != nil {
				return
			}
			n, err := device.ReadAt(task.data, task.offset)
			if n != len(task.data) && (err == nil || errors.Is(err, io.EOF)) {
				err = io.ErrUnexpectedEOF
			} else if n == len(task.data) && errors.Is(err, io.EOF) {
				err = nil
			}
			select {
			case results <- readResult{index: task.index, data: task.data, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}
}

func writeWorker(device io.WriterAt, tasks <-chan writeTask, state *writeState, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		var task writeTask
		var ok bool
		select {
		case <-state.done:
			return
		case task, ok = <-tasks:
			if !ok {
				return
			}
		}

		n, err := device.WriteAt(task.data, task.offset)
		if err != nil {
			state.fail(fmt.Errorf("error writing block %d at offset %d: %w", task.index, task.offset, err))
			return
		}
		if n != len(task.data) {
			state.fail(fmt.Errorf("short write for block %d at offset %d", task.index, task.offset))
			return
		}
	}
}

func runProgressTicker(done <-chan struct{}, current func() int64, totalSize int64, label string, stderr io.Writer) {
	ticker := time.NewTicker(progressTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			written := current()
			var percent float64
			if totalSize > 0 {
				percent = float64(written) / float64(totalSize) * 100
			}
			// Structured prefix `IO_PROGRESS` lets the controller parse the latest line
			// from pod logs to surface progress on the VolumeBackup/VolumeRestore status.
			fmt.Fprintf(stderr, "IO_PROGRESS mode=%s bytes=%d total=%d percent=%.2f\n", label, written, totalSize, percent)
		case <-done:
			fmt.Fprintln(stderr)
			return
		}
	}
}

func startProgress(current func() int64, totalSize int64, label string, stderr io.Writer) func() {
	done := make(chan struct{})
	go runProgressTicker(done, current, totalSize, label, stderr)
	return func() { close(done) }
}

func dispatchWriteTasks(
	stdin io.Reader,
	blockSize int,
	tasks chan<- writeTask,
	state *writeState,
	bytesWritten *int64,
) error {
	index := 0
	offset := int64(0)
	for {
		select {
		case <-state.done:
			return state.failure()
		default:
		}

		buf := make([]byte, blockSize)
		n, err := io.ReadFull(stdin, buf)
		if n > 0 {
			task := writeTask{index: index, offset: offset, data: buf[:n]}
			select {
			case <-state.done:
				return state.failure()
			case tasks <- task:
			}
			atomic.AddInt64(bytesWritten, int64(n))
			offset += int64(n)
			index++
		}
		switch {
		case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
			return nil
		case err != nil:
			readErr := fmt.Errorf("error reading input: %w", err)
			state.fail(readErr)
			return readErr
		}
	}
}

func writeStream(device io.WriterAt, stdin io.Reader, blockSize, workers int, bytesWritten *int64) error {
	tasks := make(chan writeTask, workers)
	state := newWriteState()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go writeWorker(device, tasks, state, &wg)
	}

	err := dispatchWriteTasks(stdin, blockSize, tasks, state, bytesWritten)
	close(tasks)
	wg.Wait()
	if err != nil {
		return err
	}
	return state.failure()
}

func getSize(file *os.File) (int64, error) {
	totalSize, err := blockDeviceSize(file)
	if err == nil {
		return totalSize, nil
	}
	stat, statErr := file.Stat()
	if statErr != nil {
		return 0, fmt.Errorf("error stat'ing device: %w", statErr)
	}
	return stat.Size(), nil
}

func readBlockDevice(devicePath string, blockSize, workers int, stdout, stderr io.Writer) error {
	file, err := os.Open(devicePath)
	if err != nil {
		return fmt.Errorf("error opening device: %w", err)
	}
	defer file.Close()

	totalSize, err := getSize(file)
	if err != nil {
		return err
	}

	var bytesRead int64
	defer startProgress(func() int64 {
		return atomic.LoadInt64(&bytesRead)
	}, totalSize, "READ", stderr)()

	return readStream(context.Background(), file, stdout, totalSize, blockSize, workers, &bytesRead)
}

// readStream owns at most workers buffers of blockSize bytes, including reads
// in flight, queued results and out-of-order results. A buffer is reused only
// after its bytes have been emitted in order. Bounded channels alone cannot
// bound an out-of-order map while its earliest read stalls.
func readStream(ctx context.Context, device io.ReaderAt, stdout io.Writer,
	totalSize int64, blockSize, workers int, bytesRead *int64,
) error {
	if totalSize < 0 || blockSize <= 0 || workers <= 0 {
		return errors.New("invalid read size or worker count")
	}
	ctx, cancel := context.WithCancel(ctx)
	tasks := make(chan readTask, workers)
	results := make(chan readResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readWorker(ctx, device, tasks, results)
		}()
	}
	defer func() {
		cancel()
		close(tasks)
		// Cancellation stops channel operations, but cannot interrupt arbitrary
		// ReadAt/Write syscalls. Wait before the caller closes the device.
		wg.Wait()
	}()

	return emitReads(ctx, stdout, tasks, results, totalSize, blockSize, workers, bytesRead)
}

func emitReads(ctx context.Context, stdout io.Writer, tasks chan<- readTask,
	results <-chan readResult, totalSize int64, blockSize, workers int, bytesRead *int64,
) error {
	var offset int64
	next := 0
	dispatch := func(buf []byte) {
		size := min(int64(blockSize), totalSize-offset)
		tasks <- readTask{index: next, offset: offset, data: buf[:size]}
		offset += size
		next++
	}
	for i := 0; i < workers && offset < totalSize; i++ {
		dispatch(make([]byte, blockSize))
	}

	pending := make(map[int][]byte, workers)
	for expected := 0; expected < next; {
		var res readResult
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res = <-results:
		}
		if res.err != nil {
			return fmt.Errorf("error reading block %d: %w", res.index, res.err)
		}
		pending[res.index] = res.data
		for {
			data, ok := pending[expected]
			if !ok {
				break
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			n, err := stdout.Write(data)
			if err != nil {
				return fmt.Errorf("error emitting block %d: %w", expected, err)
			}
			if n != len(data) {
				return io.ErrShortWrite
			}
			atomic.AddInt64(bytesRead, int64(n))
			delete(pending, expected)
			expected++
			if offset < totalSize {
				dispatch(data[:cap(data)])
			}
		}
	}
	return ctx.Err()
}

func writeBlockDevice(devicePath string, blockSize, workers int, stdin io.Reader, stderr io.Writer) error {
	device, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("error opening device for writing: %w", err)
	}
	defer device.Close()

	totalSize, err := getSize(device)
	if err != nil {
		return err
	}

	var bytesWritten int64
	defer startProgress(func() int64 {
		return atomic.LoadInt64(&bytesWritten)
	}, totalSize, "WRITE", stderr)()

	// Wait for every queued write and collect any worker failure before fsync.
	// A failed worker cancels dispatch so it cannot block sending to a queue
	// that no worker will consume.
	if err = writeStream(device, stdin, blockSize, workers, &bytesWritten); err != nil {
		return err
	}
	// fsync the device so dirty pages reach the underlying storage before we
	// declare success. Without this the Pod can exit with cached writes still
	// in flight, and a downstream consumer (e.g. a VM attaching the restored
	// PVC immediately) may read stale or partial data and fail to boot.
	if err := device.Sync(); err != nil {
		return fmt.Errorf("error syncing device: %w", err)
	}
	return nil
}
