module github.com/rancher/harvester

go 1.13

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.2.0+incompatible
	github.com/crewjam/saml => github.com/rancher/saml v0.0.0-20180713225824-ce1532152fde
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go v3.2.1-0.20200107013213-dc14462fd587+incompatible
	github.com/knative/pkg => github.com/rancher/pkg v0.0.0-20190514055449-b30ab9de040e
	github.com/rancher/apiserver => github.com/cnrancher/apiserver v0.0.0-20200731031228-a0459feeb0de
	github.com/rancher/steve => github.com/cnrancher/steve v0.0.0-20200922090254-a3cedc4d23cd
	k8s.io/client-go => k8s.io/client-go v0.18.0
	k8s.io/code-generator => k8s.io/code-generator v0.18.3
	kubevirt.io/client-go => github.com/cnrancher/kubevirt-client-go v0.31.1-0.20200715061104-844cb60487e4
	kubevirt.io/containerized-data-importer => github.com/cnrancher/kubevirt-containerized-data-importer v1.22.0-apis-only
)

require (
	github.com/containernetworking/cni v0.8.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200331171230-d50e42f2b669
	github.com/kubernetes/dashboard v1.10.1
	github.com/minio/minio-go/v6 v6.0.57
	github.com/pkg/errors v0.9.1
	github.com/rancher/apiserver v0.0.0-20200721152301-4388bb184a8e
	github.com/rancher/dynamiclistener v0.2.1-0.20200213165308-111c5b43e932
	github.com/rancher/lasso v0.0.0-20200515155337-a34e1e26ad91
	github.com/rancher/steve v0.0.0-20200622175150-3dbc369174fb
	github.com/rancher/wrangler v0.6.2-0.20200622171942-7224e49a2407
	github.com/rancher/wrangler-api v0.6.1-0.20200515193802-dcf70881b087
	github.com/sirupsen/logrus v1.5.0
	github.com/stretchr/testify v1.5.1
	github.com/urfave/cli v1.22.2
	golang.org/x/crypto v0.0.0-20200414173820-0848c9571904
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	gopkg.in/square/go-jose.v2 v2.2.2
	k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver v0.18.0
	k8s.io/apimachinery v0.18.6
	k8s.io/apiserver v0.18.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.18.3 // indirect
	k8s.io/gengo v0.0.0-20200114144118-36b2048a9120
	k8s.io/kubernetes v1.14.0
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	kubevirt.io/client-go v0.31.1-0.20200715061104-844cb60487e4
	kubevirt.io/containerized-data-importer v1.22.0
)
