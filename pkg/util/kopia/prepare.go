package kopia

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/harvester/harvester/pkg/util"
)

const (
	PrepareSubcommand = "kopia-prepare"
	defaultS3Region   = "us-east-1"
	inspectLimit      = int64(2)
)

type storedObject struct {
	key     string
	size    int64
	hasSize bool
}

type objectStore interface {
	list(context.Context, string, string, int64) ([]storedObject, bool, error)
	remove(context.Context, string, string) error
}

type s3Store struct {
	client *s3.S3
}

// Prepare makes a new Kopia repository prefix safe to initialize. It removes
// only the empty S3 directory marker that some object-store UIs create.
func Prepare(ctx context.Context) error {
	repository, err := repoFromEnv()
	if err != nil {
		return err
	}

	accessKey, err := requiredEnv(util.AWSAccessKey)
	if err != nil {
		return err
	}
	secretKey, err := requiredEnv(util.AWSSecretKey)
	if err != nil {
		return err
	}

	store, err := newStore(repository, accessKey, secretKey)
	if err != nil {
		return err
	}
	return prepareStore(ctx, store, repository)
}

func repoFromEnv() (*Repository, error) {
	bucket, err := requiredEnv(BucketKey)
	if err != nil {
		return nil, err
	}
	endpoint, err := requiredEnv(EndpointKey)
	if err != nil {
		return nil, err
	}
	prefix, err := requiredEnv(PrefixKey)
	if err != nil {
		return nil, err
	}
	disableTLS, err := strconv.ParseBool(os.Getenv(DisableTLSKey))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", DisableTLSKey, err)
	}

	return &Repository{
		Bucket:     bucket,
		Endpoint:   endpoint,
		Region:     os.Getenv(RegionKey),
		Prefix:     prefix,
		DisableTLS: disableTLS,
	}, nil
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s is empty", name)
	}
	return value, nil
}

func newStore(repository *Repository, accessKey, secretKey string) (*s3Store, error) {
	region := repository.Region
	if region == "" {
		region = defaultS3Region
	}

	config := aws.NewConfig().
		WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")).
		WithEndpoint(repositoryURL(repository)).
		WithRegion(region).
		WithS3ForcePathStyle(true)
	sess, err := session.NewSession(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 session: %w", err)
	}
	return &s3Store{client: s3.New(sess)}, nil
}

func repositoryURL(repository *Repository) string {
	scheme := "https"
	if repository.DisableTLS {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, repository.Endpoint)
}

func prepareStore(ctx context.Context, store objectStore, repository *Repository) error {
	objects, truncated, err := store.list(
		ctx,
		repository.Bucket,
		repository.Prefix,
		inspectLimit,
	)
	if err != nil {
		return fmt.Errorf("failed to inspect Kopia repository prefix: %w", err)
	}
	if len(objects) == 0 && !truncated {
		return nil
	}
	if truncated || len(objects) != 1 || !isPrefixMarker(objects[0], repository.Prefix) {
		return fmt.Errorf(
			"refusing to initialize Kopia repository: s3://%s/%s contains existing data",
			repository.Bucket,
			repository.Prefix,
		)
	}

	if err := store.remove(ctx, repository.Bucket, repository.Prefix); err != nil {
		return fmt.Errorf("failed to remove empty Kopia prefix marker: %w", err)
	}
	return nil
}

func isPrefixMarker(object storedObject, prefix string) bool {
	return object.key == prefix && object.hasSize && object.size == 0
}

func (s *s3Store) list(
	ctx context.Context,
	bucket, prefix string,
	limit int64,
) ([]storedObject, bool, error) {
	output, err := s.client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int64(limit),
		Prefix:  aws.String(prefix),
	})
	if err != nil {
		return nil, false, err
	}

	objects := make([]storedObject, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, storedObject{
			key:     aws.StringValue(object.Key),
			size:    aws.Int64Value(object.Size),
			hasSize: object.Size != nil,
		})
	}
	return objects, aws.BoolValue(output.IsTruncated), nil
}

func (s *s3Store) remove(ctx context.Context, bucket, key string) error {
	_, err := s.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}
