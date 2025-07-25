module github.com/harvester/harvester

go 1.24.2

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.7.27
	github.com/docker/distribution => github.com/docker/distribution v2.8.2+incompatible // oras dep requires a replace is set
	github.com/docker/docker => github.com/docker/docker v25.0.6+incompatible // oras dep requires a replace is set
	github.com/gin-gonic/gin => github.com/gin-gonic/gin v1.9.1
	github.com/harvester/harvester-network-controller => github.com/harvester/harvester-network-controller v1.6.0-rc3
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring => github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.68.0
	github.com/rancher/lasso => github.com/rancher/lasso v0.0.0-20241202185148-04649f379358

	github.com/rancher/rancher => github.com/rancher/rancher v0.0.0-20240919204204-3da2ae0cabd1
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20240919204204-3da2ae0cabd1
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20240919204204-3da2ae0cabd1
	// handle rancher dependenices
	go.qase.io/client => github.com/rancher/qase-go/client v0.0.0-20231114201952-65195ec001fa

	helm.sh/helm/v3 => github.com/rancher/helm/v3 v3.15.1-rancher2
	k8s.io/api => k8s.io/api v0.31.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.31.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.31.5
	k8s.io/apiserver => k8s.io/apiserver v0.31.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.31.5
	k8s.io/client-go => k8s.io/client-go v0.31.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.31.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.31.5
	k8s.io/code-generator => k8s.io/code-generator v0.31.5
	k8s.io/component-base => k8s.io/component-base v0.31.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.31.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.31.5
	k8s.io/cri-api => k8s.io/cri-api v0.31.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.31.5
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.31.5
	k8s.io/endpointslice => k8s.io/endpointslice v0.31.5
	k8s.io/gengo => k8s.io/gengo v0.0.0-20240826214909-a7b603a56eb7
	k8s.io/gengo/v2 => k8s.io/gengo/v2 v2.0.0-20240228010128-51d4e06bde70
	k8s.io/kms => k8s.io/kms v0.31.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.31.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.31.5
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.31.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.31.5
	k8s.io/kubectl => k8s.io/kubectl v0.31.5
	k8s.io/kubelet => k8s.io/kubelet v0.31.5
	k8s.io/kubernetes => k8s.io/kubernetes v1.31.6
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.31.5
	k8s.io/metrics => k8s.io/metrics v0.31.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.31.5
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.31.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.31.5
)

require (
	github.com/cisco-open/operator-tools v0.34.0
	github.com/deckarep/golang-set/v2 v2.6.0
	github.com/ehazlett/simplelog v0.0.0-20200226020431-d374894e92a4
	github.com/emicklei/go-restful/v3 v3.12.2
	github.com/go-errors/errors v1.4.2
	github.com/gobuffalo/flect v1.0.2
	github.com/gorilla/mux v1.8.1
	github.com/guonaihong/gout v0.1.3
	github.com/harvester/go-common v0.0.0-20250109132713-e748ce72a7ba
	github.com/harvester/harvester-network-controller v1.6.0-rc3
	github.com/harvester/node-manager v1.6.0-rc3
	github.com/iancoleman/strcase v0.2.0
	github.com/k3s-io/helm-controller v0.16.1
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.7.5
	github.com/k8snetworkplumbingwg/whereabouts v0.8.0
	github.com/kube-logging/logging-operator/pkg/sdk v0.11.1-0.20240314152935-421fefebc813
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0
	github.com/kubernetes/dashboard v1.10.1
	github.com/longhorn/backupstore v0.0.0-20250227220202-651bd33886fe
	github.com/longhorn/longhorn-manager v1.8.1
	github.com/mattn/go-isatty v0.0.20
	github.com/mcuadros/go-version v0.0.0-20190830083331-035f6764e8d2
	github.com/mitchellh/mapstructure v1.5.0
	github.com/onsi/ginkgo/v2 v2.23.3
	github.com/onsi/gomega v1.37.0
	github.com/openshift/api v0.0.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.78.2
	github.com/rancher/apiserver v0.0.0-20241009200134-5a4ecca7b988
	github.com/rancher/dynamiclistener v0.6.1
	github.com/rancher/fleet/pkg/apis v0.10.0
	github.com/rancher/lasso v0.2.3
	github.com/rancher/norman v0.0.0-20241001183610-78a520c160ab
	github.com/rancher/rancher v0.0.0-20240618122559-b9ec494d4f6f
	github.com/rancher/rancher/pkg/apis v0.0.0
	github.com/rancher/steve v0.5.6
	github.com/rancher/system-upgrade-controller/pkg/apis v0.0.0-20240301001845-4eacc2dabbde
	github.com/rancher/wrangler v1.1.2
	github.com/rancher/wrangler/v3 v3.2.2
	github.com/robfig/cron v1.2.0
	github.com/shopspring/decimal v1.3.1
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.8.1
	github.com/stretchr/testify v1.10.0
	github.com/tidwall/gjson v1.14.2
	github.com/urfave/cli v1.22.16
	go.uber.org/multierr v1.11.0
	golang.org/x/crypto v0.38.0
	golang.org/x/net v0.40.0
	golang.org/x/sync v0.14.0
	golang.org/x/text v0.25.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	helm.sh/helm/v3 v3.17.3
	k8s.io/api v0.33.1
	k8s.io/apiextensions-apiserver v0.33.1
	k8s.io/apimachinery v0.33.1
	k8s.io/apiserver v0.33.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/component-helpers v0.32.2
	k8s.io/kube-openapi v0.31.9
	k8s.io/kubectl v0.30.1
	k8s.io/kubelet v0.26.13
	k8s.io/utils v0.0.0-20250502105355-0f33e8f1c979
	kubevirt.io/api v1.5.0
	kubevirt.io/client-go v1.5.0
	kubevirt.io/containerized-data-importer v1.61.0
	kubevirt.io/containerized-data-importer-api v1.61.0
	kubevirt.io/kubevirt v1.5.0
	sigs.k8s.io/cluster-api v1.7.3
	sigs.k8s.io/kind v0.24.0
	sigs.k8s.io/kustomize/kyaml v0.17.1
	sigs.k8s.io/yaml v1.4.0
)

require (
	cel.dev/expr v0.20.0 // indirect
	emperror.dev/errors v0.8.1 // indirect
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20230811130428-ced1acdcaa24 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/BurntSushi/toml v1.4.0 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.3.0 // indirect
	github.com/Masterminds/sprig/v3 v3.2.3 // indirect
	github.com/Masterminds/squirrel v1.5.4 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2 // indirect
	github.com/adrg/xdg v0.5.0 // indirect
	github.com/alessio/shellescape v1.4.2 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go v1.55.6 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8 // indirect
	github.com/cenkalti/backoff/v5 v5.0.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/containerd/containerd v1.7.12 // indirect
	github.com/containerd/errdefs v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v0.2.1 // indirect
	github.com/containernetworking/cni v1.3.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/cyphar/filepath-securejoin v0.2.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/cli v25.0.3+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker v27.1.1+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/evanphx/json-patch v5.9.11+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20210407135951-1de76d718b3f // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/gammazero/deque v1.0.0 // indirect
	github.com/gammazero/workerpool v1.1.3 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gin-gonic/gin v1.7.0 // indirect
	github.com/go-gorp/gorp/v3 v3.1.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.3 // indirect
	github.com/go-kit/kit v0.13.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.1 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.1 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.2.5 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/gomodule/redigo v2.0.0+incompatible // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/cel-go v0.23.2 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20241210010833-40e02aabc2ad // indirect
	github.com/google/safetext v0.0.0-20220905092116-b49f7bc46da2 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/handlers v1.5.2 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/iancoleman/orderedmap v0.3.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jmoiron/sqlx v1.3.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/kubernetes-csi/external-snapshotter/client/v6 v6.0.1 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/longhorn/backing-image-manager v1.8.0 // indirect
	github.com/longhorn/go-common-libs v0.0.0-20250215052214-151615b29f8e // indirect
	github.com/longhorn/go-iscsi-helper v0.0.0-20250111093313-7e1930499625 // indirect
	github.com/longhorn/go-spdk-helper v0.0.0-20250116051812-eabe34a4219e // indirect
	github.com/longhorn/longhorn-engine v1.8.0 // indirect
	github.com/longhorn/longhorn-instance-manager v1.8.0 // indirect
	github.com/longhorn/longhorn-share-manager v1.8.0 // indirect
	github.com/longhorn/types v0.0.0-20250210001806-5ef312749bee // indirect
	github.com/machadovilaca/operator-observability v0.0.22 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.4.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/opencontainers/runc v1.1.13 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/openshift/client-go v3.9.0+incompatible // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/openshift/library-go v0.0.0-20240621150525-4bb4238aef81 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rancher/aks-operator v1.9.2 // indirect
	github.com/rancher/eks-operator v1.9.2 // indirect
	github.com/rancher/gke-operator v1.9.2 // indirect
	github.com/rancher/kubernetes-provider-detector v0.1.5 // indirect
	github.com/rancher/remotedialer v0.4.0 // indirect
	github.com/rancher/rke v1.6.2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/rubenv/sql-migrate v1.5.2 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/slok/goresilience v0.2.0 // indirect
	github.com/spf13/cast v1.8.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/etcd/api/v3 v3.5.16 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.16 // indirect
	go.etcd.io/etcd/client/v3 v3.5.16 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.58.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.36.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.36.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	go.opentelemetry.io/proto/otlp v1.6.0 // indirect
	go.starlark.net v0.0.0-20230525235612-a134d8f9ddca // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20250506013437-ce4c2cf36ca6 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools v0.33.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250519155744-55703ea1f237 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250519155744-55703ea1f237 // indirect
	google.golang.org/grpc v1.72.2 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	k8s.io/cli-runtime v0.32.2 // indirect
	k8s.io/code-generator v0.33.1 // indirect
	k8s.io/component-base v0.33.1 // indirect
	k8s.io/gengo v0.0.0-20250130153323-76c5745d3511 // indirect
	k8s.io/gengo/v2 v2.0.0-20250207200755-1244d31929d7 // indirect
	k8s.io/helm v2.17.0+incompatible // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kms v0.32.2 // indirect
	k8s.io/kube-aggregator v0.33.1 // indirect
	k8s.io/kubernetes v1.32.2 // indirect
	k8s.io/mount-utils v0.32.2 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	modernc.org/gc/v3 v3.0.0-20240107210532-573471604cb6 // indirect
	modernc.org/libc v1.49.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
	modernc.org/sqlite v1.29.10 // indirect
	modernc.org/strutil v1.2.0 // indirect
	modernc.org/token v1.1.0 // indirect
	oras.land/oras-go v1.2.5 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.32.1 // indirect
	sigs.k8s.io/cli-utils v0.37.2 // indirect
	sigs.k8s.io/controller-runtime v0.19.7 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/kustomize/api v0.17.2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
)
