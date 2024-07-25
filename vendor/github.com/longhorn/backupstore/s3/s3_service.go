package s3

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"

	bhttp "github.com/longhorn/backupstore/http"
)

type service struct {
	Region string
	Bucket string
	Client *http.Client
}

const (
	VirtualHostedStyle = "VIRTUAL_HOSTED_STYLE"
)

func newService(u *url.URL) (*service, error) {
	s := service{}
	if u.User != nil {
		s.Region = u.Host
		s.Bucket = u.User.Username()
	} else {
		//We would depends on AWS_REGION environment variable
		s.Bucket = u.Host
	}

	// add custom ca to http client that is used by s3 service
	customCerts := getCustomCerts()
	client, err := bhttp.GetClientWithCustomCerts(customCerts)
	if err != nil {
		return nil, err
	}
	s.Client = client

	return &s, nil
}

func (s *service) newInstance() (*s3.S3, error) {
	// get custom endpoint
	endpoints := os.Getenv("AWS_ENDPOINTS")
	config := &aws.Config{Region: &s.Region, MaxRetries: aws.Int(3)}

	virtualHostedStyleEnabled := os.Getenv(VirtualHostedStyle)
	if virtualHostedStyleEnabled == "true" {
		config.S3ForcePathStyle = aws.Bool(false)
	} else if virtualHostedStyleEnabled == "false" {
		config.S3ForcePathStyle = aws.Bool(true)
	}

	if endpoints != "" {
		config.Endpoint = aws.String(endpoints)
		if config.S3ForcePathStyle == nil {
			config.S3ForcePathStyle = aws.Bool(true)
		}
	}

	if s.Client != nil {
		config.HTTPClient = s.Client
	}

	ses, err := session.NewSession(config)
	if err != nil {
		return nil, err
	}
	if _, err := ses.Config.Credentials.Get(); err != nil {
		return nil, err
	}
	return s3.New(ses), nil
}

func (s *service) Close() {
}

func parseAwsError(err error) error {
	if awsErr, ok := err.(awserr.Error); ok {
		message := fmt.Sprintln("AWS Error: ", awsErr.Code(), awsErr.Message(), awsErr.OrigErr())
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			message += fmt.Sprintln(reqErr.StatusCode(), reqErr.RequestID())
		}
		return fmt.Errorf(message)
	}
	return err
}

func (s *service) ListObjects(key, delimiter string) ([]*s3.Object, []*s3.CommonPrefix, error) {
	svc, err := s.newInstance()
	if err != nil {
		return nil, nil, err
	}
	defer s.Close()
	// WARNING: Directory must end in "/" in S3, otherwise it may match
	// unintentionally
	params := &s3.ListObjectsInput{
		Bucket:    aws.String(s.Bucket),
		Prefix:    aws.String(key),
		Delimiter: aws.String(delimiter),
	}

	var (
		objects       []*s3.Object
		commonPrefixs []*s3.CommonPrefix
	)
	err = svc.ListObjectsPages(params, func(page *s3.ListObjectsOutput, lastPage bool) bool {
		objects = append(objects, page.Contents...)
		commonPrefixs = append(commonPrefixs, page.CommonPrefixes...)
		return !lastPage
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list objects with param: %+v error: %v",
			params, parseAwsError(err))
	}
	return objects, commonPrefixs, nil
}

func (s *service) HeadObject(key string) (*s3.HeadObjectOutput, error) {
	svc, err := s.newInstance()
	if err != nil {
		return nil, err
	}
	defer s.Close()
	params := &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}
	resp, err := svc.HeadObject(params)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for object: %v response: %v error: %v",
			key, resp.String(), parseAwsError(err))
	}
	return resp, nil
}

func (s *service) PutObject(key string, reader io.ReadSeeker) error {
	svc, err := s.newInstance()
	if err != nil {
		return err
	}
	defer s.Close()

	svc.Client.Config.Retryer = client.DefaultRetryer{
		NumMaxRetries:    10,
		MinRetryDelay:    500 * time.Millisecond,
		MinThrottleDelay: 1 * time.Second,
		MaxRetryDelay:    300 * time.Second,
		MaxThrottleDelay: 600 * time.Second,
	}

	params := &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
		Body:   reader,
	}

	resp, err := svc.PutObject(params)
	if err != nil {
		return fmt.Errorf("failed to put object: %v response: %v error: %v",
			key, resp.String(), parseAwsError(err))
	}
	return nil
}

func (s *service) GetObject(key string) (io.ReadCloser, error) {
	svc, err := s.newInstance()
	if err != nil {
		return nil, err
	}
	defer s.Close()

	params := &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}

	resp, err := svc.GetObject(params)
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %v response: %v error: %v",
			key, resp.String(), parseAwsError(err))
	}

	return resp.Body, nil
}

func (s *service) DeleteObjects(key string) error {

	objects, _, err := s.ListObjects(key, "")
	if err != nil {
		return errors.Wrapf(err, "failed to list objects with prefix %v before removing them", key)
	}

	svc, err := s.newInstance()
	if err != nil {
		return errors.Wrap(err, "failed to get a new s3 client instance before removing objects")
	}
	defer s.Close()

	var deletionFailures []string
	for _, object := range objects {
		resp, err := svc.DeleteObject(&s3.DeleteObjectInput{
			Bucket: aws.String(s.Bucket),
			Key:    object.Key,
		})

		if err != nil {
			log.Errorf("Failed to delete object: %v response: %v error: %v",
				aws.StringValue(object.Key), resp.String(), parseAwsError(err))
			deletionFailures = append(deletionFailures, aws.StringValue(object.Key))
		}
	}

	if len(deletionFailures) > 0 {
		return fmt.Errorf("failed to delete objects %v", deletionFailures)
	}

	return nil
}
