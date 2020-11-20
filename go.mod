module github.com/rancher/harvester

go 1.13

require (
	github.com/ghodss/yaml v1.0.0
	github.com/jroimartin/gocui v0.4.0
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/nsf/termbox-go v0.0.0-20200418040025-38ba6e5628f1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/rancher/k3os v0.19.2-dev.4
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/tredoe/osutil v1.0.5
	k8s.io/apimachinery v0.0.0
)

replace (
	github.com/nsf/termbox-go => github.com/gitlawr/termbox-go v0.0.0-20201103025537-250e644d56a6
	k8s.io/api => github.com/rancher/kubernetes/staging/src/k8s.io/api v1.19.3-k3s1
	k8s.io/apiextensions-apiserver => github.com/rancher/kubernetes/staging/src/k8s.io/apiextensions-apiserver v1.19.3-k3s1
	k8s.io/apimachinery => github.com/rancher/kubernetes/staging/src/k8s.io/apimachinery v1.19.3-k3s1
	k8s.io/apiserver => github.com/rancher/kubernetes/staging/src/k8s.io/apiserver v1.19.3-k3s1
	k8s.io/client-go => github.com/rancher/kubernetes/staging/src/k8s.io/client-go v1.19.3-k3s1
	k8s.io/code-generator => github.com/rancher/kubernetes/staging/src/k8s.io/code-generator v1.19.3-k3s1
	k8s.io/component-base => github.com/rancher/kubernetes/staging/src/k8s.io/component-base v1.19.3-k3s1
	k8s.io/kube-aggregator => github.com/rancher/kubernetes/staging/src/k8s.io/kube-aggregator v1.19.3-k3s1
	k8s.io/metrics => github.com/rancher/kubernetes/staging/src/k8s.io/metrics v1.19.3-k3s1
)
