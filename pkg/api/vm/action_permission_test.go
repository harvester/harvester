package vm

import (
	"testing"

	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCanDoVMAction(t *testing.T) {
	operator := &accesscontrol.AccessSet{}
	for _, sub := range []string{"virtualmachines/start", "virtualmachines/stop", "virtualmachines/restart"} {
		operator.Add("update", schema.GroupResource{Group: "subresources.kubevirt.io", Resource: sub}, accesscontrol.Access{Namespace: "ns-a", ResourceName: "*"})
	}
	backupOperator := &accesscontrol.AccessSet{}
	backupOperator.Add("create", schema.GroupResource{Group: "harvesterhci.io", Resource: "virtualmachinebackups"}, accesscontrol.Access{Namespace: "ns-a", ResourceName: "*"})

	tests := []struct {
		name      string
		accessSet *accesscontrol.AccessSet
		canUpdate bool
		namespace string
		action    string
		want      bool
	}{
		{name: "operator can start", accessSet: operator, namespace: "ns-a", action: startVM, want: true},
		{name: "operator can force stop", accessSet: operator, namespace: "ns-a", action: forceStopVM, want: true},
		{name: "operator cannot start in another namespace", accessSet: operator, namespace: "ns-b", action: startVM, want: false},
		{name: "operator cannot back up", accessSet: operator, namespace: "ns-a", action: backupVM, want: false},
		{name: "operator cannot clone (unmapped, needs update)", accessSet: operator, namespace: "ns-a", action: cloneVM, want: false},
		{name: "backup operator can back up", accessSet: backupOperator, namespace: "ns-a", action: backupVM, want: true},
		{name: "backup operator can snapshot", accessSet: backupOperator, namespace: "ns-a", action: snapshotVM, want: true},
		{name: "backup operator cannot stop", accessSet: backupOperator, namespace: "ns-a", action: stopVM, want: false},
		{name: "editor keeps unmapped actions", accessSet: &accesscontrol.AccessSet{}, canUpdate: true, namespace: "ns-a", action: cloneVM, want: true},
		{name: "no access set falls back to update", accessSet: nil, canUpdate: true, namespace: "ns-a", action: startVM, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canDoVMAction(tc.accessSet, tc.canUpdate, tc.namespace, "vm-a", tc.action))
		})
	}
}
