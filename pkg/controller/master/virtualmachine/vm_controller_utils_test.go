package virtualmachine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	backendstorage "kubevirt.io/kubevirt/pkg/storage/backend-storage"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
)

// fakeDataVolumeClient implements ctlcdiv1.DataVolumeClient for testing.
type fakeDataVolumeClient struct {
	dvs map[string]*cdiv1.DataVolume // "namespace/name" -> DataVolume
}

func newFakeDataVolumeClient(dvs ...*cdiv1.DataVolume) *fakeDataVolumeClient {
	m := make(map[string]*cdiv1.DataVolume)
	for _, dv := range dvs {
		m[dv.Namespace+"/"+dv.Name] = dv
	}
	return &fakeDataVolumeClient{dvs: m}
}

func (c *fakeDataVolumeClient) Get(namespace, name string, _ metav1.GetOptions) (*cdiv1.DataVolume, error) {
	if dv, ok := c.dvs[namespace+"/"+name]; ok {
		return dv, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "cdi.kubevirt.io", Resource: "datavolumes"}, name)
}

func (c *fakeDataVolumeClient) Create(_ *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) Update(_ *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) UpdateStatus(_ *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) List(_ string, _ metav1.ListOptions) (*cdiv1.DataVolumeList, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (*cdiv1.DataVolume, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*cdiv1.DataVolume, *cdiv1.DataVolumeList], error) {
	panic("not implemented")
}

// fakePVCCache implements v1.PersistentVolumeClaimCache for testing.
type fakePVCCache struct {
	pvcs map[string]*corev1.PersistentVolumeClaim // "namespace/name" -> PVC
}

func newFakePVCCache(pvcs ...*corev1.PersistentVolumeClaim) *fakePVCCache {
	m := make(map[string]*corev1.PersistentVolumeClaim)
	for _, pvc := range pvcs {
		m[pvc.Namespace+"/"+pvc.Name] = pvc
	}
	return &fakePVCCache{pvcs: m}
}

func (c *fakePVCCache) Get(namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	if pvc, ok := c.pvcs[namespace+"/"+name]; ok {
		return pvc, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "persistentvolumeclaims"}, name)
}

func (c *fakePVCCache) List(_ string, _ labels.Selector) ([]*corev1.PersistentVolumeClaim, error) {
	panic("not implemented")
}

func (c *fakePVCCache) AddIndexer(_ string, _ generic.Indexer[*corev1.PersistentVolumeClaim]) {
	panic("not implemented")
}

func (c *fakePVCCache) GetByIndex(_, _ string) ([]*corev1.PersistentVolumeClaim, error) {
	panic("not implemented")
}

type fakeStorageClassClient struct {
	scs map[string]*storagev1.StorageClass
}

func newFakeStorageClassClient(scs ...*storagev1.StorageClass) *fakeStorageClassClient {
	m := make(map[string]*storagev1.StorageClass)
	for _, sc := range scs {
		if sc == nil {
			continue
		}
		m[sc.Name] = sc
	}
	return &fakeStorageClassClient{scs: m}
}

func (c *fakeStorageClassClient) Get(name string, _ metav1.GetOptions) (*storagev1.StorageClass, error) {
	if sc, ok := c.scs[name]; ok {
		return sc, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "storage.k8s.io", Resource: "storageclasses"}, name)
}

func (c *fakeStorageClassClient) Create(_ *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	panic("not implemented")
}

func (c *fakeStorageClassClient) Update(_ *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	panic("not implemented")
}

func (c *fakeStorageClassClient) UpdateStatus(_ *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	panic("not implemented")
}

func (c *fakeStorageClassClient) Delete(_ string, _ *metav1.DeleteOptions) error {
	panic("not implemented")
}

func (c *fakeStorageClassClient) List(opts metav1.ListOptions) (*storagev1.StorageClassList, error) {
	var items []storagev1.StorageClass
	selector, err := labels.Parse(opts.LabelSelector)
	if err != nil {
		return nil, err
	}
	for _, sc := range c.scs {
		if sc == nil {
			continue
		}
		if selector.Matches(labels.Set(sc.Labels)) {
			items = append(items, *sc)
		}
	}
	return &storagev1.StorageClassList{Items: items}, nil
}

func (c *fakeStorageClassClient) Watch(_ metav1.ListOptions) (watch.Interface, error) {
	panic("not implemented")
}

func (c *fakeStorageClassClient) Patch(_ string, _ types.PatchType, _ []byte, _ ...string) (*storagev1.StorageClass, error) {
	panic("not implemented")
}

func (c *fakeStorageClassClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*storagev1.StorageClass, *storagev1.StorageClassList], error) {
	panic("not implemented")
}

type fakeStorageClassCache struct {
	scs map[string]*storagev1.StorageClass
}

func newFakeStorageClassCache(scs ...*storagev1.StorageClass) *fakeStorageClassCache {
	m := make(map[string]*storagev1.StorageClass)
	for _, sc := range scs {
		if sc == nil {
			continue
		}
		m[sc.Name] = sc
	}
	return &fakeStorageClassCache{scs: m}
}

func (c *fakeStorageClassCache) Get(name string) (*storagev1.StorageClass, error) {
	if sc, ok := c.scs[name]; ok {
		return sc, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "storage.k8s.io", Resource: "storageclasses"}, name)
}

func (c *fakeStorageClassCache) List(selector labels.Selector) ([]*storagev1.StorageClass, error) {
	var result []*storagev1.StorageClass
	for _, sc := range c.scs {
		if sc == nil {
			continue
		}
		if selector.Matches(labels.Set(sc.Labels)) {
			result = append(result, sc)
		}
	}
	return result, nil
}

func (c *fakeStorageClassCache) AddIndexer(_ string, _ generic.Indexer[*storagev1.StorageClass]) {
	panic("not implemented")
}

func (c *fakeStorageClassCache) GetByIndex(_, _ string) ([]*storagev1.StorageClass, error) {
	panic("not implemented")
}

// mustMarshalEntries is a test helper that marshals VolumeClaimTemplateEntry slice to JSON string.
func mustMarshalEntries(entries []util.VolumeClaimTemplateEntry) string {
	data, err := json.Marshal(entries)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// newFakeVMClient creates a fakeVMClient from a fake clientset.
// Reuses the fakeVMClient type defined in vmi_network_controller_test.go.
func newFakeVMClient(clientset *fake.Clientset) fakeVMClient {
	return fakeVMClient(clientset.KubevirtV1().VirtualMachines)
}

type enqueueCall struct {
	namespace string
	name      string
	after     time.Duration
}

type fakeRequeueVMController struct {
	fakeVMClient
	enqueues []enqueueCall
}

func (c *fakeRequeueVMController) Informer() cache.SharedIndexInformer {
	return nil
}

func (c *fakeRequeueVMController) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{}
}

func (c *fakeRequeueVMController) AddGenericHandler(context.Context, string, generic.Handler) {}

func (c *fakeRequeueVMController) AddGenericRemoveHandler(context.Context, string, generic.Handler) {}

func (c *fakeRequeueVMController) Updater() generic.Updater {
	return nil
}

func (c *fakeRequeueVMController) OnChange(context.Context, string, generic.ObjectHandler[*kubevirtv1.VirtualMachine]) {
}

func (c *fakeRequeueVMController) OnRemove(context.Context, string, generic.ObjectHandler[*kubevirtv1.VirtualMachine]) {
}

func (c *fakeRequeueVMController) Enqueue(string, string) {}

func (c *fakeRequeueVMController) EnqueueAfter(namespace, name string, after time.Duration) {
	c.enqueues = append(c.enqueues, enqueueCall{
		namespace: namespace,
		name:      name,
		after:     after,
	})
}

func (c *fakeRequeueVMController) Cache() generic.CacheInterface[*kubevirtv1.VirtualMachine] {
	return nil
}

// fakePVCClient implements v1.PersistentVolumeClaimClient for testing.
type fakePVCClient struct {
	pvcs    map[string]*corev1.PersistentVolumeClaim // "namespace/name" -> PVC
	created []*corev1.PersistentVolumeClaim
	counter int
}

func newFakePVCClient(pvcs ...*corev1.PersistentVolumeClaim) *fakePVCClient {
	m := make(map[string]*corev1.PersistentVolumeClaim)
	for _, pvc := range pvcs {
		if pvc == nil {
			continue
		}
		m[pvc.Namespace+"/"+pvc.Name] = pvc
	}
	return &fakePVCClient{pvcs: m, created: []*corev1.PersistentVolumeClaim{}}
}

func (c *fakePVCClient) Get(namespace, name string, _ metav1.GetOptions) (*corev1.PersistentVolumeClaim, error) {
	if pvc, ok := c.pvcs[namespace+"/"+name]; ok {
		return pvc, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "persistentvolumeclaims"}, name)
}

func (c *fakePVCClient) Create(pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	if pvc.Name == "" && pvc.GenerateName != "" {
		c.counter++
		pvc = pvc.DeepCopy()
		pvc.Name = fmt.Sprintf("%s%d", pvc.GenerateName, c.counter)
	}
	key := pvc.Namespace + "/" + pvc.Name
	if _, ok := c.pvcs[key]; ok {
		return nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: "", Resource: "persistentvolumeclaims"}, pvc.Name)
	}
	c.pvcs[key] = pvc
	c.created = append(c.created, pvc)
	return pvc, nil
}

func (c *fakePVCClient) List(namespace string, opts metav1.ListOptions) (*corev1.PersistentVolumeClaimList, error) {
	var items []corev1.PersistentVolumeClaim
	selector, err := labels.Parse(opts.LabelSelector)
	if err != nil {
		return nil, err
	}
	for _, pvc := range c.pvcs {
		if pvc == nil {
			continue
		}
		if namespace != "" && pvc.Namespace != namespace {
			continue
		}
		if selector.Matches(labels.Set(pvc.Labels)) {
			items = append(items, *pvc)
		}
	}
	return &corev1.PersistentVolumeClaimList{Items: items}, nil
}

func (c *fakePVCClient) Update(_ *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	panic("not implemented")
}

func (c *fakePVCClient) UpdateStatus(_ *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	panic("not implemented")
}

func (c *fakePVCClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("not implemented")
}

func (c *fakePVCClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("not implemented")
}

func (c *fakePVCClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (*corev1.PersistentVolumeClaim, error) {
	panic("not implemented")
}

func (c *fakePVCClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*corev1.PersistentVolumeClaim, *corev1.PersistentVolumeClaimList], error) {
	panic("not implemented")
}

func (c *fakePVCClient) Cache() generic.CacheInterface[*corev1.PersistentVolumeClaim] {
	panic("not implemented")
}

// fakeJobClient implements ctlbatchv1.JobClient for testing.
type fakeJobClient struct {
	jobs    map[string]*batchv1.Job // "namespace/name" -> Job
	created []*batchv1.Job
	deleted []*batchv1.Job
	counter int
}

func newFakeJobClient(jobs ...*batchv1.Job) *fakeJobClient {
	m := make(map[string]*batchv1.Job)
	for _, job := range jobs {
		if job == nil {
			continue
		}
		m[job.Namespace+"/"+job.Name] = job
	}
	return &fakeJobClient{jobs: m, created: []*batchv1.Job{}, deleted: []*batchv1.Job{}}
}

func (c *fakeJobClient) Get(namespace, name string, _ metav1.GetOptions) (*batchv1.Job, error) {
	if job, ok := c.jobs[namespace+"/"+name]; ok {
		return job, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, name)
}

func (c *fakeJobClient) Create(job *batchv1.Job) (*batchv1.Job, error) {
	if job.Name == "" && job.GenerateName != "" {
		c.counter++
		job = job.DeepCopy()
		job.Name = fmt.Sprintf("%s%d", job.GenerateName, c.counter)
	}
	key := job.Namespace + "/" + job.Name
	if _, ok := c.jobs[key]; ok {
		return nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: "batch", Resource: "jobs"}, job.Name)
	}
	c.jobs[key] = job
	c.created = append(c.created, job)
	return job, nil
}

func (c *fakeJobClient) List(namespace string, opts metav1.ListOptions) (*batchv1.JobList, error) {
	var items []batchv1.Job
	selector, err := labels.Parse(opts.LabelSelector)
	if err != nil {
		return nil, err
	}
	for _, job := range c.jobs {
		if job == nil {
			continue
		}
		if namespace != "" && job.Namespace != namespace {
			continue
		}
		if selector.Matches(labels.Set(job.Labels)) {
			items = append(items, *job)
		}
	}
	return &batchv1.JobList{Items: items}, nil
}

func (c *fakeJobClient) Update(_ *batchv1.Job) (*batchv1.Job, error) {
	panic("not implemented")
}

func (c *fakeJobClient) UpdateStatus(_ *batchv1.Job) (*batchv1.Job, error) {
	panic("not implemented")
}

func (c *fakeJobClient) Delete(namespace, name string, _ *metav1.DeleteOptions) error {
	if job, ok := c.jobs[namespace+"/"+name]; ok {
		c.deleted = append(c.deleted, job)
	}
	delete(c.jobs, namespace+"/"+name)
	return nil
}

func (c *fakeJobClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("not implemented")
}

func (c *fakeJobClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (*batchv1.Job, error) {
	panic("not implemented")
}

func (c *fakeJobClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*batchv1.Job, *batchv1.JobList], error) {
	panic("not implemented")
}

func (c *fakeJobClient) Cache() generic.CacheInterface[*batchv1.Job] {
	panic("not implemented")
}

// fakeJobCache implements ctlbatchv1.JobCache for testing.
type fakeJobCache struct {
	jobs map[string]*batchv1.Job // "namespace/name" -> Job
}

func newFakeJobCache(jobs ...*batchv1.Job) *fakeJobCache {
	m := make(map[string]*batchv1.Job)
	for _, job := range jobs {
		if job == nil {
			continue
		}
		m[job.Namespace+"/"+job.Name] = job
	}
	return &fakeJobCache{jobs: m}
}

func (c *fakeJobCache) Get(namespace, name string) (*batchv1.Job, error) {
	if job, ok := c.jobs[namespace+"/"+name]; ok {
		return job, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, name)
}

func (c *fakeJobCache) List(_ string, _ labels.Selector) ([]*batchv1.Job, error) {
	panic("not implemented")
}

func (c *fakeJobCache) AddIndexer(_ string, _ generic.Indexer[*batchv1.Job]) {
	panic("not implemented")
}

func (c *fakeJobCache) GetByIndex(_, _ string) ([]*batchv1.Job, error) {
	panic("not implemented")
}

func ptrRunStrategy(s kubevirtv1.VirtualMachineRunStrategy) *kubevirtv1.VirtualMachineRunStrategy {
	return &s
}

func newTestVM(namespace, name string, annotations map[string]string) *kubevirtv1.VirtualMachine {
	return &kubevirtv1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kubevirtv1.VirtualMachineGroupVersionKind.GroupVersion().String(),
			Kind:       "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			UID:         "target-vm-uid",
			Annotations: annotations,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
			Template:    &kubevirtv1.VirtualMachineInstanceTemplateSpec{},
		},
	}
}

func newTestVMWithPersistentEFI(namespace, name string, annotations map[string]string) *kubevirtv1.VirtualMachine {
	vm := newTestVM(namespace, name, annotations)
	persistent := true
	vm.Spec.Template.Spec.Domain.Firmware = &kubevirtv1.Firmware{
		Bootloader: &kubevirtv1.Bootloader{
			EFI: &kubevirtv1.EFI{
				Persistent: &persistent,
			},
		},
	}
	return vm
}

func newTestVMWithPersistentTPM(namespace, name string, annotations map[string]string) *kubevirtv1.VirtualMachine {
	vm := newTestVM(namespace, name, annotations)
	persistent := true
	vm.Spec.Template.Spec.Domain.Devices.TPM = &kubevirtv1.TPMDevice{
		Persistent: &persistent,
	}
	return vm
}

func newTestSourcePVC(namespace, sourceVMName, storageClassName string, volumeMode corev1.PersistentVolumeMode) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backendstorage.PVCPrefix + "-" + sourceVMName,
			Labels: map[string]string{
				backendstorage.PVCPrefix: sourceVMName,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClassName,
			VolumeMode:       &volumeMode,
		},
	}
}

func newTestClonedPVC(namespace, targetVMName string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      backendStorageClonePVCGenerateName(targetVMName) + "existing",
			Labels: map[string]string{
				util.LabelKubeVirtPersistentState: targetVMName,
				util.LabelGeneratedBy:             util.ValueGeneratedByHarvester,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: kubevirtv1.VirtualMachineGroupVersionKind.GroupVersion().String(),
				Kind:       kubevirtv1.VirtualMachineGroupVersionKind.Kind,
				UID:        "target-vm-uid",
				Name:       targetVMName,
			}},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}
}

func newTestStorageClass(name string, defaultClass bool) *storagev1.StorageClass {
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if defaultClass {
		sc.Annotations = map[string]string{
			util.AnnotationIsDefaultStorageClassName: "true",
		}
	}
	return sc
}

func mustBuildBackendStorageJob(t *testing.T, ctrl *VMController, vm *kubevirtv1.VirtualMachine, sourceVMName string, clonedPVC *corev1.PersistentVolumeClaim, cloneAction string) *batchv1.Job {
	t.Helper()
	job, err := ctrl.buildBackendStorageJob(vm, sourceVMName, clonedPVC, cloneAction)
	require.NoError(t, err)
	return job
}

func newTestBackendStorageJob(job *batchv1.Job, mutate func(*batchv1.Job)) *batchv1.Job {
	if job == nil {
		return nil
	}

	jobCopy := job.DeepCopy()
	if jobCopy.Name == "" && jobCopy.GenerateName != "" {
		jobCopy.Name = jobCopy.GenerateName + "existing"
	}
	if mutate != nil {
		mutate(jobCopy)
	}
	return jobCopy
}

func newTestCloneInProgressAnnotations(sourceVMName string) map[string]string {
	return newTestCloneInProgressAnnotationsWithAction(sourceVMName, util.CloneActionRenameEFI)
}

func newTestCloneInProgressAnnotationsWithAction(sourceVMName, cloneAction string) map[string]string {
	return map[string]string{
		util.AnnotationBSCloneStatus:      util.CloneInProgress,
		util.AnnotationBSCloneSourceVM:    sourceVMName,
		util.AnnotationBSCloneRunStrategy: string(kubevirtv1.RunStrategyRerunOnFailure),
		util.AnnotationBSCloneActions:     cloneAction,
		util.AnnotationBSCloneStartTime:   time.Now().UTC().Format(time.RFC3339),
	}
}

func assertCreatedTestPVC(t *testing.T, expected, actual *corev1.PersistentVolumeClaim) {
	t.Helper()
	require.NotNil(t, expected)
	require.NotNil(t, actual)
	assert.Equal(t, expected.GenerateName, actual.GenerateName)
	if expected.Name != "" {
		assert.Equal(t, expected.Name, actual.Name)
	} else {
		assert.True(t, strings.HasPrefix(actual.Name, expected.GenerateName))
	}
	assert.Equal(t, expected.Namespace, actual.Namespace)
	assert.Equal(t, expected.Labels, actual.Labels)
	assert.Equal(t, expected.OwnerReferences, actual.OwnerReferences)
	assert.Equal(t, expected.Spec, actual.Spec)
}

func assertCreatedTestJob(t *testing.T, expected, actual *batchv1.Job) {
	t.Helper()
	require.NotNil(t, expected)
	require.NotNil(t, actual)
	assert.Equal(t, expected.GenerateName, actual.GenerateName)
	if expected.Name != "" {
		assert.Equal(t, expected.Name, actual.Name)
	} else {
		assert.True(t, strings.HasPrefix(actual.Name, expected.GenerateName))
	}
	assert.Equal(t, expected.Namespace, actual.Namespace)
	assert.Equal(t, expected.Labels, actual.Labels)
	assert.Equal(t, expected.OwnerReferences, actual.OwnerReferences)
	assert.Equal(t, expected.Spec, actual.Spec)
}
