module github.com/cnrancher/rancher-vm

go 1.13

replace (
	github.com/crewjam/saml => github.com/rancher/saml v0.0.0-20180713225824-ce1532152fde
	github.com/rancher/apiserver => github.com/rancheredge/apiserver v0.0.0-20200708064345-bd2aa8f9b7d1
	github.com/rancher/steve => github.com/rancheredge/steve v0.0.0-20200708031911-f69e0f4820b4
	k8s.io/client-go => k8s.io/client-go v0.18.0
	kubevirt.io/client-go => github.com/orangedeng/client-go v0.31.1-0.20200715023159-4acafc7ad243
)

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20191027212112-611e8accdfc9 // indirect
	github.com/google/go-cmp v0.4.0 // indirect
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/rancher/lasso v0.0.0-20200515155337-a34e1e26ad91
	github.com/rancher/wrangler v0.6.2-0.20200515155908-1923f3f8ec3f
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.5.1 // indirect
	github.com/urfave/cli v1.22.2
	golang.org/x/crypto v0.0.0-20200414173820-0848c9571904 // indirect
	golang.org/x/sys v0.0.0-20200122134326-e047566fdf82 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	golang.org/x/tools v0.0.0-20191115202509-3a792d9c32b2 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v12.0.0+incompatible
	kubevirt.io/client-go v0.31.1-0.20200715023159-4acafc7ad243
)
