package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHarvesterUpgradeImage(t *testing.T) {
	assert.Equal(t, "rancher/harvester-upgrade:v1.9.1", GetHarvesterUpgradeImage("", "v1.9.1"))
	assert.Equal(t, "registry.example.com/harvester/harvester-upgrade:v1.9.1", GetHarvesterUpgradeImage("registry.example.com", "v1.9.1"))
	assert.Equal(t, "registry.example.com:5000/prime/harvester/harvester-upgrade:v1.9.1", GetHarvesterUpgradeImage("registry.example.com:5000/prime", "v1.9.1"))
}
