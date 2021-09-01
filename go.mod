module github.com/harvester/harvester

go 1.16

replace (
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go v3.2.1-0.20200107013213-dc14462fd587+incompatible
	github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d
	github.com/docker/docker => github.com/docker/docker v1.4.2-0.20200203170920-46ec8731fbce
	github.com/go-kit/kit => github.com/go-kit/kit v0.3.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	github.com/knative/pkg => github.com/rancher/pkg v0.0.0-20190514055449-b30ab9de040e
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20210702001641-82b212ddba18
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20210702001641-82b212ddba18

	helm.sh/helm/v3 => github.com/rancher/helm/v3 v3.5.4-rancher.1
	k8s.io/api => k8s.io/api v0.21.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.21.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.2
	k8s.io/apiserver => k8s.io/apiserver v0.21.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.21.2
	k8s.io/client-go => k8s.io/client-go v0.21.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.21.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.21.2
	k8s.io/code-generator => k8s.io/code-generator v0.21.2
	k8s.io/component-base => k8s.io/component-base v0.21.2
	k8s.io/component-helpers => k8s.io/component-helpers v0.21.2
	k8s.io/controller-manager => k8s.io/controller-manager v0.21.2
	k8s.io/cri-api => k8s.io/cri-api v0.21.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.21.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.21.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.21.2
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.21.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.21.2
	k8s.io/kubectl => k8s.io/kubectl v0.21.2
	k8s.io/kubelet => k8s.io/kubelet v0.21.2
	k8s.io/kubernetes => k8s.io/kubernetes v1.21.2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.21.2
	k8s.io/metrics => k8s.io/metrics v0.21.2
	k8s.io/mount-utils => k8s.io/mount-utils v0.21.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.21.2

	kubevirt.io/client-go => github.com/kubevirt/client-go v0.44.0
	kubevirt.io/containerized-data-importer => github.com/rancher/kubevirt-containerized-data-importer v1.26.1-0.20210802100720-9bcf4e7ba0ce
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)

require (
	github.com/aws/aws-sdk-go-v2 v1.6.0
	github.com/aws/aws-sdk-go-v2/config v1.3.0
	github.com/aws/aws-sdk-go-v2/credentials v1.2.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.9.0
	github.com/containernetworking/cni v0.8.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/ehazlett/simplelog v0.0.0-20200226020431-d374894e92a4
	github.com/emicklei/go-restful v2.10.0+incompatible
	github.com/go-openapi/spec v0.20.3
	github.com/gorilla/mux v1.8.0
	github.com/guonaihong/gout v0.1.3
	github.com/harvester/harvester-network-controller v0.1.7
	github.com/iancoleman/strcase v0.1.2
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200331171230-d50e42f2b669
	github.com/kubernetes-csi/external-snapshotter/v2 v2.1.1
	github.com/kubernetes/dashboard v1.10.1
	github.com/longhorn/backupstore v0.0.0-20210817080617-8ea3843e6b0d
	github.com/longhorn/longhorn-manager v1.2.0
	github.com/mattn/go-isatty v0.0.12
	github.com/mcuadros/go-version v0.0.0-20190830083331-035f6764e8d2
	github.com/onsi/ginkgo v1.16.1
	github.com/onsi/gomega v1.11.0
	github.com/openshift/api v0.0.0
	github.com/pkg/errors v0.9.1
	github.com/rancher/apiserver v0.0.0-20210727155917-6a723678dd3d
	github.com/rancher/dynamiclistener v0.3.1-0.20210803172359-94e22490cfa8
	github.com/rancher/lasso v0.0.0-20210616224652-fc3ebd901c08
	github.com/rancher/norman v0.0.0-20210608202517-59b3523c3133
	github.com/rancher/rancher v0.0.0-20210702001641-82b212ddba18
	github.com/rancher/rancher/pkg/apis v0.0.0
	github.com/rancher/steve v0.0.0-20210804220357-bb76e4db5669
	github.com/rancher/system-upgrade-controller/pkg/apis v0.0.0-20210424054953-634d28b7def3
	github.com/rancher/wrangler v0.8.1-0.20210618171953-ab479ee75244
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.8.0
	github.com/urfave/cli v1.22.2
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	gopkg.in/square/go-jose.v2 v2.5.1
	helm.sh/helm/v3 v3.5.4
	k8s.io/api v0.21.2
	k8s.io/apiextensions-apiserver v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/apiserver v0.21.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/gengo v0.0.0-20201214224949-b6c5ce23f027
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	k8s.io/kubernetes v1.21.2
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10
	kubevirt.io/client-go v0.44.0
	kubevirt.io/containerized-data-importer v1.36.0
	kubevirt.io/kubevirt v0.44.0
	sigs.k8s.io/kind v0.11.1
	sigs.k8s.io/yaml v1.2.0
)
