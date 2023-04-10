package manager

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

type SupportBundle struct {
	Name  string
	State longhorn.SupportBundleState

	Filename           string
	ProgressPercentage int
	Size               int64

	Error string
}

// CreateSupportBundle creates a SupportBundle custom resource that triggers
// creation of support bundle manager deployment. The support bundle manager then
// creates a bundle zip file that is available in https://<cluster-ip>:8080/bundle
func (m *VolumeManager) CreateSupportBundle(issueURL string, description string) (*SupportBundle, error) {
	now := strings.ToLower(strings.Replace(util.Now(), ":", "-", -1))
	newSupportBundle := &longhorn.SupportBundle{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(types.SupportBundleNameFmt, now),
		},
		Spec: longhorn.SupportBundleSpec{
			Description: description,
			IssueURL:    issueURL,
		},
	}

	supportBundle, err := m.ds.CreateSupportBundle(newSupportBundle)
	if err != nil {
		return nil, err
	}

	ret := &SupportBundle{
		Name:  supportBundle.Name,
		State: supportBundle.Status.State,
	}
	return ret, nil
}

func (m *VolumeManager) DeleteSupportBundle(name string) error {
	return m.ds.DeleteSupportBundle(name)
}

func (m *VolumeManager) GetSupportBundle(name string) (*SupportBundle, string, error) {
	supportBundle, err := m.ds.GetSupportBundleRO(name)
	if err != nil {
		return nil, "", err
	}

	if supportBundle.Name != name {
		return nil, "", errors.Errorf("cannot find SupportBundle %s", name)
	}

	supportBundleError := types.GetCondition(supportBundle.Status.Conditions, longhorn.SupportBundleConditionTypeError)
	ret := &SupportBundle{
		Name:               supportBundle.Name,
		State:              supportBundle.Status.State,
		Filename:           supportBundle.Status.Filename,
		Size:               supportBundle.Status.Filesize,
		ProgressPercentage: supportBundle.Status.Progress,
		Error:              fmt.Sprintf("%v: %v", supportBundleError.Reason, supportBundleError.Message),
	}

	return ret, supportBundle.Status.IP, nil
}

func (m *VolumeManager) ListSupportBundlesSorted() ([]*longhorn.SupportBundle, error) {
	supportBundles, err := m.ds.ListSupportBundles()
	if err != nil {
		return []*longhorn.SupportBundle{}, err
	}

	supportBundleList := make([]*longhorn.SupportBundle, len(supportBundles))
	supportBundleNames, err := util.SortKeys(supportBundles)
	if err != nil {
		return []*longhorn.SupportBundle{}, err
	}
	for i, name := range supportBundleNames {
		supportBundleList[i] = supportBundles[name]
	}
	return supportBundleList, nil
}
