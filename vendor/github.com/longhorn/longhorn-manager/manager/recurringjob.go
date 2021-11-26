package manager

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/longhorn/longhorn-manager/util"
)

func (m *VolumeManager) GetRecurringJob(name string) (*longhorn.RecurringJob, error) {
	return m.ds.GetRecurringJob(name)
}

func (m *VolumeManager) ListRecurringJobsSorted() ([]*longhorn.RecurringJob, error) {
	jobMap, err := m.ds.ListRecurringJobs()
	if err != nil {
		return []*longhorn.RecurringJob{}, err
	}

	jobs := make([]*longhorn.RecurringJob, len(jobMap))
	jobNames, err := sortKeys(jobMap)
	if err != nil {
		return []*longhorn.RecurringJob{}, err
	}
	for i, name := range jobNames {
		jobs[i] = jobMap[name]
	}
	return jobs, nil
}

func (m *VolumeManager) CreateRecurringJob(spec *longhorn.RecurringJobSpec) (*longhorn.RecurringJob, error) {
	name := util.AutoCorrectName(spec.Name, datastore.NameMaximumLength)
	if !util.ValidateName(name) {
		return nil, fmt.Errorf("invalid name %v", name)
	}

	jobs := []longhorn.RecurringJobSpec{
		{
			Name:        spec.Name,
			Groups:      spec.Groups,
			Task:        spec.Task,
			Cron:        spec.Cron,
			Retain:      spec.Retain,
			Concurrency: spec.Concurrency,
			Labels:      spec.Labels,
		},
	}
	if err := datastore.ValidateRecurringJobs(jobs); err != nil {
		return nil, err
	}

	job := &longhorn.RecurringJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: *spec,
	}
	job, err := m.ds.CreateRecurringJob(job)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Created recurring job %v", name)
	return job, nil
}

func (m *VolumeManager) UpdateRecurringJob(spec longhorn.RecurringJobSpec) (*longhorn.RecurringJob, error) {
	var err error
	defer func() {
		err = errors.Wrapf(err, "unable to update %v recurring job", spec.Name)
	}()

	jobs := []longhorn.RecurringJobSpec{
		{
			Name:        spec.Name,
			Groups:      spec.Groups,
			Task:        spec.Task,
			Cron:        spec.Cron,
			Retain:      spec.Retain,
			Concurrency: spec.Concurrency,
			Labels:      spec.Labels,
		},
	}
	if err = datastore.ValidateRecurringJobs(jobs); err != nil {
		return nil, err
	}

	recurringJob, err := m.ds.GetRecurringJob(spec.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get recurring job %v", spec.Name)
	}
	sort.Strings(recurringJob.Spec.Groups)
	sort.Strings(spec.Groups)
	if recurringJob.Spec.Cron == spec.Cron &&
		reflect.DeepEqual(recurringJob.Spec.Groups, spec.Groups) &&
		recurringJob.Spec.Retain == spec.Retain &&
		recurringJob.Spec.Concurrency == spec.Concurrency &&
		reflect.DeepEqual(recurringJob.Spec.Labels, spec.Labels) {
		return recurringJob, nil
	}
	recurringJob.Spec.Cron = spec.Cron
	recurringJob.Spec.Groups = spec.Groups
	recurringJob.Spec.Retain = spec.Retain
	recurringJob.Spec.Concurrency = spec.Concurrency
	recurringJob.Spec.Labels = spec.Labels
	return m.ds.UpdateRecurringJob(recurringJob)
}

func (m *VolumeManager) DeleteRecurringJob(name string) error {
	if err := m.ds.DeleteRecurringJob(name); err != nil {
		return err
	}
	logrus.Infof("Deleted recurring job %v", name)
	return nil
}
