package util

import (
	"net/url"
	"strings"
	"sync"

	"github.com/minio/minio-go/v6"
	"github.com/rancher/harvester/pkg/config"
	"github.com/sirupsen/logrus"
)

const (
	BucketName           = "vm-images"
	BucketLocation       = "us-east-1"
	DownloadBucketPolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetBucketLocation","s3:ListBucket"],"Resource":["arn:aws:s3:::vm-images"]},{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::vm-images/*"]}]}`
)

var (
	once sync.Once
)

func NewMinioClient() (*minio.Client, error) {
	var secure bool
	var endpoint = config.ImageStorageEndpoint
	if strings.HasPrefix(endpoint, "http://") ||
		strings.HasPrefix(endpoint, "https://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		endpoint = u.Host
		secure = u.Scheme == "https"
	}
	client, err := minio.New(endpoint, config.ImageStorageAccessKey, config.ImageStorageSecretKey, secure)
	if err != nil {
		return nil, err
	}
	once.Do(func() {
		err = initBucket(client)
	})
	return client, err
}

func initBucket(client *minio.Client) error {
	exist, err := client.BucketExists(BucketName)
	if err != nil {
		return err
	}

	if !exist {
		err = client.MakeBucket(BucketName, BucketLocation)
		if err != nil {
			return err
		}
		logrus.Debugf("Successfully created bucket %s\n", BucketName)
	}

	return client.SetBucketPolicy(BucketName, DownloadBucketPolicy)
}
