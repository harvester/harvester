module github.com/cnrancher/rancher-vm

go 1.13

replace (
	github.com/crewjam/saml => github.com/rancher/saml v0.0.0-20180713225824-ce1532152fde
	github.com/rancher/apiserver => github.com/rancheredge/apiserver v0.0.0-20200708064345-bd2aa8f9b7d1
	github.com/rancher/steve => github.com/rancheredge/steve v0.0.0-20200708031911-f69e0f4820b4
	k8s.io/client-go => k8s.io/client-go v0.18.0
	kubevirt.io/client-go => github.com/orangedeng/client-go v0.31.1-0.20200714145902-55fe7aa2f876
)

require (
	github.com/cnrancher/octopus-api-server v0.0.0-20200713114219-fdf69e9764f5 // indirect
	github.com/rancher/lasso v0.0.0-20200515155337-a34e1e26ad91
	github.com/rancher/wrangler v0.6.2-0.20200515155908-1923f3f8ec3f
	github.com/sirupsen/logrus v1.4.2
	github.com/urfave/cli v1.22.2
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v12.0.0+incompatible
	kubevirt.io/client-go v0.31.1-0.20200714145902-55fe7aa2f876
)
