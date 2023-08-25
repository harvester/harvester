package controllers

import (
	"context"
	"fmt"

	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util/iptables"
)

var (
	attachedSecurityGroup = ""
)

type VMNetworkPolicyHandler struct {
	ctx          context.Context
	vmName       string
	vmNamespace  string
	vmController ctlkubevirtv1.VirtualMachineController
	sgController ctlharvesterv1.SecurityGroupController
}

func (f *VMNetworkPolicyHandler) Register() error {
	f.vmController.OnChange(f.ctx, "manager-vmnetworkpolicy-rules", f.manageRules)
	relatedresource.Watch(f.ctx, "watch-security-group", f.watchSecurityGroup, f.vmController, f.sgController)
	return nil
}

// manageRules runs pre-flight checks to ensure we are reconcilling the correct VM
func (f *VMNetworkPolicyHandler) manageRules(_ string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	if vm == nil || vm.DeletionTimestamp != nil {
		return nil, nil
	}

	if vm.Name == f.vmName && vm.Namespace == f.vmNamespace {
		// reconcile rules
		var ok bool
		attachedSecurityGroup, ok = vm.Annotations[harvesterv1beta1.SecurityGroupPrefix]
		if !ok {
			logrus.Debugf("no security group attached to vm %s/%s.. ignoring", vm.Namespace, vm.Name)
			return vm, nil
		}
		return f.reconcileRules(vm)
	}

	logrus.Debugf("ignore vm %s%s since its not matching current vm %s/%s", vm.Namespace, vm.Name, f.vmNamespace, f.vmName)
	return vm, nil
}

// reconcileRules ensures securityGroup definition matches current rule definition
func (f *VMNetworkPolicyHandler) reconcileRules(vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	sgObj, err := f.sgController.Get(vm.Namespace, attachedSecurityGroup, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return vm, nil
		}

		return vm, fmt.Errorf("error looking up security group %s/%s: %v", vm.Namespace, attachedSecurityGroup, err)
	}

	err = iptables.ApplyRules(sgObj, vm)
	return vm, err
}

// watchSecurityGroup will watch changes to security group, and requeue vm object
// to ensure iptables rules are updated to reflect current definitions
func (f *VMNetworkPolicyHandler) watchSecurityGroup(_ string, _ string, obj runtime.Object) ([]relatedresource.Key, error) {
	// empty securityGroup, nothing to watch yet, return early
	if attachedSecurityGroup == "" {
		return nil, nil
	}
	if sg, ok := obj.(*harvesterv1beta1.SecurityGroup); ok {
		if sg.Name == attachedSecurityGroup {
			return []relatedresource.Key{{
				Name:      f.vmName,
				Namespace: f.vmNamespace,
			},
			}, nil
		}
	}
	return nil, nil
}
