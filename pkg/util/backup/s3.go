package backup

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/harvester/harvester/pkg/settings"
)

// S3Config describes the connection without an engine-specific repository prefix.
type S3Config struct {
	Bucket     string
	Endpoint   string
	Region     string
	DisableTLS bool
}

// ParseS3Target validates the target and normalizes its endpoint to host[:port].
func ParseS3Target(target *settings.BackupTarget) (*S3Config, error) {
	if target == nil {
		return nil, fmt.Errorf("backup target is nil")
	}
	if target.Type != settings.S3BackupType {
		return nil, fmt.Errorf("expected %s backup target, got %s", settings.S3BackupType, target.Type)
	}
	endpoint, disableTLS, err := normalizeS3Endpoint(target.Endpoint)
	if err != nil {
		return nil, err
	}
	if target.BucketName == "" {
		return nil, fmt.Errorf("s3 backup target bucket name is empty")
	}
	return &S3Config{
		Bucket:     target.BucketName,
		Endpoint:   endpoint,
		Region:     target.BucketRegion,
		DisableTLS: disableTLS,
	}, nil
}

func normalizeS3Endpoint(endpoint string) (string, bool, error) {
	endpoint = strings.TrimRight(endpoint, "/")
	if endpoint == "" {
		return "s3.amazonaws.com", false, nil
	}
	if !strings.Contains(endpoint, "://") {
		return endpoint, false, nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("failed to parse s3 backup target endpoint %q: %w", endpoint, err)
	}
	if parsed.Host == "" {
		return "", false, fmt.Errorf("s3 backup target endpoint %q is missing host", endpoint)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false, fmt.Errorf("s3 endpoint must not include a path, got %q", endpoint)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("s3 endpoint must not include query or fragment, got %q", endpoint)
	}

	switch parsed.Scheme {
	case "http":
		return parsed.Host, true, nil
	case "https":
		return parsed.Host, false, nil
	default:
		return "", false, fmt.Errorf("unsupported s3 backup target endpoint scheme %q", parsed.Scheme)
	}
}
