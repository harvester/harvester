module github.com/harvester/harvester

go 1.20

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.6.18
	github.com/docker/distribution => github.com/docker/distribution v2.8.0+incompatible // oras dep requires a replace is set
	github.com/docker/docker => github.com/docker/docker v20.10.9+incompatible // oras dep requires a replace is set
	github.com/gin-gonic/gin => github.com/gin-gonic/gin v1.7.7
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20230124173128-2207cfed1803
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20230124173128-2207cfed1803

	helm.sh/helm/v3 => github.com/rancher/helm/v3 v3.9.0-rancher1
	k8s.io/api => k8s.io/api v0.26.10
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.26.10
	k8s.io/apimachinery => k8s.io/apimachinery v0.26.10
	k8s.io/apiserver => k8s.io/apiserver v0.26.10
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.26.10
	k8s.io/client-go => k8s.io/client-go v0.26.10
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.26.10
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.26.10
	k8s.io/code-generator => k8s.io/code-generator v0.26.10
	k8s.io/component-base => k8s.io/component-base v0.26.10
	k8s.io/component-helpers => k8s.io/component-helpers v0.26.10
	k8s.io/controller-manager => k8s.io/controller-manager v0.26.10
	k8s.io/cri-api => k8s.io/cri-api v0.26.10
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.26.10
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.26.10
	k8s.io/kms => k8s.io/kms v0.26.10
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.26.10
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.26.10
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.26.10
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.26.10
	k8s.io/kubectl => k8s.io/kubectl v0.26.10
	k8s.io/kubelet => k8s.io/kubelet v0.26.10
	k8s.io/kubernetes => k8s.io/kubernetes v1.26.10
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.26.10
	k8s.io/metrics => k8s.io/metrics v0.26.10
	k8s.io/mount-utils => k8s.io/mount-utils v0.26.10
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.26.10
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.26.10

	kubevirt.io/api => kubevirt.io/api v1.0.0
	kubevirt.io/client-go => kubevirt.io/client-go v1.0.0
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)

require (
	github.com/Masterminds/semver/v3 v3.2.0
	github.com/cisco-open/operator-tools v0.29.0
	github.com/containernetworking/cni v1.1.2
	github.com/docker/go-units v0.5.0
	github.com/ehazlett/simplelog v0.0.0-20200226020431-d374894e92a4
	github.com/emicklei/go-restful/v3 v3.9.0
	github.com/gobuffalo/flect v1.0.2
	github.com/gorilla/mux v1.8.0
	github.com/guonaihong/gout v0.1.3
	github.com/harvester/harvester-network-controller v0.3.1
	github.com/harvester/node-manager v0.1.5-0.20230614075852-de2da3ef3aca
	github.com/iancoleman/strcase v0.2.0
	github.com/k3s-io/helm-controller v0.11.7
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.3.0
	github.com/kube-logging/logging-operator/pkg/sdk v0.9.1
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0
	github.com/kubernetes/dashboard v1.10.1
	github.com/longhorn/backupstore v0.0.0-20231025000449-5316e8b5320d
	github.com/longhorn/longhorn-manager v1.5.3
	github.com/mattn/go-isatty v0.0.17
	github.com/mcuadros/go-version v0.0.0-20190830083331-035f6764e8d2
	github.com/mitchellh/mapstructure v1.5.0
	github.com/onsi/ginkgo/v2 v2.9.4
	github.com/onsi/gomega v1.27.6
	github.com/openshift/api v0.0.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.63.0
	github.com/rancher/apiserver v0.0.0-20230120214941-e88c32739dc7
	github.com/rancher/dynamiclistener v0.3.5
	github.com/rancher/fleet/pkg/apis v0.0.0-20230123175930-d296259590be
	github.com/rancher/lasso v0.0.0-20221227210133-6ea88ca2fbcc
	github.com/rancher/norman v0.0.0-20221205184727-32ef2e185b99
	github.com/rancher/rancher v0.0.0-20230124173128-2207cfed1803
	github.com/rancher/rancher/pkg/apis v0.0.0
	github.com/rancher/steve v0.0.0-20221209194631-acf9d31ce0dd
	github.com/rancher/system-upgrade-controller/pkg/apis v0.0.0-20210727200656-10b094e30007
	github.com/rancher/wharfie v0.5.3
	github.com/rancher/wrangler v1.1.1
	github.com/sirupsen/logrus v1.9.2
	github.com/stretchr/testify v1.8.3
	github.com/tidwall/gjson v1.9.3
	github.com/urfave/cli v1.22.13
	go.uber.org/multierr v1.11.0
	golang.org/x/crypto v0.14.0
	golang.org/x/exp v0.0.0-20230522175609-2e198f4a06a1
	golang.org/x/net v0.17.0
	golang.org/x/sync v0.4.0
	golang.org/x/text v0.14.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	helm.sh/helm/v3 v3.9.4
	k8s.io/api v0.27.1
	k8s.io/apiextensions-apiserver v0.26.10
	k8s.io/apimachinery v0.27.1
	k8s.io/apiserver v0.27.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/gengo v0.0.0-20220902162205-c0856e24416d
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f
	k8s.io/kubectl v0.25.0
	k8s.io/kubelet v0.26.10
	k8s.io/utils v0.0.0-20230505201702-9f6742963106
	kubevirt.io/api v1.0.0
	kubevirt.io/containerized-data-importer-api v1.57.0-alpha1
	kubevirt.io/kubevirt v1.0.0
	sigs.k8s.io/cluster-api v1.4.8
	sigs.k8s.io/controller-runtime v0.14.7
	sigs.k8s.io/kind v0.14.0
	sigs.k8s.io/kustomize/kyaml v0.13.9
	sigs.k8s.io/yaml v1.3.0
)

require (
	emperror.dev/errors v0.8.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/sprig/v3 v3.2.3 // indirect
	github.com/Masterminds/squirrel v1.5.3 // indirect
	github.com/RoaringBitmap/roaring v1.2.3 // indirect
	github.com/adrg/xdg v0.3.1 // indirect
	github.com/alessio/shellescape v1.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20200907205600-7a23bdc65eef // indirect
	github.com/aws/aws-sdk-go v1.44.116 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.7.0 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/containerd/containerd v1.6.6 // indirect
	github.com/coreos/prometheus-operator v0.38.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/cli v20.10.17+incompatible // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker v20.10.18+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gammazero/deque v0.2.1 // indirect
	github.com/gammazero/workerpool v1.1.3 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gin-gonic/gin v1.7.0 // indirect
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-gorp/gorp/v3 v3.0.2 // indirect
	github.com/go-kit/kit v0.10.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.1 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.1.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gomodule/redigo v2.0.0+incompatible // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/fscrypt v0.3.4 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/go-containerregistry v0.7.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.7.0 // indirect
	github.com/huandu/xstrings v1.3.3 // indirect
	github.com/iancoleman/orderedmap v0.2.0 // indirect
	github.com/imdario/mergo v0.3.15 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jinzhu/copier v0.3.5 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jmoiron/sqlx v1.3.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.10.6 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/longhorn/backing-image-manager v1.4.0-rc1.0.20230521151917-38ff27cc2cbb // indirect
	github.com/longhorn/go-iscsi-helper v0.0.0-20231113050545-9df1e6b605c7 // indirect
	github.com/longhorn/go-spdk-helper v0.0.0-20231002161457-6c31a95f76e8 // indirect
	github.com/longhorn/longhorn-engine v1.5.2-0.20230906182804-9475221d9535 // indirect
	github.com/longhorn/longhorn-instance-manager v1.5.2-0.20231006022904-b2c88ac8080d // indirect
	github.com/longhorn/longhorn-share-manager v1.5.3-rc1.0.20231115141215-5109d5939e17 // indirect
	github.com/longhorn/longhorn-spdk-engine v0.0.0-20231005170812-e9b634e07e47 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/moby/term v0.0.0-20220808134915-39b0c02b01ae // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20220114050600-8b9d41f48198 // indirect
	github.com/opencontainers/selinux v1.10.2 // indirect
	github.com/openshift/client-go v0.0.0 // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.15.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/rancher/aks-operator v1.0.7 // indirect
	github.com/rancher/eks-operator v1.1.5 // indirect
	github.com/rancher/gke-operator v1.1.4 // indirect
	github.com/rancher/kubernetes-provider-detector v0.1.5 // indirect
	github.com/rancher/remotedialer v0.2.6-0.20220624190122-ea57207bf2b8 // indirect
	github.com/rancher/rke v1.3.18 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/rubenv/sql-migrate v1.1.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.2.0 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/slok/goresilience v0.2.0 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190809123943-df4f5c81cb3b // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.35.1 // indirect
	go.opentelemetry.io/otel v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.10.0 // indirect
	go.opentelemetry.io/otel/metric v0.31.0 // indirect
	go.opentelemetry.io/otel/sdk v1.10.0 // indirect
	go.opentelemetry.io/otel/trace v1.10.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	golang.org/x/mod v0.13.0 // indirect
	golang.org/x/oauth2 v0.13.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/term v0.13.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.14.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto v0.0.0-20230525234025-438c736192d0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230525234020-1aefcd67740a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/grpc v1.55.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/cli-runtime v0.27.1 // indirect
	k8s.io/code-generator v0.26.10 // indirect
	k8s.io/component-base v0.27.1 // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.100.1 // indirect
	k8s.io/kube-aggregator v0.26.3 // indirect
	k8s.io/kubernetes v1.27.1 // indirect
	k8s.io/mount-utils v0.27.1 // indirect
	kubevirt.io/client-go v1.0.0 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	oras.land/oras-go v1.2.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.1.1 // indirect
	sigs.k8s.io/cli-utils v0.27.0 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/kustomize/api v0.12.1 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)
