package repository

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

func TestStorageEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		head int
		list int
		body string
		fail bool
	}{
		{"absent and empty", 404, 200, `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`, false},
		{"config exists", 200, 200, "", true},
		{"access denied", 403, 200, "", true},
		{"list denied", 404, 403, "", true},
		{"unknown response", 500, 200, "", true},
		{"incomplete listing", 404, 200, `<ListBucketResult/>`, true},
		{"truncated", 404, 200, `<ListBucketResult><IsTruncated>true</IsTruncated></ListBucketResult>`, true},
		{"remaining data", 404, 200, `<ListBucketResult><Contents><Key>restic/data/x</Key><Size>1</Size></Contents></ListBucketResult>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodHead:
					w.WriteHeader(tc.head)
				case http.MethodGet:
					require.Equal(t, "restic/", r.URL.Query().Get("prefix"))
					w.WriteHeader(tc.list)
					_, _ = fmt.Fprint(w, tc.body)
				default:
					t.Errorf("unexpected mutation: %s", r.Method)
				}
			}))
			defer server.Close()
			target := repoTarget()
			target.Endpoint = server.URL
			store, err := newStore(target, repoData())
			require.NoError(t, err)
			defer store.close()
			err = emptyPrefix(context.Background(), store, "restic/", "config")
			require.Equal(t, tc.fail, err != nil, "%v", err)
		})
	}
}

func TestInspectionUsesJobAddressing(t *testing.T) {
	for _, tc := range []struct {
		endpoint, bucket string
		tls, path        bool
	}{
		{"minio.example:9000", "backups", false, true},
		{"s3.amazonaws.com", "backups", true, false},
		{"s3.us-west-2.amazonaws.com:443", "backups", true, false},
		{"s3.cn-north-1.amazonaws.com.cn", "backups", true, false},
		{"storage.googleapis.com", "backups", true, false},
		{"oss-cn-hangzhou.aliyuncs.com", "backups", true, false},
		{"s3.amazonaws.com", "dotted.bucket", true, true},
	} {
		require.Equal(t, tc.path, pathStyle(&backuputil.S3Config{
			Endpoint: tc.endpoint, Bucket: tc.bucket, DisableTLS: !tc.tls,
		}))
	}
}

func TestUntrustedTLSBlocksInitialization(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	target := repoTarget()
	target.Endpoint = server.URL
	calls := 0
	manager := Manager{run: func(_ context.Context, cmd command) error {
		calls++
		return &commandError{engine: cmd.engine, action: cmd.action, code: 10}
	}}
	require.ErrorContains(t, manager.Init(context.Background(), target, repoData()), "cannot inspect")
	require.Equal(t, 1, calls, "a TLS inspection failure must not start initialization")
}
