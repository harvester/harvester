package repository

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

type object struct {
	key  string
	size int64
}

type objectStore interface {
	close()
	exists(context.Context, string) (bool, error)
	list(context.Context, string) ([]object, bool, error)
	remove(context.Context, string) error
}

type s3Store struct {
	client    *s3.S3
	bucket    string
	transport *http.Transport
}

func newStore(target *settings.BackupTarget, data map[string][]byte) (objectStore, error) {
	config, err := backuputil.ParseS3Target(target)
	if err != nil {
		return nil, err
	}
	// Match operational Jobs: use system CA roots without target-specific proxy
	// or certificate overrides. Keep the controller's ambient proxy out.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	scheme := "https://"
	if config.DisableTLS {
		scheme = "http://"
	}
	region := config.Region
	if region == "" {
		region = "us-east-1"
	}
	clientConfig := aws.NewConfig().WithRegion(region).WithEndpoint(scheme + config.Endpoint).
		WithCredentials(credentials.NewStaticCredentials(string(data[util.AWSAccessKey]), string(data[util.AWSSecretKey]), "")).
		WithS3ForcePathStyle(pathStyle(config)).
		WithHTTPClient(&http.Client{Transport: transport}).WithMaxRetries(2)
	sess, err := session.NewSession(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("creating repository S3 client")
	}
	return &s3Store{client: s3.New(sess), bucket: config.Bucket, transport: transport}, nil
}

func emptyPrefix(ctx context.Context, store objectStore, prefix, config string) error {
	exists, err := store.exists(ctx, prefix+config)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("repository configuration exists but could not be opened")
	}
	objects, more, err := store.list(ctx, prefix)
	if err != nil {
		return err
	}
	if len(objects) == 0 && !more {
		return nil
	}
	if hasOnlyMarker(objects, more, prefix) {
		return store.remove(ctx, prefix)
	}
	return fmt.Errorf("repository prefix contains existing data; refusing initialization")
}

func hasOnlyMarker(objects []object, more bool, prefix string) bool {
	return !more && len(objects) == 1 && objects[0].key == prefix && objects[0].size == 0
}

func (s *s3Store) exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObjectWithContext(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err == nil {
		return true, nil
	}
	if e, ok := err.(awserr.RequestFailure); ok && e.StatusCode() == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("cannot inspect repository configuration")
}

func (s *s3Store) list(ctx context.Context, prefix string) ([]object, bool, error) {
	result, err := s.client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int64(2),
	})
	if err != nil {
		return nil, false, fmt.Errorf("cannot list repository prefix")
	}
	if result.IsTruncated == nil {
		return nil, false, fmt.Errorf("repository prefix listing is incomplete")
	}
	var objects []object
	for _, item := range result.Contents {
		size := int64(-1)
		if item.Size != nil {
			size = *item.Size
		}
		objects = append(objects, object{key: aws.StringValue(item.Key), size: size})
	}
	return objects, aws.BoolValue(result.IsTruncated), nil
}

func (s *s3Store) remove(ctx context.Context, key string) error {
	_, err := s.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("cannot remove empty repository prefix marker")
	}
	return nil
}

func (s *s3Store) close() { s.transport.CloseIdleConnections() }

// Restic and Kopia currently use MinIO's automatic bucket addressing. Keep
// inspection consistent: custom endpoints use paths; standard S3 endpoints use
// DNS unless a dotted bucket would fail TLS hostname verification.
func pathStyle(config *backuputil.S3Config) bool {
	host, _, err := net.SplitHostPort(config.Endpoint)
	if err != nil {
		host = config.Endpoint
	}
	if !config.DisableTLS && strings.Contains(config.Bucket, ".") {
		return true
	}
	if strings.Contains(host, "s3-outposts.") {
		return true
	}
	return host != "storage.googleapis.com" &&
		!strings.HasSuffix(host, ".amazonaws.com") &&
		!strings.HasSuffix(host, ".amazonaws.com.cn") &&
		!strings.HasSuffix(host, ".aliyuncs.com")
}
