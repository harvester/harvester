package drainhelper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func Test_defaultDrainHelper(t *testing.T) {
	assert := require.New(t)
	cfg := &rest.Config{
		Host: "localhost",
	}
	dh, err := defaultDrainHelper(context.TODO(), cfg)
	assert.NoError(err, "expected no error during generation of node helper")
	assert.NotNil(dh, "expected to get a valid drain client")
	assert.True(dh.IgnoreAllDaemonSets, "expected drain helper to ignore daemonsets")
	assert.Equal(defaultGracePeriodSeconds, dh.GracePeriodSeconds, "expected grace period seconds to match predefined constant")
	assert.True(dh.DeleteEmptyDirData, "expected to skip deletion of empty data directory")
	assert.True(dh.Force, "expected force to be set")
	assert.Equal(defaultSkipPodLabels, dh.PodSelector, "expected drain handler pod labels to match const")
}
