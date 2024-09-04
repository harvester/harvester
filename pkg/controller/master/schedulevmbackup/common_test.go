package schedulevmbackup

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

var (
	timestamp1   = "20240717.0827"
	timestamp2   = "20240717.0829"
	timestamp3   = "20240717.0833"
	timestampErr = "20240717.0830"
	retain       = 3
	maxFailure   = 2

	svmbackup = &harvesterv1.ScheduleVMBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "svmbackup",
			UID:       "841f1346-ce15-4495-a5f6-fae76d11f92e",
		},
		Spec: harvesterv1.ScheduleVMBackupSpec{
			Retain:     retain,
			MaxFailure: maxFailure,
		},
	}

	cronJob = &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cronJobNamespace,
			Name:      fmt.Sprintf("%s-%s", svmbackupPrefix, svmbackup.UID),
		},
	}

	vmbackup1 = &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svmbackup.Namespace,
			Name:      fmt.Sprintf("%s-%s-%s", svmbackupPrefix, svmbackup.UID, timestamp1),
			Annotations: map[string]string{
				util.AnnotationSVMBackupID: ref.Construct(svmbackup.Namespace, svmbackup.Name),
			},
			Labels: map[string]string{
				util.LabelSVMBackupUID:       string(svmbackup.UID),
				util.LabelSVMBackupTimestamp: timestamp1,
			},
		},
		Status: &harvesterv1.VirtualMachineBackupStatus{
			ReadyToUse: pointer.Bool(true),
			Error:      nil,
		},
	}

	vmbackup1Info = &harvesterv1.VMBackupInfo{
		Name:       vmbackup1.Name,
		ReadyToUse: pointer.Bool(true),
		Error:      nil,
	}

	vmbackup2 = &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svmbackup.Namespace,
			Name:      fmt.Sprintf("%s-%s-%s", svmbackupPrefix, svmbackup.UID, timestamp2),
			Annotations: map[string]string{
				util.AnnotationSVMBackupID: ref.Construct(svmbackup.Namespace, svmbackup.Name),
			},
			Labels: map[string]string{
				util.LabelSVMBackupUID:       string(svmbackup.UID),
				util.LabelSVMBackupTimestamp: timestamp2,
			},
		},
		Status: &harvesterv1.VirtualMachineBackupStatus{
			ReadyToUse: pointer.Bool(true),
			Error:      nil,
		},
	}

	vmbackup3 = &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svmbackup.Namespace,
			Name:      fmt.Sprintf("%s-%s-%s", svmbackupPrefix, svmbackup.UID, timestamp3),
			Annotations: map[string]string{
				util.AnnotationSVMBackupID: ref.Construct(svmbackup.Namespace, svmbackup.Name),
			},
			Labels: map[string]string{
				util.LabelSVMBackupUID:       string(svmbackup.UID),
				util.LabelSVMBackupTimestamp: timestamp3,
			},
		},
		Status: &harvesterv1.VirtualMachineBackupStatus{
			ReadyToUse: pointer.Bool(true),
			Error:      nil,
		},
	}

	vmbackupErr = &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svmbackup.Namespace,
			Name:      fmt.Sprintf("%s-%s-%s", svmbackupPrefix, svmbackup.UID, timestampErr),
			Annotations: map[string]string{
				util.AnnotationSVMBackupID: ref.Construct(svmbackup.Namespace, svmbackup.Name),
			},
			Labels: map[string]string{
				util.LabelSVMBackupUID:       string(svmbackup.UID),
				util.LabelSVMBackupTimestamp: timestampErr,
			},
		},
		Status: &harvesterv1.VirtualMachineBackupStatus{
			ReadyToUse: pointer.Bool(false),
			Error:      &harvesterv1.Error{},
		},
	}
)

func Test_CronJobName(t *testing.T) {
	cronJobName := cronJobName(svmbackup)
	assert := require.New(t)
	assert.Equal(cronJob.Name, cronJobName, "expected to correct cronjob name")
}

func Test_GetCronJob(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	assert := require.New(t)

	h := &svmbackupHandler{
		cronJobCache: fakeclients.CronJobCache(clientset.BatchV1().CronJobs),
	}

	err := clientset.Tracker().Add(cronJob)
	assert.Nil(err, "cronJob should add into fake controller")

	getCronJob, err := getCronJob(h, svmbackup)
	assert.Nil(err, "cronjob should get from fake controller")
	assert.Equal(cronJob, getCronJob, "cronJob not equals")
}

func Test_VMBackupName(t *testing.T) {
	vmbackupName := vmBackupName(svmbackup, timestamp1)
	assert := require.New(t)
	assert.Equal(vmbackup1.Name, vmbackupName, "expected to correct vmbackup name")
}

func Test_GetVMBackup(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	assert := require.New(t)

	h := &svmbackupHandler{
		vmBackupCache: fakeclients.VMBackupCache(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
	}

	err := clientset.Tracker().Add(vmbackup1)
	assert.Nil(err, "vmbackup should add into fake controller")

	getVMBackup, err := getVMBackup(h, svmbackup, timestamp1)
	assert.Nil(err, "vmbackup should get from fake controller")
	assert.Equal(vmbackup1, getVMBackup, "vmbackup not equals")
}

func Test_CurrentVMBackups(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	assert := require.New(t)

	h := &svmbackupHandler{
		vmBackupCache: fakeclients.VMBackupCache(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
	}

	err := clientset.Tracker().Add(vmbackup1)
	assert.Nil(err, "vmbackup1 should add into fake controller")

	err = clientset.Tracker().Add(vmbackup2)
	assert.Nil(err, "vmbackup2 should add into fake controller")

	vmBackups, errVMBackups, lastVMBackup, failure, err := currentVMBackups(h, svmbackup)
	assert.Nil(err, "vmbackups should get from fake controller")
	assert.Len(vmBackups, 2, "expected to find 2 vmbackups")
	assert.Len(errVMBackups, 0, "expected to find 0 err vmbackups")
	assert.Equal(vmbackup2, lastVMBackup, "last vmbackup not equals vmbackup2")
	assert.Equal(failure, 0, "failure should be zero")
}

func Test_CreateVMBackup(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	assert := require.New(t)

	h := &svmbackupHandler{
		vmBackupClient: fakeclients.VMBackupClient(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
	}

	getVMBackup, err := createVMBackup(h, svmbackup, timestamp1)
	assert.Nil(err, "vmbackup should create from fake controller")
	assert.Equal(vmbackup1.ObjectMeta, getVMBackup.ObjectMeta, "vmbackup not equals")
}

func Test_ConvertVMBackupToInfo(t *testing.T) {
	assert := require.New(t)

	getVMBackupInfo := convertVMBackupToInfo(vmbackup1)
	assert.Equal(vmbackup1Info, &getVMBackupInfo, "vmbackup info not equals")
}

func Test_ReconcileVMBackupList(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	assert := require.New(t)

	h := &svmbackupHandler{
		vmBackupCache:   fakeclients.VMBackupCache(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
		svmbackupClient: fakeclients.SVMBackupClient(clientset.HarvesterhciV1beta1().ScheduleVMBackups),
		svmbackupCache:  fakeclients.SVMBackupCache(clientset.HarvesterhciV1beta1().ScheduleVMBackups),
	}

	err := clientset.Tracker().Add(vmbackup1)
	assert.Nil(err, "vmbackup1 should add into fake controller")

	err = clientset.Tracker().Add(vmbackup2)
	assert.Nil(err, "vmbackup2 should add into fake controller")

	err = clientset.Tracker().Add(vmbackupErr)
	assert.Nil(err, "vmbackupErr should add into fake controller")

	err = clientset.Tracker().Add(svmbackup)
	assert.Nil(err, "svmbackup should add into fake controller")

	err = reconcileVMBackupList(h, svmbackup)
	assert.Nil(err, "reconcile vmbackup list should success")

	getSVMBackup, err := h.svmbackupCache.Get(svmbackup.Namespace, svmbackup.Name)
	assert.Nil(err, "svmbackup should get from fake controller")
	assert.Len(getSVMBackup.Status.VMBackupInfo, 3, "expected to find 3 vmbackup info")
}

func Test_HandleReachMaxFailure(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	assert := require.New(t)

	h := &svmbackupHandler{
		svmbackupClient: fakeclients.SVMBackupClient(clientset.HarvesterhciV1beta1().ScheduleVMBackups),
		svmbackupCache:  fakeclients.SVMBackupCache(clientset.HarvesterhciV1beta1().ScheduleVMBackups),
		cronJobsClient:  fakeclients.CronJobClient(clientset.BatchV1().CronJobs),
		cronJobCache:    fakeclients.CronJobCache(clientset.BatchV1().CronJobs),
	}

	svmbackup.Status.Failure = maxFailure
	err := clientset.Tracker().Add(svmbackup)
	assert.Nil(err, "svmbackup should add into fake controller")

	err = clientset.Tracker().Add(cronJob)
	assert.Nil(err, "cronjob should add into fake controller")

	err = handleReachMaxFailure(h, svmbackup, "")
	assert.Nil(err, "handle reach max failure should should success")

	getSVMBackup, err := h.svmbackupCache.Get(svmbackup.Namespace, svmbackup.Name)
	assert.Nil(err, "svmbackup should get from fake controller")
	assert.True(getSVMBackup.Status.Suspended, "svmbackup should suspend")

	getCronJob, err := h.cronJobCache.Get(cronJob.Namespace, cronJob.Name)
	assert.Nil(err, "cronjob should get from fake controller")
	assert.True(*getCronJob.Spec.Suspend, "cronjob should suspend")
}

func Test_NewVMBackups(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	assert := require.New(t)

	h := &svmbackupHandler{
		vmBackupClient: fakeclients.VMBackupClient(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
		vmBackupCache:  fakeclients.VMBackupCache(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
	}

	err := clientset.Tracker().Add(vmbackup1)
	assert.Nil(err, "vmbackup1 should add into fake controller")

	err = clientset.Tracker().Add(vmbackup2)
	assert.Nil(err, "vmbackup2 should add into fake controller")

	getVMbackup, err := newVMBackups(h, svmbackup, timestamp3)
	assert.Nil(err, "new vmbackup should success")
	assert.Equal(vmbackup3.Spec, getVMbackup.Spec, "new vmbackup should equals vmbackup3")
}
