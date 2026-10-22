package acceleratedio

import (
	"bytes"
	"errors"
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
