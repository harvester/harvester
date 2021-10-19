package s3

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/longhorn/backupstore"
	"github.com/longhorn/backupstore/http"
	"github.com/sirupsen/logrus"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "s3"})
)

type BackupStoreDriver struct {
	destURL string
	path    string
	service Service
}

const (
	KIND = "s3"
)

func init() {
	if err := backupstore.RegisterDriver(KIND, initFunc); err != nil {
		panic(err)
	}
}

func initFunc(destURL string) (backupstore.BackupStoreDriver, error) {
	b := &BackupStoreDriver{}

	u, err := url.Parse(destURL)
	if err != nil {
		return nil, err
	}

	if u.Scheme != KIND {
		return nil, fmt.Errorf("BUG: Why dispatch %v to %v?", u.Scheme, KIND)
	}

	if u.User != nil {
		b.service.Region = u.Host
		b.service.Bucket = u.User.Username()
	} else {
		//We would depends on AWS_REGION environment variable
		b.service.Bucket = u.Host
	}
	b.path = u.Path
	if b.service.Bucket == "" || b.path == "" {
		return nil, fmt.Errorf("Invalid URL. Must be either s3://bucket@region/path/, or s3://bucket/path")
	}

	// add custom ca to http client that is used by s3 service
	if customCerts := getCustomCerts(); customCerts != nil {
		client, err := http.GetClientWithCustomCerts(customCerts)
		if err != nil {
			return nil, err
		}
		b.service.Client = client
	}

	//Leading '/' can cause mystery problems for s3
	b.path = strings.TrimLeft(b.path, "/")

	//Test connection
	if _, err := b.List(""); err != nil {
		return nil, err
	}

	b.destURL = KIND + "://" + b.service.Bucket
	if b.service.Region != "" {
		b.destURL += "@" + b.service.Region
	}
	b.destURL += "/" + b.path

	log.Debugf("Loaded driver for %v", b.destURL)
	return b, nil
}

func getCustomCerts() []byte {
	// Certificates in PEM format (base64)
	certs := os.Getenv("AWS_CERT")
	if certs == "" {
		return nil
	}

	return []byte(certs)
}

func (s *BackupStoreDriver) Kind() string {
	return KIND
}

func (s *BackupStoreDriver) GetURL() string {
	return s.destURL
}

func (s *BackupStoreDriver) updatePath(path string) string {
	return filepath.Join(s.path, path)
}

func (s *BackupStoreDriver) List(listPath string) ([]string, error) {
	var result []string

	path := s.updatePath(listPath) + "/"
	contents, prefixes, err := s.service.ListObjects(path, "/")
	if err != nil {
		log.Error("Fail to list s3: ", err)
		return result, err
	}

	sizeC := len(contents)
	sizeP := len(prefixes)
	if sizeC == 0 && sizeP == 0 {
		return result, nil
	}
	result = []string{}
	for _, obj := range contents {
		r := strings.TrimPrefix(*obj.Key, path)
		if r != "" {
			result = append(result, r)
		}
	}
	for _, p := range prefixes {
		r := strings.TrimPrefix(*p.Prefix, path)
		r = strings.TrimSuffix(r, "/")
		if r != "" {
			result = append(result, r)
		}
	}

	return result, nil
}

func (s *BackupStoreDriver) FileExists(filePath string) bool {
	return s.FileSize(filePath) >= 0
}

func (s *BackupStoreDriver) FileSize(filePath string) int64 {
	path := s.updatePath(filePath)
	head, err := s.service.HeadObject(path)
	if err != nil || head.ContentLength == nil {
		return -1
	}
	return *head.ContentLength
}

func (s *BackupStoreDriver) FileTime(filePath string) time.Time {
	path := s.updatePath(filePath)
	head, err := s.service.HeadObject(path)
	if err != nil || head.ContentLength == nil {
		return time.Time{}
	}
	return aws.TimeValue(head.LastModified).UTC()
}

func (s *BackupStoreDriver) Remove(path string) error {
	return s.service.DeleteObjects(s.updatePath(path))
}

func (s *BackupStoreDriver) Read(src string) (io.ReadCloser, error) {
	path := s.updatePath(src)
	rc, err := s.service.GetObject(path)
	if err != nil {
		return nil, err
	}
	return rc, nil
}

func (s *BackupStoreDriver) Write(dst string, rs io.ReadSeeker) error {
	path := s.updatePath(dst)
	return s.service.PutObject(path, rs)
}

func (s *BackupStoreDriver) Upload(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return nil
	}
	defer file.Close()
	path := s.updatePath(dst)
	return s.service.PutObject(path, file)
}

func (s *BackupStoreDriver) Download(src, dst string) error {
	if _, err := os.Stat(dst); err != nil {
		os.Remove(dst)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	path := s.updatePath(src)
	rc, err := s.service.GetObject(path)
	if err != nil {
		return err
	}
	defer rc.Close()

	_, err = io.Copy(f, rc)
	if err != nil {
		return err
	}
	return nil
}
