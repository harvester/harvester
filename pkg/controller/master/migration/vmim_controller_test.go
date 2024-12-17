//go:build linux && amd64

package migration

import (
	"testing"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakecore "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	fakegenerated "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlvirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	resourceQuotaNamespace = "rs"
	resourceQuotaName      = "rq1"

	errMockTrackerAdd = "mock resource should add into fake controller tracker"
	errMockTrackerGet = "mock resource should get into fake controller tracker"

	vmName       = "vm1"
	migrationUID = "f560ca71-7ecb-4c0e-8614-b63111ef92c4"
)

func TestHandler_OnVmimChanged_WithResourceQuota(t *testing.T) {

	type fields struct {
		rqs      ctlharvcorev1.ResourceQuotaClient
		rqCache  ctlharvcorev1.ResourceQuotaCache
		vms      ctlvirtv1.VirtualMachineClient
		vmCache  ctlvirtv1.VirtualMachineCache
		vmis     ctlvirtv1.VirtualMachineInstanceClient
		vmiCache ctlvirtv1.VirtualMachineInstanceCache
		podCache ctlcorev1.PodCache
	}
	type args struct {
		rq   *corev1.ResourceQuota
		vm   *kubevirtv1.VirtualMachine
		vmi  *kubevirtv1.VirtualMachineInstance
		vmim *kubevirtv1.VirtualMachineInstanceMigration
	}

	getOriginRQ := func() *corev1.ResourceQuota {
		return &corev1.ResourceQuota{
			ObjectMeta: v1.ObjectMeta{
				Namespace: resourceQuotaNamespace,
				Name:      resourceQuotaName,
				Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
					corev1.ResourceLimitsMemory: *resource.NewQuantity(1297436672, resource.BinarySI),
				},
			},
		}
	}

	getScaledRQ := func() *corev1.ResourceQuota {
		return &corev1.ResourceQuota{
			ObjectMeta: v1.ObjectMeta{
				Annotations: map[string]string{
					util.AnnotationMigratingPrefix + vmName: `{"limits.cpu":"1","limits.memory":"1266Mi"}`,
				},
				Namespace: resourceQuotaNamespace,
				Name:      resourceQuotaName,
				Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceLimitsCPU:    *resource.NewQuantity(2, resource.DecimalSI),
					corev1.ResourceLimitsMemory: *resource.NewQuantity(2624933888, resource.BinarySI),
				},
			},
		}
	}

	getNormalVMI := func() *kubevirtv1.VirtualMachineInstance {
		return &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: v1.ObjectMeta{
				Name:      vmName,
				Namespace: resourceQuotaNamespace,
			},
			Spec: kubevirtv1.VirtualMachineInstanceSpec{
				Domain: kubevirtv1.DomainSpec{
					Resources: kubevirtv1.ResourceRequirements{
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
							corev1.ResourceMemory: *resource.NewQuantity(1073741824, resource.BinarySI),
						},
					},
				},
			},
		}
	}

	getMigrationSucceededVMI := func() *kubevirtv1.VirtualMachineInstance {
		return &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: v1.ObjectMeta{
				Name:      vmName,
				Namespace: resourceQuotaNamespace,
				Annotations: map[string]string{
					util.AnnotationMigrationUID: migrationUID,
				},
			},
			Spec: kubevirtv1.VirtualMachineInstanceSpec{
				Domain: kubevirtv1.DomainSpec{
					Resources: kubevirtv1.ResourceRequirements{
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
							corev1.ResourceMemory: *resource.NewQuantity(1073741824, resource.BinarySI),
						},
					},
				},
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
					MigrationUID: migrationUID,
					Completed:    true,
				},
			},
		}
	}

	/*
		getMigrationAbortedVMI := func() *kubevirtv1.VirtualMachineInstance {
			return &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: v1.ObjectMeta{
					Name:      vmName,
					Namespace: resourceQuotaNamespace,
					Annotations: map[string]string{
						util.AnnotationMigrationUID: migrationUID,
					},
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Resources: kubevirtv1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
								corev1.ResourceMemory: *resource.NewQuantity(1073741824, resource.BinarySI),
							},
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
						MigrationUID: migrationUID,
						Completed: true,
						AbortStatus: kubevirtv1.MigrationAbortSucceeded,
					},
				},
			}
		}
	*/

	getArgs := func(vmimPhase kubevirtv1.VirtualMachineInstanceMigrationPhase, vmi *kubevirtv1.VirtualMachineInstance, rq *corev1.ResourceQuota) *args {
		return &args{
			rq: rq,
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: v1.ObjectMeta{
					Name:      vmName,
					Namespace: resourceQuotaNamespace,
				},
			},
			vmi: vmi,
			vmim: &kubevirtv1.VirtualMachineInstanceMigration{
				ObjectMeta: v1.ObjectMeta{
					Name:      vmName,
					Namespace: resourceQuotaNamespace,
					UID:       "f560ca71-7ecb-4c0e-8614-b63111ef92c4",
				},
				Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: vmName},
				Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
					Phase: vmimPhase,
				},
			},
		}
	}

	tests := []struct {
		name            string
		fields          fields
		args            *args
		wantErr         bool
		want            *corev1.ResourceQuota
		wantVMTsUpdated bool
		baseVMTs        string
		onChangeVMI     bool
		wantVMIErr      bool
		abort           bool
	}{
		{
			name:            "In Pending",
			fields:          fields{},
			args:            getArgs(kubevirtv1.MigrationPending, getNormalVMI(), getOriginRQ()),
			wantErr:         false,
			want:            getScaledRQ(),
			wantVMTsUpdated: true,
			baseVMTs:        time.Now().Format(time.RFC3339),
			onChangeVMI:     false,
			wantVMIErr:      false,
			abort:           true,
		},
		{
			name:            "In Scheduling",
			fields:          fields{},
			args:            getArgs(kubevirtv1.MigrationScheduling, getNormalVMI(), getScaledRQ()),
			wantErr:         false,
			want:            getScaledRQ(),
			wantVMTsUpdated: true,
			baseVMTs:        time.Now().Format(time.RFC3339),
			onChangeVMI:     false,
			wantVMIErr:      false,
			abort:           true,
		},
		{
			name:            "Scheduled",
			fields:          fields{},
			args:            getArgs(kubevirtv1.MigrationScheduled, getNormalVMI(), getScaledRQ()),
			wantErr:         false,
			want:            getScaledRQ(),
			wantVMTsUpdated: false, // VM is not expected to be forcely synced
			baseVMTs:        time.Now().Format(time.RFC3339),
			onChangeVMI:     false,
			wantVMIErr:      false,
			abort:           true,
		},
		{
			name:            "Running",
			fields:          fields{},
			args:            getArgs(kubevirtv1.MigrationRunning, getNormalVMI(), getScaledRQ()),
			wantErr:         false,
			want:            getScaledRQ(),
			wantVMTsUpdated: false, // VM is not expected to be forcely synced
			baseVMTs:        time.Now().Format(time.RFC3339),
			onChangeVMI:     false,
			wantVMIErr:      false,
			abort:           true,
		},
		{
			name:            "Succeeded",
			fields:          fields{},
			args:            getArgs(kubevirtv1.MigrationSucceeded, getMigrationSucceededVMI(), getScaledRQ()),
			wantErr:         false,
			want:            getOriginRQ(),
			wantVMTsUpdated: true,
			baseVMTs:        time.Now().Format(time.RFC3339),
			onChangeVMI:     true,
			wantVMIErr:      false,
			abort:           false,
		},
		// There is no podCache ATM, leave this case TBD.
		/*
			{
				name:   "Aborted",
				fields: fields{},
				args: getArgs(kubevirtv1.MigrationFailed, getMigrationAbortedVMI(), getScaledRQ()),
				wantErr: false,
				want: getOriginRQ(),
				wantVMTsUpdated: true,
				baseVMTs:        time.Now().Format(time.RFC3339),
				onChangeVMI: true,
				wantVMIErr: false,
				abort: false,
			},
		*/
	}
	// ensure timestamp can be compared
	time.Sleep(1 * time.Second)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clientset = fakecore.NewSimpleClientset()
			var harvFakeClient = fakegenerated.NewSimpleClientset()

			if tt.args.rq != nil {
				err := clientset.Tracker().Add(tt.args.rq)
				assert.Nil(t, err, errMockTrackerAdd)
			}
			if tt.args.vm != nil {
				err := harvFakeClient.Tracker().Add(tt.args.vm)
				assert.Nil(t, err, errMockTrackerAdd)
			}
			if tt.args.vmi != nil {
				err := harvFakeClient.Tracker().Add(tt.args.vmi)
				assert.Nil(t, err, errMockTrackerAdd)
			}
			if tt.args.vmim != nil {
				err := harvFakeClient.Tracker().Add(tt.args.vmim)
				assert.Nil(t, err, errMockTrackerAdd)
			}

			h := &Handler{
				rqs:      fakeclients.ResourceQuotaClient(clientset.CoreV1().ResourceQuotas),
				rqCache:  fakeclients.ResourceQuotaCache(clientset.CoreV1().ResourceQuotas),
				vms:      fakeclients.VirtualMachineClient(harvFakeClient.KubevirtV1().VirtualMachines),
				vmCache:  fakeclients.VirtualMachineCache(harvFakeClient.KubevirtV1().VirtualMachines),
				vmis:     fakeclients.VirtualMachineInstanceClient(harvFakeClient.KubevirtV1().VirtualMachineInstances),
				vmiCache: fakeclients.VirtualMachineInstanceCache(harvFakeClient.KubevirtV1().VirtualMachineInstances),
			}

			_, err := h.OnVmimChanged("", tt.args.vmim)
			if (err != nil) != tt.wantErr {
				t.Errorf("OnVmimChanged() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// in successed/aborted cases, vmi onChange will further sync VM
			if tt.onChangeVMI {
				_, err := h.OnVmiChanged("", tt.args.vmi)
				if (err != nil) != tt.wantVMIErr {
					t.Errorf("OnVmiChanged() error = %v, wantErr %v", err, tt.wantVMIErr)
					return
				}
			}

			rq, err := clientset.Tracker().Get(corev1.SchemeGroupVersion.WithResource("resourcequotas"), tt.args.rq.Namespace, tt.args.rq.Name)
			assert.Nil(t, err, errMockTrackerGet)
			assert.Equal(t, tt.want, rq, "case %q", tt.name)

			// vmim change will cause vm is updated thus the UI can render proper migrate button
			obj, err := harvFakeClient.Tracker().Get(kubevirtv1.SchemeGroupVersion.WithResource("virtualmachines"), tt.args.vm.Namespace, tt.args.vm.Name)
			assert.Nil(t, err, errMockTrackerGet)
			vm, ok := obj.(*kubevirtv1.VirtualMachine)
			assert.Equal(t, ok, true, "case %q", tt.name)
			if tt.wantVMTsUpdated {
				assert.True(t, len(vm.Annotations) == 1, "case %q", tt.name)
				assert.True(t, vm.Annotations[util.AnnotationTimestamp] > tt.baseVMTs, tt.name)
			}
		})
	}
}
