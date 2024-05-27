package util

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/docker/docker/api/types"
	v3 "github.com/rancher/rke/types"
)

const proxyEndpointScheme = "https://"

var ecrPattern = regexp.MustCompile(`(^[a-zA-Z0-9][a-zA-Z0-9-_]*)\.dkr\.ecr(\-fips)?\.([a-zA-Z0-9][a-zA-Z0-9-_]*)\.amazonaws\.com(\.cn)?`)

// ECRCredentialPlugin is a wrapper to generate ECR token using the AWS Credentials
func ECRCredentialPlugin(plugin *v3.ECRCredentialPlugin, pr string) (authConfig types.AuthConfig, err error) {
	if plugin == nil {
		err = fmt.Errorf("ECRCredentialPlugin: ECRCredentialPlugin called with nil plugin data")
		return authConfig, err
	}

	logrus.Tracef("ECRCredentialPlugin: ECRCredentialPlugin called with plugin [%v] and pr [%s]", plugin, pr)

	if strings.HasPrefix(pr, proxyEndpointScheme) {
		pr = strings.TrimPrefix(pr, proxyEndpointScheme)
	}
	matches := ecrPattern.FindStringSubmatch(pr)
	if len(matches) == 0 {
		return authConfig, fmt.Errorf("Not a valid ECR registry")
	} else if len(matches) < 3 {
		return authConfig, fmt.Errorf(pr + "is not a valid repository URI for Amazon Elastic Container Registry.")
	}

	config := &aws.Config{
		Region: aws.String(matches[3]),
	}

	logrus.Debugf("ECRCredentialPlugin: Setting Region to [%s]", matches[3])
	var sess *session.Session

	// Use predefined keys and override env lookup if keys are present //
	if plugin.AwsAccessKeyID != "" && plugin.AwsSecretAccessKey != "" {
		// if session token doesn't exist just pass empty string
		config.Credentials = credentials.NewStaticCredentials(plugin.AwsAccessKeyID, plugin.AwsSecretAccessKey, plugin.AwsSessionToken)
		sess, err = session.NewSession(config)
	} else {
		logrus.Debug("ECRCredentialPlugin: aws_access_key_id and aws_secret_access_key keys not in plugin, using IAM role or env variables")
		sess, err = session.NewSessionWithOptions(session.Options{
			Config:            *config,
			SharedConfigState: session.SharedConfigEnable,
		})
	}

	if err != nil {
		logrus.Trace("ECRCredentialPlugin: Error found while constructing auth session, returning authConfig")
		return authConfig, err
	}

	ecrClient := ecr.New(sess)

	result, err := ecrClient.GetAuthorizationToken(&ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return authConfig, err
	}
	if len(result.AuthorizationData) == 0 {
		return authConfig, fmt.Errorf("No authorization data returned")
	}

	authConfig, err = extractToken(*result.AuthorizationData[0].AuthorizationToken)
	return authConfig, err
}

func extractToken(token string) (authConfig types.AuthConfig, err error) {
	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return authConfig, fmt.Errorf("Invalid token: %v", err)
	}

	parts := strings.SplitN(string(decodedToken), ":", 2)
	if len(parts) < 2 {
		return authConfig, fmt.Errorf("Invalid token: expected two parts, got %d", len(parts))
	}

	authConfig = types.AuthConfig{
		Username: parts[0],
		Password: parts[1],
	}

	return authConfig, nil
}
