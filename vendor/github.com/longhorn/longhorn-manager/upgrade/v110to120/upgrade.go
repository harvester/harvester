package v110to120

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/types"
	upgradeutil "github.com/longhorn/longhorn-manager/upgrade/util"
	"github.com/longhorn/longhorn-manager/util"
)

const (
	upgradeLogPrefix = "upgrade from v1.1.0 to v1.2.0: "

	longhornFinalizerKey = "longhorn.io"
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) error {
	if err := upgradeRecurringJobs(namespace, lhClient, kubeClient); err != nil {
		return err
	}
	if err := upgradeInstanceManagers(namespace, lhClient, kubeClient); err != nil {
		return err
	}
	return nil
}

func upgradeInstanceManagers(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade instance managers failed")
	}()

	// The pod update should happen before IM CR update. Hence we should not abstract this part as a function call in `doPodsUpgrade`.
	blockOwnerDeletion := true
	imPodList, err := upgradeutil.ListIMPods(namespace, kubeClient)
	if err != nil {
		return err
	}
	for _, imPod := range imPodList {
		if imPod.OwnerReferences == nil || len(imPod.OwnerReferences) == 0 {
			im, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).Get(context.TODO(), imPod.Name, metav1.GetOptions{})
			if err != nil {
				logrus.Errorf("cannot find the instance manager CR for the instance manager pod %v that has no owner reference during v1.2.0 upgrade: %v", imPod.Name, err)
				continue
			}
			imPod.OwnerReferences = datastore.GetOwnerReferencesForInstanceManager(im)
			if _, err = kubeClient.CoreV1().Pods(namespace).Update(context.TODO(), &imPod, metav1.UpdateOptions{}); err != nil {
				return err
			}
			continue
		}
		if imPod.OwnerReferences[0].BlockOwnerDeletion == nil || !*imPod.OwnerReferences[0].BlockOwnerDeletion {
			imPod.OwnerReferences[0].BlockOwnerDeletion = &blockOwnerDeletion
			if _, err = kubeClient.CoreV1().Pods(namespace).Update(context.TODO(), &imPod, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	imList, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, im := range imList.Items {
		if !util.FinalizerExists(longhornFinalizerKey, &im) {
			// finalizer already removed
			// skip updating this instance manager
			continue
		}
		if err := util.RemoveFinalizer(longhornFinalizerKey, &im); err != nil {
			return err
		}
		if _, err := lhClient.LonghornV1beta2().InstanceManagers(namespace).Update(context.TODO(), &im, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}

type recurringJobUpgrade struct {
	log *logrus.Entry

	namespace string

	kubeClient *clientset.Clientset
	lhClient   *lhclientset.Clientset

	recurringJobMapSpec map[string]*longhorn.RecurringJobSpec
	volumeMapLabels     map[string]map[string]string

	storageClass          *storagev1.StorageClass
	storageClassConfigMap *corev1.ConfigMap

	volumes *longhorn.VolumeList
}

type recurringJobSelector struct {
	Name    string `json:"name"`
	IsGroup bool   `json:"isGroup"`
}

func newRecurringJobUpgrade(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) *recurringJobUpgrade {
	return &recurringJobUpgrade{
		log:                 logrus.WithField("namespace", namespace),
		namespace:           namespace,
		kubeClient:          kubeClient,
		lhClient:            lhClient,
		recurringJobMapSpec: map[string]*longhorn.RecurringJobSpec{},
		volumeMapLabels:     map[string]map[string]string{},
	}
}

// upgradeRecurringJobs creates CRs from existing recurringJobs settings in
// storageClass and volumes spec.
// Here will also translates the storageClass recurringJobs to
// recurringJobSelector and volume.spec recurringJobs to volume labels.
func upgradeRecurringJobs(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade recurring jobs failed")
	}()

	run := newRecurringJobUpgrade(namespace, lhClient, kubeClient)

	err = run.translateStorageClassRecurringJobs()
	if err != nil {
		return err
	}

	err = run.translateVolumeRecurringJobs()
	if err != nil {
		return err
	}

	err = run.cleanupAppliedVolumeCronJobs()
	if err != nil {
		return err
	}
	return nil
}

func (run *recurringJobUpgrade) translateStorageClassRecurringJobs() (err error) {
	revertLog := run.log
	defer func() {
		if err == nil {
			run.log.Info(upgradeLogPrefix + "Finished storageClass recurring job translation")
		}
		err = errors.Wrapf(err, upgradeLogPrefix+"translate storage class recurring jobs failed")

		run.log = revertLog
		run.recurringJobMapSpec = map[string]*longhorn.RecurringJobSpec{}
	}()
	run.log = run.log.WithField("translate", "storage-class-recurring-jobs")
	run.log.Info(upgradeLogPrefix + "Starting storageClass recurring job translation")

	run.storageClassConfigMap, err = run.kubeClient.CoreV1().ConfigMaps(run.namespace).Get(context.TODO(), types.DefaultStorageClassConfigMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get %v ConfigMap: %v", types.DefaultStorageClassConfigMapName, err)
	}
	storageClassYAML, found := run.storageClassConfigMap.Data["storageclass.yaml"]
	if !found {
		return fmt.Errorf("failed to find storageclass.yaml inside the %v ConfigMap", types.DefaultStorageClassConfigMapName)
	}
	err = run.initStorageClassObjFromYAML(storageClassYAML)
	if err != nil {
		return err
	}
	scRecurringJobsJSON, ok := run.storageClass.Parameters["recurringJobs"]
	if !ok {
		return nil
	}
	scRecurringJobs := []longhorn.RecurringJobSpec{}
	err = json.Unmarshal([]byte(scRecurringJobsJSON), &scRecurringJobs)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal: %v", scRecurringJobs)
	}

	recurringJobIDs := []string{}
	for _, recurringJob := range scRecurringJobs {
		id, err := createRecurringJobID(recurringJob)
		if err != nil {
			return errors.Wrapf(err, "failed to create ID for recurring job %v", recurringJob)
		}
		run.recurringJobMapSpec[id] = &longhorn.RecurringJobSpec{
			Name:        id,
			Task:        recurringJob.Task,
			Cron:        recurringJob.Cron,
			Retain:      recurringJob.Retain,
			Concurrency: types.DefaultRecurringJobConcurrency,
			Labels:      recurringJob.Labels,
		}
		if !util.Contains(recurringJobIDs, id) {
			recurringJobIDs = append(recurringJobIDs, id)
		}
	}
	if err := run.createRecurringJobCRs(); err != nil {
		return err
	}
	if err := run.convertToSelectors(recurringJobIDs); err != nil {
		return err
	}

	return nil
}

func (run *recurringJobUpgrade) convertToSelectors(recurringJobIDs []string) (err error) {
	revertLog := run.log
	defer func() {
		run.log = revertLog
		err = errors.Wrapf(err, "convert storageClass recurringJobs to recurringJobSelector failed")
	}()
	run.log = run.log.WithField("action", "convert-recurring-jobs-to-selectors")

	if len(recurringJobIDs) == 0 {
		run.log.Debug(upgradeLogPrefix + "Found 0 recurring job to convert to recurring job selector")
		return
	}

	selectors := []recurringJobSelector{}
	selectorIDs := []string{}
	if selectorParameter, ok := run.storageClass.Parameters["recurringJobSelector"]; ok {
		err = json.Unmarshal([]byte(selectorParameter), &selectors)
		if err != nil {
			return errors.Wrapf(err, "failed to unmarshal: %v", selectorParameter)
		}
		for _, selector := range selectors {
			selectorIDs = append(selectorIDs, selector.Name)
		}
	}
	for _, id := range recurringJobIDs {
		if util.Contains(selectorIDs, id) {
			continue
		}
		selectors = append(selectors, recurringJobSelector{
			Name:    id,
			IsGroup: false,
		})
	}
	selectorJSON, err := json.Marshal(selectors)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSON %v", selectors)
	}
	run.log.Infof(upgradeLogPrefix+"Adding %v to recurringJobSelector", recurringJobIDs)
	run.storageClass.Parameters["recurringJobSelector"] = string(selectorJSON)

	run.log.Info(upgradeLogPrefix + "Removing recurringJobs")
	delete(run.storageClass.Parameters, "recurringJobs")

	logrus.Infof(upgradeLogPrefix+"Updating %v configmap", run.storageClassConfigMap.Name)
	newStorageClassYAML, err := yaml.Marshal(run.storageClass)
	run.storageClassConfigMap.Data["storageclass.yaml"] = string(newStorageClassYAML)
	if _, err := run.kubeClient.CoreV1().ConfigMaps(run.namespace).Update(context.TODO(), run.storageClassConfigMap, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, "failed to update %v configmap", run.storageClassConfigMap.Name)
	}
	return nil
}

func (run *recurringJobUpgrade) translateVolumeRecurringJobs() (err error) {
	revertLog := run.log
	defer func() {
		if err == nil {
			run.log.Info(upgradeLogPrefix + "Finished volume recurring job translation")
		}
		err = errors.Wrapf(err, upgradeLogPrefix+"translate volume recurringJobs failed")

		run.log = revertLog
		run.recurringJobMapSpec = map[string]*longhorn.RecurringJobSpec{}
	}()
	run.log = run.log.WithField("translate", "volume-recurring-jobs")
	run.log.Info(upgradeLogPrefix + "Starting volume recurring job translation")

	run.volumes, err = run.lhClient.LonghornV1beta2().Volumes(run.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			run.log.Debug(upgradeLogPrefix + "Found 0 volume")
			return nil
		}
		return errors.Wrap(err, "failed to list volumes")
	}
	for _, volume := range run.volumes.Items {
		addVolumeLabels := map[string]string{}
		for _, recurringJob := range volume.Spec.RecurringJobs {
			recurringJobSpec := longhorn.RecurringJobSpec{
				Name:        recurringJob.Name,
				Task:        recurringJob.Task,
				Cron:        recurringJob.Cron,
				Retain:      recurringJob.Retain,
				Concurrency: types.DefaultRecurringJobConcurrency,
				Labels:      recurringJob.Labels,
			}
			id, err := createRecurringJobID(recurringJobSpec)
			if err != nil {
				return errors.Wrapf(err, "failed to create ID for recurring job %v", recurringJob)
			}
			if _, exist := run.recurringJobMapSpec[id]; !exist {
				recurringJobSpec.Name = id
				run.recurringJobMapSpec[id] = &recurringJobSpec
			}
			key := types.GetRecurringJobLabelKey(types.LonghornLabelRecurringJob, id)
			addVolumeLabels[key] = types.LonghornLabelValueEnabled
		}
		if len(addVolumeLabels) != 0 {
			run.volumeMapLabels[volume.Name] = addVolumeLabels
		}
	}
	if err := run.createRecurringJobCRs(); err != nil {
		return err
	}
	if err := run.convertToVolumeLabels(); err != nil {
		return err
	}

	return nil
}

func (run *recurringJobUpgrade) convertToVolumeLabels() (err error) {
	revertLog := run.log
	defer func() {
		run.log = revertLog
		err = errors.Wrapf(err, upgradeLogPrefix+"convert volume recurring jobs to labels failed")
	}()
	run.log = run.log.WithField("action", "convert-volume-recurring-jobs-to-labels")

	volumeClient := run.lhClient.LonghornV1beta2().Volumes(run.namespace)
	for volumeName, labels := range run.volumeMapLabels {
		if len(labels) == 0 {
			continue
		}
		volume, err := volumeClient.Get(context.TODO(), volumeName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				run.log.Debug(upgradeLogPrefix + "Cannot find volume, could be removed")
				continue
			}
			return errors.Wrapf(err, "failed to get volume %v", volumeName)
		}
		volumeLabels := volume.Labels
		if volumeLabels == nil {
			volumeLabels = map[string]string{}
		}
		for key, value := range labels {
			volumeLabels[key] = value
		}
		volume.Labels = volumeLabels
		volume.Spec.RecurringJobs = nil
		run.log.Infof(upgradeLogPrefix+"Updating %v volume labels to %v", volume.Name, volume.Labels)
		_, err = volumeClient.Update(context.TODO(), volume, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update %v volume", volume.Name)
		}
	}
	return nil
}

func (run *recurringJobUpgrade) cleanupAppliedVolumeCronJobs() (err error) {
	revertLog := run.log
	defer func() {
		if err == nil {
			run.log.Info(upgradeLogPrefix + "Finished volume cron job cleanup")
		}
		err = errors.Wrapf(err, upgradeLogPrefix+"cleanup applied volume cron jobs failed")
		run.log = revertLog
	}()
	run.log = run.log.WithField("action", "cleanup-applied-volume-cron-jobs")
	run.log.Info(upgradeLogPrefix + "Starting volume cron job cleanup")

	propagation := metav1.DeletePropagationForeground
	cronJobClient := run.kubeClient.BatchV1beta1().CronJobs(run.namespace)
	for _, v := range run.volumes.Items {
		run.log.Debugf(upgradeLogPrefix+"Listing all cron jobs for volume %v", v.Name)
		appliedCronJobROs, err := listVolumeCronJobROs(v.Name, run.namespace, run.kubeClient)
		if err != nil {
			return errors.Wrapf(err, "failed to list all cron jobs for volume %v", v.Name)
		}
		for name := range appliedCronJobROs {
			run.log.Infof(upgradeLogPrefix+"Deleting %v cronjob job for %v volume", name, v.Name)
			err := cronJobClient.Delete(context.TODO(), name, metav1.DeleteOptions{PropagationPolicy: &propagation})
			if err != nil {
				return errors.Wrapf(err, "failed to delete %v cron job for volume %v", name, v.Name)
			}
		}
		run.log.Infof(upgradeLogPrefix+"Deleted %v cronjob job for %v volume", len(appliedCronJobROs), v.Name)
	}

	return nil
}

func (run *recurringJobUpgrade) createRecurringJobCRs() (err error) {
	revertLog := run.log
	defer func() {
		run.log = revertLog
		err = errors.Wrapf(err, "create recurringJob CRs failed")
	}()
	run.log = run.log.WithField("action", "create-recurringJob-CRs")

	if len(run.recurringJobMapSpec) == 0 {
		run.log.Debug(upgradeLogPrefix + "Found 0 recurring job")
		return nil
	}

	recurringJobClient := run.lhClient.LonghornV1beta2().RecurringJobs(run.namespace)
	for recurringJobName, spec := range run.recurringJobMapSpec {
		run.log.Infof(upgradeLogPrefix+"Creating %v recurring job CR", recurringJobName)
		newRecurringJob := &longhorn.RecurringJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: recurringJobName,
			},
			Spec: *spec,
		}
		run.log.Debugf(upgradeLogPrefix+"Checking if %v recurring job CR already exists", recurringJobName)
		obj, err := recurringJobClient.Get(context.TODO(), newRecurringJob.Name, metav1.GetOptions{})
		if err == nil {
			run.log.Debugf(upgradeLogPrefix+"Recurring job CR already exists %v", obj)
			continue
		}
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get recurring job %v", recurringJobName)
		}
		_, err = recurringJobClient.Create(context.TODO(), newRecurringJob, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to create recurring job CR with %v", spec)
		}
	}
	return nil
}

func (run *recurringJobUpgrade) initStorageClassObjFromYAML(storageclassYAML string) error {
	decode := scheme.Codecs.UniversalDeserializer().Decode
	obj, _, err := decode([]byte(storageclassYAML), nil, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to decoding YAML string")
	}
	sc, ok := obj.(*storagev1.StorageClass)
	if !ok {
		return fmt.Errorf("invalid storageclass YAML string: %v", storageclassYAML)
	}
	run.storageClass = sc
	return nil
}

func createRecurringJobID(recurringJob longhorn.RecurringJobSpec) (key string, err error) {
	labelJSON, err := json.Marshal(recurringJob.Labels)
	if err != nil {
		return key, errors.Wrapf(err, "failed to marshal JSON %v", recurringJob.Labels)
	}
	return fmt.Sprintf("%v-%v-%v-%v",
		recurringJob.Task,
		recurringJob.Retain,
		util.GetStringHash(recurringJob.Cron),
		util.GetStringHash(string(labelJSON)),
	), nil
}

func listVolumeCronJobROs(volumeName, namespace string, kubeClient *clientset.Clientset) (map[string]*batchv1beta1.CronJob, error) {
	itemMap := map[string]*batchv1beta1.CronJob{}
	list, err := kubeClient.BatchV1beta1().CronJobs(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: types.LonghornLabelVolume + "=" + volumeName,
	})
	if err != nil {
		return nil, err
	}
	for _, cj := range list.Items {
		itemMap[cj.Name] = &cj
	}
	return itemMap, nil
}
