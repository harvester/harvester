module github.com/rancher/harvester

go 1.13

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.2.0+incompatible
	github.com/crewjam/saml => github.com/rancher/saml v0.0.0-20180713225824-ce1532152fde
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go v3.2.1-0.20200107013213-dc14462fd587+incompatible
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.3.1
	github.com/knative/pkg => github.com/rancher/pkg v0.0.0-20190514055449-b30ab9de040e
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20191125132246-f6563a70e19a
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a
	github.com/rancher/apiserver => github.com/cnrancher/apiserver v0.0.0-20210111070015-74fc636626b5
	github.com/rancher/steve => github.com/cnrancher/steve v0.0.0-20210111071104-8891a7756b81
	github.com/rancher/wrangler => github.com/rancher/wrangler v0.7.3-0.20201204033916-d7f8c90b22e5
	k8s.io/api => k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.6
	k8s.io/apiserver => k8s.io/apiserver v0.18.6
	k8s.io/client-go => k8s.io/client-go v0.18.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.4
	k8s.io/code-generator => k8s.io/code-generator v0.18.3
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	kubevirt.io/client-go => github.com/rancher/kubevirt-client-go v0.20.2-0.20201124032527-ff3943be35bf
	kubevirt.io/containerized-data-importer => github.com/rancher/kubevirt-containerized-data-importer v1.22.1-0.20201127032458-4f73ed532d31
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)

require (
	github.com/containernetworking/cni v0.8.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/ehazlett/simplelog v0.0.0-20200226020431-d374894e92a4
	github.com/gorilla/mux v1.7.4
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/guonaihong/gout v0.1.3
	github.com/iancoleman/strcase v0.1.2
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200331171230-d50e42f2b669
	github.com/kubernetes/dashboard v1.10.1
	github.com/mattn/go-isatty v0.0.12
	github.com/minio/minio-go/v6 v6.0.57
	github.com/nxadm/tail v1.4.5 // indirect
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	github.com/rancher/apiserver v0.0.0-20201023000256-1a0a904f9197
	github.com/rancher/dynamiclistener v0.2.1-0.20200714201033-9c1939da3af9
	github.com/rancher/lasso v0.0.0-20200905045615-7fcb07d6a20b
	github.com/rancher/steve v0.0.0-20201110183734-21c7add15f64
	github.com/rancher/wrangler v0.7.1
	github.com/rancher/wrangler-api v0.6.1-0.20200515193802-dcf70881b087
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/tidwall/gjson v1.6.3
	github.com/urfave/cli v1.22.2
	golang.org/x/crypto v0.0.0-20200423211502-4bdfaf469ed5
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	golang.org/x/sys v0.0.0-20201221093633-bc327ba9c2f0 // indirect
	gopkg.in/square/go-jose.v2 v2.3.1
	gopkg.in/yaml.v2 v2.4.0 // indirect
	helm.sh/helm/v3 v3.2.0
	k8s.io/api v0.19.0-rc.2
	k8s.io/apiextensions-apiserver v0.19.0-rc.2
	k8s.io/apimachinery v0.19.0-rc.2
	k8s.io/apiserver v0.19.0-rc.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/gengo v0.0.0-20200413195148-3a45101e95ac
	k8s.io/kubernetes v1.14.0
	k8s.io/utils v0.0.0-20200720150651-0bdb4ca86cbc
	kubevirt.io/client-go v0.0.0-00010101000000-000000000000
	kubevirt.io/containerized-data-importer v1.26.1
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/kind v0.9.0
	sigs.k8s.io/yaml v1.2.0
)
