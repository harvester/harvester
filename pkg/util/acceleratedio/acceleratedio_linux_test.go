package acceleratedio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testTimeout = time.Second

type blockingWriter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (w *blockingWriter) WriteAt(_ []byte, _ int64) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return 0, errors.New("injected write failure")
}

type notifyingReader struct {
	blocks    int
	reads     int
	thirdRead chan struct{}
}

func (r *notifyingReader) Read(data []byte) (int, error) {
	if r.reads == r.blocks {
		return 0, io.EOF
	}

	for i := range data {
		data[i] = 1
	}
	r.reads++
	if r.reads == 3 {
		close(r.thirdRead)
	}
	return len(data), nil
}

func TestWriteStream(t *testing.T) {
	data := []byte("0123456789abcdef")
	path := filepath.Join(t.TempDir(), "device")
	device, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()

	var bytesWritten int64
	if err := writeStream(device, bytes.NewReader(data), 4, 2, &bytesWritten); err != nil {
		t.Fatalf("writeStream returned an error: %v", err)
	}
	if bytesWritten != int64(len(data)) {
		t.Fatalf("expected %d dispatched bytes, got %d", len(data), bytesWritten)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, data) {
		t.Fatalf("expected %q, got %q", data, written)
	}
}

func TestWriteStreamReturnsLateWorkerError(t *testing.T) {
	writer := newBlockingWriter()
	result := make(chan error, 1)
	go func() {
		var bytesWritten int64
		result <- writeStream(writer, bytes.NewReader([]byte("data")), 4, 1, &bytesWritten)
	}()

	waitForSignal(t, writer.started, "write worker to start")
	select {
	case err := <-result:
		t.Fatalf("writeStream returned before the worker completed: %v", err)
	default:
	}

	close(writer.release)
	err := waitForResult(t, result)
	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("expected injected write failure, got %v", err)
	}
}

func TestWriteStreamCancelsBlockedDispatch(t *testing.T) {
	writer := newBlockingWriter()
	reader := &notifyingReader{
		blocks:    3,
		thirdRead: make(chan struct{}),
	}
	result := make(chan error, 1)
	go func() {
		var bytesWritten int64
		result <- writeStream(writer, reader, 4, 1, &bytesWritten)
	}()

	waitForSignal(t, writer.started, "write worker to start")
	waitForSignal(t, reader.thirdRead, "dispatcher to fill the task queue")
	close(writer.release)

	err := waitForResult(t, result)
	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("expected injected write failure, got %v", err)
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(testTimeout):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for writeStream")
		return nil
	}
}

// readGate keeps the first read outstanding while reporting all read attempts.
type readGate struct {
	started chan int64
	release chan struct{}
}

func (r *readGate) ReadAt(p []byte, offset int64) (int, error) {
	r.started <- offset
	if offset == 0 {
		<-r.release
	}
	for i := range p {
		p[i] = byte((offset + int64(i)) & 0xff)
	}
	return len(p), nil
}

type streamWriter func([]byte) (int, error)

func (w streamWriter) Write(p []byte) (int, error) { return w(p) }

type streamReader func([]byte, int64) (int, error)

func (r streamReader) ReadAt(p []byte, off int64) (int, error) { return r(p, off) }

func TestReadWindow(t *testing.T) {
	for _, stallOutput := range []bool{false, true} {
		t.Run(fmt.Sprint("stallOutput=", stallOutput), func(t *testing.T) {
			const workers, blockSize = 4, 8
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := &readGate{started: make(chan int64, 100), release: make(chan struct{})}
			outStarted := make(chan struct{})
			outRelease := make(chan struct{})
			var once sync.Once
			out := streamWriter(func(p []byte) (int, error) {
				once.Do(func() { close(outStarted) })
				<-outRelease
				return len(p), nil
			})
			if stallOutput {
				close(r.release)
			}
			result := make(chan error, 1)
			go func() {
				var count int64
				result <- readStream(ctx, r, out, 100*blockSize, blockSize, workers, &count)
			}()
			for i := 0; i < workers; i++ {
				select {
				case <-r.started:
				case <-time.After(testTimeout):
					t.Fatal("read window did not fill")
				}
			}
			if stallOutput {
				waitForSignal(t, outStarted, "blocked output")
			}
			// Every read slot is now held. Neither completed later reads nor a
			// blocked output may release one for a new block.
			select {
			case off := <-r.started:
				t.Errorf("read beyond the window while stalled: offset %d", off)
			case <-time.After(100 * time.Millisecond):
			}
			cancel()
			if !stallOutput {
				close(r.release)
			}
			close(outRelease)
			if err := waitForResult(t, result); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation, got %v", err)
			}
		})
	}
}

func TestReadStreamOrderAndBufferReuse(t *testing.T) {
	const workers, blockSize = 4, 8
	input := make([]byte, 101)
	for i := range input {
		input[i] = byte(i)
	}
	var mu sync.Mutex
	buffers := map[*byte]bool{}
	reader := streamReader(func(p []byte, offset int64) (int, error) {
		mu.Lock()
		buffers[&p[0]] = true
		mu.Unlock()
		return bytes.NewReader(input).ReadAt(p, offset)
	})
	var out bytes.Buffer
	var count int64
	if err := readStream(context.Background(), reader, &out, int64(len(input)), blockSize, workers, &count); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(input, out.Bytes()) || count != int64(len(input)) {
		t.Fatalf("stream mismatch: %v, count=%d", out.Bytes(), count)
	}
	if len(buffers) > workers {
		t.Fatalf("allocated %d buffers for %d workers", len(buffers), workers)
	}
}

func TestReadStreamErrors(t *testing.T) {
	deviceErr := errors.New("device failure")
	outputErr := errors.New("disk full")
	for _, tc := range []struct {
		name     string
		n        int
		readErr  error
		writeN   int
		writeErr error
		want     error
	}{
		{name: "read", n: 0, readErr: deviceErr, want: deviceErr},
		{name: "early EOF", n: 0, readErr: io.EOF, want: io.ErrUnexpectedEOF},
		{name: "short read", n: 2, want: io.ErrUnexpectedEOF},
		{name: "short read EOF", n: 2, readErr: io.EOF, want: io.ErrUnexpectedEOF},
		{name: "full read EOF", n: 4, readErr: io.EOF, writeN: 4},
		{name: "write", n: 4, writeErr: outputErr, want: outputErr},
		{name: "short write", n: 4, writeN: 2, want: io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := streamReader(func([]byte, int64) (int, error) { return tc.n, tc.readErr })
			writer := streamWriter(func([]byte) (int, error) { return tc.writeN, tc.writeErr })
			var count int64
			err := readStream(context.Background(), reader, writer, 4, 4, 4, &count)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestReadStreamEmpty(t *testing.T) {
	var count int64
	var out bytes.Buffer
	if err := readStream(context.Background(), bytes.NewReader(nil), &out, 0, 4, 2, &count); err != nil {
		t.Fatal(err)
	}
	if count != 0 || out.Len() != 0 {
		t.Fatal("empty device emitted data")
	}
}
