package iptables

import (
	"testing"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_generatePortString(t *testing.T) {
	assert := require.New(t)

	tests := []struct {
		name     string
		ports    []uint32
		expected string
	}{
		{
			name:     "single port",
			ports:    []uint32{1},
			expected: "1",
		},
		{
			name:     "multiple ports",
			ports:    []uint32{1, 3, 5},
			expected: "1,3,5",
		},
	}

	for _, v := range tests {
		stringPort := generatePortString(v.ports)
		assert.Equal(v.expected, stringPort, "expected generated stringPort to meet expectation", v.name)
	}

}

func Test_emptySecurityGroup(t *testing.T) {
	assert := require.New(t)
	sg := &harvesterv1beta1.SecurityGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-rules",
			Namespace: "default",
		},
	}
	links := []string{"k6t-net1"}
	macSourceAddresses := []string{"46:8c:01:13:38:13"}
	newRules := generateRules(sg, links, macSourceAddresses)
	assert.Len(newRules, 3, "expected to find only one rule")
	assert.Equal(newRules[len(newRules)-1], []string{"-j", "DROP", "-i", links[0], "-m", "state", "--state", "NEW"}, "expected last rule to be drop rule")
}

func Test_ruleWithNoSourcePort(t *testing.T) {
	assert := require.New(t)
	sg := &harvesterv1beta1.SecurityGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-rules",
			Namespace: "default",
		},
		Spec: []harvesterv1beta1.IngressRules{
			{
				SourceAddress: "192.168.0.100",
				IpProtocol:    "tcp",
			},
		},
	}
	links := []string{"k6t-net1"}
	macSourceAddresses := []string{"46:8c:01:13:38:13"}
	newRules := generateRules(sg, links, macSourceAddresses)
	assert.Len(newRules, 4, "expected to find two rules")
	assert.Equal(newRules[len(newRules)-1], []string{"-j", "DROP", "-i", links[0], "-m", "state", "--state", "NEW"}, "expected last rule to be drop rule")
	assert.Equal(newRules[0], []string{"-p", "tcp", "-s", "192.168.0.100", "-i", "k6t-net1", "-j", "ACCEPT", "-m", "state", "--state", "NEW,ESTABLISHED,RELATED"})
}
