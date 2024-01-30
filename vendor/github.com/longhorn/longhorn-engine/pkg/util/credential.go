package util

import (
	"errors"
	"os"

	"github.com/longhorn/longhorn-engine/pkg/types"
)

func SetupCredential(backupType string, credential map[string]string) error {
	switch backupType {
	case "s3":
		return setupS3Credential(credential)
	case "cifs":
		return setupCIFSCredential(credential)
	case "azblob":
		return setupAZBlobCredential(credential)
	default:
		return nil
	}
}

func setupS3Credential(credential map[string]string) error {
	if credential == nil {
		return nil
	}

	if credential[types.AWSAccessKey] == "" && credential[types.AWSSecretKey] != "" {
		return errors.New("s3 credential access key not found")
	}
	if credential[types.AWSAccessKey] != "" && credential[types.AWSSecretKey] == "" {
		return errors.New("s3 credential secret access key not found")
	}
	if credential[types.AWSAccessKey] != "" && credential[types.AWSSecretKey] != "" {
		os.Setenv(types.AWSAccessKey, credential[types.AWSAccessKey])
		os.Setenv(types.AWSSecretKey, credential[types.AWSSecretKey])
	}

	os.Setenv(types.AWSEndPoint, credential[types.AWSEndPoint])
	os.Setenv(types.HTTPSProxy, credential[types.HTTPSProxy])
	os.Setenv(types.HTTPProxy, credential[types.HTTPProxy])
	os.Setenv(types.NOProxy, credential[types.NOProxy])
	os.Setenv(types.VirtualHostedStyle, credential[types.VirtualHostedStyle])

	// set a custom ca cert if available
	if credential[types.AWSCert] != "" {
		os.Setenv(types.AWSCert, credential[types.AWSCert])
	}

	return nil
}

func setupCIFSCredential(credential map[string]string) error {
	if credential == nil {
		return nil
	}

	os.Setenv(types.CIFSUsername, credential[types.CIFSUsername])
	os.Setenv(types.CIFSPassword, credential[types.CIFSPassword])

	return nil
}

func setupAZBlobCredential(credential map[string]string) error {
	if credential == nil {
		return nil
	}

	if credential[types.AZBlobAccountName] == "" && credential[types.AZBlobAccountKey] != "" {
		return errors.New("Azure Blob Storage credential account name not found")
	}
	if credential[types.AZBlobAccountName] != "" && credential[types.AZBlobAccountKey] == "" {
		return errors.New("Azure Blob Storage credential account key not found")
	}

	os.Setenv(types.AZBlobAccountName, credential[types.AZBlobAccountName])
	os.Setenv(types.AZBlobAccountKey, credential[types.AZBlobAccountKey])
	os.Setenv(types.AZBlobEndpoint, credential[types.AZBlobEndpoint])
	os.Setenv(types.HTTPSProxy, credential[types.HTTPSProxy])
	os.Setenv(types.HTTPProxy, credential[types.HTTPProxy])
	os.Setenv(types.NOProxy, credential[types.NOProxy])

	if credential[types.AZBlobCert] != "" {
		os.Setenv(types.AZBlobCert, credential[types.AZBlobCert])
	}

	return nil
}

func getCredentialFromEnvVars(backupType string) (map[string]string, error) {
	switch backupType {
	case "s3":
		return getS3CredentialFromEnvVars()
	case "cifs":
		return getCIFSCredentialFromEnvVars()
	case "azblob":
		return getAZBlobCredentialFromEnvVars()
	default:
		return nil, nil
	}
}

func getAZBlobCredentialFromEnvVars() (map[string]string, error) {
	credential := map[string]string{}

	credential[types.AZBlobAccountName] = os.Getenv(types.AZBlobAccountName)
	credential[types.AZBlobAccountKey] = os.Getenv(types.AZBlobAccountKey)
	credential[types.AZBlobEndpoint] = os.Getenv(types.AZBlobEndpoint)
	credential[types.AZBlobCert] = os.Getenv(types.AZBlobCert)
	credential[types.HTTPSProxy] = os.Getenv(types.HTTPSProxy)
	credential[types.HTTPProxy] = os.Getenv(types.HTTPProxy)
	credential[types.NOProxy] = os.Getenv(types.NOProxy)

	return credential, nil
}

func getCIFSCredentialFromEnvVars() (map[string]string, error) {
	credential := map[string]string{}

	credential[types.CIFSUsername] = os.Getenv(types.CIFSUsername)
	credential[types.CIFSPassword] = os.Getenv(types.CIFSPassword)

	return credential, nil
}

func getS3CredentialFromEnvVars() (map[string]string, error) {
	credential := map[string]string{}

	accessKey := os.Getenv(types.AWSAccessKey)
	secretKey := os.Getenv(types.AWSSecretKey)
	if accessKey == "" && secretKey != "" {
		return nil, errors.New("s3 credential access key not found")
	}
	if accessKey != "" && secretKey == "" {
		return nil, errors.New("s3 credential secret access key not found")
	}
	if accessKey != "" && secretKey != "" {
		credential[types.AWSAccessKey] = accessKey
		credential[types.AWSSecretKey] = secretKey
	}

	credential[types.AWSEndPoint] = os.Getenv(types.AWSEndPoint)
	credential[types.AWSCert] = os.Getenv(types.AWSCert)
	credential[types.HTTPSProxy] = os.Getenv(types.HTTPSProxy)
	credential[types.HTTPProxy] = os.Getenv(types.HTTPProxy)
	credential[types.NOProxy] = os.Getenv(types.NOProxy)
	credential[types.VirtualHostedStyle] = os.Getenv(types.VirtualHostedStyle)

	return credential, nil
}

func GetBackupCredential(backupURL string) (map[string]string, error) {
	backupType, err := CheckBackupType(backupURL)
	if err != nil {
		return nil, err
	}

	return getCredentialFromEnvVars(backupType)
}
