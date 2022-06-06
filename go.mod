module github.com/harvester/harvester

go 1.18

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
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20211208233239-77392a65423d
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20211208233239-77392a65423d

	helm.sh/helm/v3 => github.com/rancher/helm/v3 v3.7.1-rancher.1
	k8s.io/api => k8s.io/api v0.21.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.21.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.5
	k8s.io/apiserver => k8s.io/apiserver v0.21.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.21.5
	k8s.io/client-go => k8s.io/client-go v0.21.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.21.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.21.5
	k8s.io/code-generator => k8s.io/code-generator v0.21.5
	k8s.io/component-base => k8s.io/component-base v0.21.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.21.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.21.5
	k8s.io/cri-api => k8s.io/cri-api v0.21.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.21.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.21.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.21.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.21.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.21.5
	k8s.io/kubectl => k8s.io/kubectl v0.21.5
	k8s.io/kubelet => k8s.io/kubelet v0.21.5
	k8s.io/kubernetes => k8s.io/kubernetes v1.21.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.21.5
	k8s.io/metrics => k8s.io/metrics v0.21.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.21.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.21.5
	kubevirt.io/api => github.com/kubevirt/api v0.49.0

	kubevirt.io/client-go => github.com/kubevirt/client-go v0.49.0
	kubevirt.io/containerized-data-importer => github.com/kubevirt/containerized-data-importer v1.41.0
	kubevirt.io/containerized-data-importer-api => github.com/kubevirt/containerized-data-importer-api v1.41.0
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v0.4.4
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)

require (
	github.com/containernetworking/cni v0.8.1
	github.com/ehazlett/simplelog v0.0.0-20200226020431-d374894e92a4
	github.com/emicklei/go-restful v2.10.0+incompatible
	github.com/go-openapi/spec v0.20.3
	github.com/gorilla/mux v1.8.0
	github.com/guonaihong/gout v0.1.3
	github.com/harvester/harvester-network-controller v0.1.7
	github.com/iancoleman/strcase v0.1.2
	github.com/k3s-io/helm-controller v0.11.7
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200331171230-d50e42f2b669
	github.com/kubernetes-csi/external-snapshotter/v2 v2.1.1
	github.com/kubernetes/dashboard v1.10.1
	github.com/longhorn/backupstore v0.0.0-20210908163358-43c9d3298665
	github.com/longhorn/longhorn-manager v1.2.4
	github.com/mattn/go-isatty v0.0.14
	github.com/mcuadros/go-version v0.0.0-20190830083331-035f6764e8d2
	github.com/mitchellh/mapstructure v1.4.2
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/openshift/api v0.0.0
	github.com/pkg/errors v0.9.1
	github.com/rancher/apiserver v0.0.0-20211025232108-df28932a5627
	github.com/rancher/dynamiclistener v0.3.1-0.20211104200948-cd5d71f2fe95
	github.com/rancher/fleet/pkg/apis v0.0.0-20210918015053-5a141a6b18f0
	github.com/rancher/lasso v0.0.0-20210709145333-6c6cd7fd6607
	github.com/rancher/norman v0.0.0-20211201154850-abe17976423e
	github.com/rancher/rancher v0.0.0-20211208233239-77392a65423d
	github.com/rancher/rancher/pkg/apis v0.0.0
	github.com/rancher/steve v0.0.0-20220126170519-376e30bba7be
	github.com/rancher/system-upgrade-controller/pkg/apis v0.0.0-20210727200656-10b094e30007
	github.com/rancher/wrangler v0.8.11-0.20211214201934-f5aa5d9f2e81
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.9.3
	github.com/urfave/cli v1.22.2
	golang.org/x/crypto v0.0.0-20211202192323-5770296d904e
	golang.org/x/net v0.0.0-20211123203042-d83791d6bcd9
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	helm.sh/helm/v3 v3.7.1
	k8s.io/api v0.22.3
	k8s.io/apiextensions-apiserver v0.22.3
	k8s.io/apimachinery v0.22.3
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/gengo v0.0.0-20201214224949-b6c5ce23f027
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	kubevirt.io/api v0.0.0-20220111180619-bd15f69822b9
	kubevirt.io/containerized-data-importer-api v1.41.0
	kubevirt.io/kubevirt v0.49.0
	sigs.k8s.io/cluster-api v0.4.4
	sigs.k8s.io/kind v0.11.1
	sigs.k8s.io/yaml v1.2.0
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/BurntSushi/toml v0.4.1 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/Masterminds/sprig/v3 v3.2.2 // indirect
	github.com/Masterminds/squirrel v1.5.0 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/RoaringBitmap/roaring v0.4.18 // indirect
	github.com/adrg/xdg v0.3.1 // indirect
	github.com/alessio/shellescape v1.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20200907205600-7a23bdc65eef // indirect
	github.com/aws/aws-sdk-go v1.38.65 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/containerd/containerd v1.5.10 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/cli v20.10.7+incompatible // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v20.10.6+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.3 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/evanphx/json-patch v4.11.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.2.0 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gin-gonic/gin v1.7.0 // indirect
	github.com/glycerine/go-unsnap-stream v0.0.0-20181221182339-f9677308dec2 // indirect
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.3 // indirect
	github.com/google/btree v1.0.0 // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/honestbee/jobq v1.0.2 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jinzhu/copier v0.0.0-20190924061706-b57f9002281a // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jmoiron/sqlx v1.3.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.11 // indirect
	github.com/klauspost/compress v1.13.1 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.10.3 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/longhorn/backing-image-manager v0.0.0-20210809125601-48e29abcd637 // indirect
	github.com/longhorn/go-iscsi-helper v0.0.0-20201111045018-ee87992ec536 // indirect
	github.com/longhorn/longhorn-instance-manager v0.0.0-20210729081215-50c310f97378 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-colorable v0.1.11 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/copystructure v1.1.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.1 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/term v0.0.0-20201216013528-df9cb8a40635 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mschoch/smat v0.0.0-20160514031455-90eadee771ae // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/openshift/custom-resource-status v0.0.0-20200602122900-c002fd1547ca // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/pelletier/go-toml v1.9.4 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/philhofer/fwd v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.11.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/rancher/aks-operator v1.0.2 // indirect
	github.com/rancher/eks-operator v1.1.1 // indirect
	github.com/rancher/gke-operator v1.1.1 // indirect
	github.com/rancher/kubernetes-provider-detector v0.1.5 // indirect
	github.com/rancher/remotedialer v0.2.6-0.20220104192242-f3837f8d649a // indirect
	github.com/rancher/rke v1.3.3-rc4 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/rubenv/sql-migrate v0.0.0-20210614095031-55d5740dbbcc // indirect
	github.com/russross/blackfriday v1.5.2 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/cobra v1.2.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tinylib/msgp v1.1.1-0.20190612170807-0573788bc2a8 // indirect
	github.com/willf/bitset v1.1.11 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190809123943-df4f5c81cb3b // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v0.0.0-20181112141820-a009c3971eca // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	golang.org/x/mod v0.5.1 // indirect
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f // indirect
	golang.org/x/sys v0.0.0-20211013075003-97ac67df715c // indirect
	golang.org/x/term v0.0.0-20210220032956-6a3ed077a48d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	golang.org/x/tools v0.1.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210828152312-66f60bf46e71 // indirect
	google.golang.org/grpc v1.40.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/gorp.v1 v1.7.2 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/apiserver v0.22.3 // indirect
	k8s.io/cli-runtime v0.22.3 // indirect
	k8s.io/code-generator v0.21.5 // indirect
	k8s.io/component-base v0.21.5 // indirect
	k8s.io/component-helpers v0.21.5 // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.10.0 // indirect
	k8s.io/kube-aggregator v0.21.0 // indirect
	k8s.io/kubectl v0.22.3 // indirect
	k8s.io/kubernetes v1.21.0 // indirect
	kubevirt.io/client-go v0.49.0 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk v0.2.1 // indirect
	oras.land/oras-go v0.4.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.22 // indirect
	sigs.k8s.io/cli-utils v0.16.0 // indirect
	sigs.k8s.io/controller-runtime v0.9.7 // indirect
	sigs.k8s.io/kustomize/api v0.8.8 // indirect
	sigs.k8s.io/kustomize/kyaml v0.10.17 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.1.2 // indirect
)
