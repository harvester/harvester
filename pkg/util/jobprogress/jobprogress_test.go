package jobprogress

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestFetchRestoreProgress(t *testing.T) {
	tests := []struct {
		name string
		logs string
		want int
	}{
		{name: "partial", logs: "IO_PROGRESS mode=WRITE bytes=50 total=100 percent=50.75", want: 50},
		{
			name: "latest line",
			logs: "IO_PROGRESS mode=WRITE bytes=50 total=100 percent=50.00\n" +
				"IO_PROGRESS mode=WRITE bytes=75 total=100 percent=75.00",
			want: 75,
		},
		{name: "stream finished", logs: "IO_PROGRESS mode=WRITE bytes=100 total=100 percent=100.00", want: 99},
		{name: "overreported", logs: "IO_PROGRESS mode=WRITE bytes=100 total=100 percent=101.00", want: 99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/namespaces/default/pods":
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","items":[{"metadata":{"name":"restore-pod"}}]}`)
				case "/api/v1/namespaces/default/pods/restore-pod/log":
					_, _ = fmt.Fprint(w, tt.logs)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			require.NoError(t, err)
			pct, err := FetchRestoreProgress(context.Background(), clientset, "default", "restore")
			require.NoError(t, err)
			require.Equal(t, tt.want, pct)
		})
	}
}

func TestFetchRestoreProgressMissingLogs(t *testing.T) {
	_, err := FetchRestoreProgress(context.Background(), nil, "default", "restore")
	require.Error(t, err)
}
