module github.com/rancher/harvester

go 1.13

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.2.0+incompatible
	github.com/crewjam/saml => github.com/rancher/saml v0.0.0-20180713225824-ce1532152fde
	github.com/knative/pkg => github.com/rancher/pkg v0.0.0-20190514055449-b30ab9de040e
	github.com/rancher/apiserver => github.com/rancheredge/apiserver v0.0.0-20200731031228-a0459feeb0de
	github.com/rancher/steve => github.com/rancheredge/steve v0.0.0-20200708031911-f69e0f4820b4
	k8s.io/client-go => k8s.io/client-go v0.18.0
	kubevirt.io/client-go => github.com/orangedeng/client-go v0.31.1-0.20200715061104-844cb60487e4
	kubevirt.io/containerized-data-importer => github.com/thxcode/kubevirt-containerized-data-importer v1.22.0-apis-only
)

require (
	github.com/Masterminds/squirrel v1.4.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20200428143746-21a406dcc535 // indirect
	github.com/containerd/containerd v1.3.4 // indirect
	github.com/docker/docker v17.12.0-ce-rc1.0.20200618181300-9dc6525e6118+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/gorilla/mux v1.7.3
	github.com/guonaihong/gout v0.1.2
	github.com/iancoleman/strcase v0.1.1
	github.com/lib/pq v1.7.0 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/minio/minio-go/v6 v6.0.57
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/rancher/apiserver v0.0.0-20200721152301-4388bb184a8e
	github.com/rancher/dynamiclistener v0.2.1-0.20200213165308-111c5b43e932
	github.com/rancher/lasso v0.0.0-20200515155337-a34e1e26ad91
	github.com/rancher/steve v0.0.0-20200622175150-3dbc369174fb
	github.com/rancher/wrangler v0.6.2-0.20200622171942-7224e49a2407
	github.com/rancher/wrangler-api v0.6.1-0.20200515193802-dcf70881b087
	github.com/rubenv/sql-migrate v0.0.0-20200616145509-8d140a17f351 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/tidwall/gjson v1.6.0
	github.com/tidwall/sjson v1.1.1
	github.com/urfave/cli v1.22.2
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	helm.sh/helm/v3 v3.2.4
	k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/gengo v0.0.0-20200114144118-36b2048a9120
	k8s.io/kubectl v0.18.8 // indirect
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	kubevirt.io/client-go v0.31.1-0.20200715061104-844cb60487e4
	kubevirt.io/containerized-data-importer v1.22.0
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/kind v0.8.1
)
