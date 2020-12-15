# App Mode Installation

## Install as an App(for development)
Harvester can be installed on a Kubernetes cluster in the following ways:	
- [Helm](https://helm.sh/)	
- Rancher catalog app
    - Add the [rancher/harvester](https://github.com/rancher/harvester) repo to the Rancher Catalog as a Helm `v3` App	
    
Please refer to the harvester [helm chart](../deploy/charts/harvester) for more details.
    
### Requirements
Kubernetes node must have hardware virtualization support, e.g. user can validate using `cat /proc/cpuinfo | grep vmx`.

### Option 1: Install with Helm
1. `$ git clone https://github.com/rancher/harvester.git --depth=1`
1. `$ cd deploy/charts`
1. installing the Harvester chart with the following commands:

```bash
### To install the chart with the release name `harvester`.

## create target namespace
$ kubectl create ns harvester-system

## install chart to target namespace
$ helm install harvester harvester --namespace harvester-system --set longhorn.enabled=true,minio.persistence.storageClass=longhorn
```
    
### Option 2: Install with Rancher Catalog App
Notes: You can create a testing Kubernetes environment in the Rancher using Digitial-Ocean provider, prefer to use the `8C, 16G RAM` node which will have the nested virtualization enabled by default.
![do.png](./assets/do.png)

1. Adding the harvester repo `https://github.com/rancher/harvester` to your Rancher catalogs via `Global -> Tools -> Catalogs`
1. Specify the URL and name, the default branch is master, and set the `Helm version` to be `Helm v3`.
![harvester-catalog.png](./assets/harvester-catalog.png)
1. Click create and navigate to your project-level `Apps`
1. Click `Launch` and choose the Harvester app
1. (optional) You can modify the configurations if needed, otherwise, leave it to the default options
1. Click `Launch` and wait for the app's components to be ready
1. Click the `/index.html` link to navigate to the Harvester UI
![harvester-app.png](./assets/harvester-app.png)

Note: you may check this [video](https://vimeo.com/491085524) to reference the App mode installation.
