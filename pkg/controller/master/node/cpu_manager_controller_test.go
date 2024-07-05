package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetCPUManagerUpdateStatus(t *testing.T) {
	_, err := GetCPUManagerUpdateStatus(`{"policy":"foo","status":"requested"}`)
	assert.Equal(t, "invalid policy", err.Error())

	_, err = GetCPUManagerUpdateStatus(`{"policy":"static","status":"bar"}`)
	assert.Equal(t, "invalid status", err.Error())

	updateStatus, err := GetCPUManagerUpdateStatus(`{"policy":"static","status":"requested"}`)
	assert.Nil(t, err)
	assert.Equal(t, updateStatus.Policy, CPUManagerStaticPolicy)
	assert.Equal(t, updateStatus.Status, CPUManagerRequestedStatus)
}
