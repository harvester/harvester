package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathFallsBackToBundledWhenIndexUnreachable(t *testing.T) {
	// ui-index points at a URL that always fails, simulating a
	// misconfigured or unreachable external dashboard index.
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	}))
	defer badServer.Close()

	h := newHandler(
		func() string { return badServer.URL },
		func() string { return "/bundled/ui" },
		func() string { return "external" },
	)

	path, isURL := h.path()
	assert.False(t, isURL, "expected fallback to the bundled UI path")
	assert.Equal(t, "/bundled/ui", path)
}

func TestPathUsesIndexWhenReachable(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))
	defer okServer.Close()

	h := newHandler(
		func() string { return okServer.URL },
		func() string { return "/bundled/ui" },
		func() string { return "external" },
	)

	path, isURL := h.path()
	assert.True(t, isURL)
	assert.Equal(t, okServer.URL, path)
}

func TestPathBundledModeIgnoresIndex(t *testing.T) {
	h := newHandler(
		func() string { return "http://unreachable.invalid" },
		func() string { return "/bundled/ui" },
		func() string { return "bundled" },
	)

	path, isURL := h.path()
	assert.False(t, isURL)
	assert.Equal(t, "/bundled/ui", path)
}

func TestPathAutoModeFallsBackWhenIndexUnreachable(t *testing.T) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	defer badServer.Close()

	h := newHandler(
		func() string { return badServer.URL },
		func() string { return "/bundled/ui" },
		func() string { return "auto" },
	)

	path, isURL := h.path()
	assert.False(t, isURL, "expected fallback to the bundled UI path")
	assert.Equal(t, "/bundled/ui", path)
}

// TestIndexFileServesBundledUIWhenExternalIndexUnreachable mirrors the "To
// Reproduce" steps from harvester/harvester#10224: ui-source is set to
// External and ui-index points at a URL that 404s. It drives the actual
// IndexFile() HTTP handler, the same one that serves a browser reload, and
// asserts the admin gets the packaged dashboard back instead of the raw
// error response that previously left them locked out.
func TestIndexFileServesBundledUIWhenExternalIndexUnreachable(t *testing.T) {
	badServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	}))
	defer badServer.Close()

	bundledDir := t.TempDir()
	const bundledContent = "<html>bundled dashboard</html>"
	require.NoError(t, os.WriteFile(filepath.Join(bundledDir, "index.html"), []byte(bundledContent), 0600))

	h := newHandler(
		func() string { return badServer.URL },
		func() string { return bundledDir },
		func() string { return "external" },
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	h.IndexFile().ServeHTTP(rw, req)

	resp := rw.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), bundledContent)
}

func TestPathFallsBackOnNetworkError(t *testing.T) {
	h := newHandler(
		// No listener on this address, so the request fails at the
		// network level rather than returning an HTTP status.
		func() string { return "http://127.0.0.1:1" },
		func() string { return "/bundled/ui" },
		func() string { return "external" },
	)

	path, isURL := h.path()
	assert.False(t, isURL, "expected fallback to the bundled UI path")
	assert.Equal(t, "/bundled/ui", path)
}
