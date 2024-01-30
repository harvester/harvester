package client

const (
	KUBERNETES_STATUS_TYPE = "kubernetesStatus"
)

type KubernetesStatus struct {
	Resource `yaml:"-"`

	LastPVCRefAt string `json:"lastPVCRefAt,omitempty" yaml:"last_pvcref_at,omitempty"`

	LastPodRefAt string `json:"lastPodRefAt,omitempty" yaml:"last_pod_ref_at,omitempty"`

	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	PvName string `json:"pvName,omitempty" yaml:"pv_name,omitempty"`

	PvStatus string `json:"pvStatus,omitempty" yaml:"pv_status,omitempty"`

	PvcName string `json:"pvcName,omitempty" yaml:"pvc_name,omitempty"`

	WorkloadsStatus []WorkloadStatus `json:"workloadsStatus,omitempty" yaml:"workloads_status,omitempty"`
}

type KubernetesStatusCollection struct {
	Collection
	Data   []KubernetesStatus `json:"data,omitempty"`
	client *KubernetesStatusClient
}

type KubernetesStatusClient struct {
	rancherClient *RancherClient
}

type KubernetesStatusOperations interface {
	List(opts *ListOpts) (*KubernetesStatusCollection, error)
	Create(opts *KubernetesStatus) (*KubernetesStatus, error)
	Update(existing *KubernetesStatus, updates interface{}) (*KubernetesStatus, error)
	ById(id string) (*KubernetesStatus, error)
	Delete(container *KubernetesStatus) error
}

func newKubernetesStatusClient(rancherClient *RancherClient) *KubernetesStatusClient {
	return &KubernetesStatusClient{
		rancherClient: rancherClient,
	}
}

func (c *KubernetesStatusClient) Create(container *KubernetesStatus) (*KubernetesStatus, error) {
	resp := &KubernetesStatus{}
	err := c.rancherClient.doCreate(KUBERNETES_STATUS_TYPE, container, resp)
	return resp, err
}

func (c *KubernetesStatusClient) Update(existing *KubernetesStatus, updates interface{}) (*KubernetesStatus, error) {
	resp := &KubernetesStatus{}
	err := c.rancherClient.doUpdate(KUBERNETES_STATUS_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *KubernetesStatusClient) List(opts *ListOpts) (*KubernetesStatusCollection, error) {
	resp := &KubernetesStatusCollection{}
	err := c.rancherClient.doList(KUBERNETES_STATUS_TYPE, opts, resp)
	resp.client = c
	return resp, err
}

func (cc *KubernetesStatusCollection) Next() (*KubernetesStatusCollection, error) {
	if cc != nil && cc.Pagination != nil && cc.Pagination.Next != "" {
		resp := &KubernetesStatusCollection{}
		err := cc.client.rancherClient.doNext(cc.Pagination.Next, resp)
		resp.client = cc.client
		return resp, err
	}
	return nil, nil
}

func (c *KubernetesStatusClient) ById(id string) (*KubernetesStatus, error) {
	resp := &KubernetesStatus{}
	err := c.rancherClient.doById(KUBERNETES_STATUS_TYPE, id, resp)
	if apiError, ok := err.(*ApiError); ok {
		if apiError.StatusCode == 404 {
			return nil, nil
		}
	}
	return resp, err
}

func (c *KubernetesStatusClient) Delete(container *KubernetesStatus) error {
	return c.rancherClient.doResourceDelete(KUBERNETES_STATUS_TYPE, &container.Resource)
}
