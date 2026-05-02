# Rancher Integration RBAC - Automated Test Plan

This document outlines the automated test plan for the Rancher Integration RBAC enhancement. The focus is to programmatically verify RBAC authorization at the cluster-scoped and namespace-scoped API level, without relying on the UI. This approach allows for more comprehensive and repeatable testing of the RBAC permissions defined in the enhancement.

The test plan assumes that Harvester is already imported into an external Rancher instance.

Follow the instructions at <https://github.com/harvester/charts/pull/475> to install the `harvester-rbac` Helm chart on Rancher.

## High Level Test Design

In general, the test plan should include the following steps:

* Assign users with each of the newly defined RBAC roles.
* Generate API tokens for these users.
* Use these tokens to execute API requests against Harvester APIs to verify that the permissions granted by each role are correctly enforced.
* Ensure that unauthorized actions are appropriately denied and return the correct error responses.

The easiest way to set up the test environment is to preseed the Rancher instance with test users and their corresponding RBAC roles from the UI. Then retrieve their kubeconfig files from the UI to automate the API execution against the Harvester cluster.

To programmatically create the test users and generate their kubeconfig files, see the next section.

## Example Test Scenario Using The Users and Kubeconfig API

In this example test scenario, we create a new Rancher user named `testuser` on an instance of Rancher v2.13.2. This user is assigned the `View Virtualization Resources` project role in the `demo` project in a Harvester cluster named `hv-lab`. A kubeconfig file is generated for `testuser` to access the Harvester cluster and verify the permissions granted by the role assigned to `testuser`.

This test setup uses the [Rancher CLI][6] to simplify the [authentication process][7] to generate the API token needed for the kubeconfig file.

Obtain the Rancher CA certificate from the Rancher "Global Settings" UI and save it as `.cacert` in the current directory.

Login via the Rancher CLI:

```sh
./rancher login --cacert ./.cacert --token <bearer-token-from-rancher-ui> https://<rancher-hostname>
```

Create a `demo` project in the Harvester cluster if it does not exist.

Export some common variables in the current shell:

```sh
export USER_ID="testuser"
export USER_PASSWORD=$(openssl rand -base64 14)
export ROLE_TEMPLATE_NAME="virt-project-view"
export HARVESTER_CLUSTER_NAME="hv-lab"
export HARVESTER_PROJECT_NAME="demo"
export RANCHER_SERVER_URL="https://<rancher-hostname>"
export CLUSTER_ID=$(rancher clusters ls | grep ${HARVESTER_CLUSTER_NAME} | tr -s ' ' | cut -d' ' -f2)
export PROJECT_ID=$(rancher projects ls | grep ${HARVESTER_PROJECT_NAME} | tr -s ' ' | cut -d' ' -f1 | cut -d ":" -f2)
```

🔒 The `USER_PASSWORD`  can be overridden to suit your testing environment. It is required later to generate the API token for `testuser`.

Retrieve the Rancher's `cluster-admin` kubeconfig file:

```sh
rancher clusters kf local > rancher.kubeconfig
```

On Rancher, create the new user `testuser`:

```sh
kubectl --kubeconfig=rancher.kubeconfig create -f -<<EOF
apiVersion: management.cattle.io/v3
kind: User
metadata:
  name: "${USER_ID}"
displayName: test user
username: "${USER_ID}"
EOF
```

Create a password for `testuser` in the `cattle-local-user-passwords` namespace:

```sh
kubectl --kubeconfig=rancher.kubeconfig create -f -<<EOF
apiVersion: v1
kind: Secret
metadata:
  name: testuser
  namespace: cattle-local-user-passwords
type: Opaque
stringData:
  password: ${USER_PASSWORD}
EOF
```

Assign the `Standard User` global role and the `View Virtualization Resources` project role to the user:

```sh
kubectl --kubeconfig=rancher.kubeconfig create -f -<<EOF
apiVersion: management.cattle.io/v3
globalRoleName: user
kind: GlobalRoleBinding
metadata:
  generateName: grb-
userName: ${USER_ID}
userPrincipalName: ${USER_ID}
---
apiVersion: management.cattle.io/v3
kind: ProjectRoleTemplateBinding
metadata:
  generateName: prtb-
  namespace: ${CLUSTER_ID}-${PROJECT_ID}
projectName: ${CLUSTER_ID}:${PROJECT_ID}
roleTemplateName: ${ROLE_TEMPLATE_NAME}
userName: ${USER_ID}
userPrincipalName: local://${USER_ID}
EOF
```

Retrieve an API token for `testuser`:

🔒 By default, the Rancher CLI uses the `local` auth provider to perform the authentication in this next step. When prompted, provide the `${USER_ID}` and `${USER_PASSWORD}` variables for authentication. It is possible to configure the Rancher CLI to use the OAuth auth provider, depending on the Rancher instance setup.

```sh
API_TOKEN=$(rancher token --server "${RANCHER_SERVER_URL}" --user "${USER_ID}" --cluster "${CLUSTER_ID}" --skip-verify | jq -r .status.token)
```

Generate a kubeconfig file for `testuser` to access the Harvester cluster:

```sh
curl -sk -XPOST -H"Authorization: Bearer ${API_TOKEN}" "${RANCHER_SERVER_URL}/v3/clusters/${CLUSTER_ID}?action=generateKubeconfig" | jq -r .config | yq . > testuser.kubeconfig
```

🤷 If the `testuser.kubeconfig` file is empty, then the API token isn't generated correctly. Change the `--user` option to an arbirtrary user (e.g., `testuser1`) to trigger a relogin. This usually fixes the issue and generates the API token correctly.

Use the `testuser.kubeconfig` file to execute API requests against the Harvester cluster to verify the permissions granted by the assigned role:

```sh
$ kubectl --kubeconfig=testuser.kubeconfig -n demo-blue get vm,vmi,po
NAME                               AGE   STATUS    READY
virtualmachine.kubevirt.io/vm-00   10d   Running   True
virtualmachine.kubevirt.io/vm-01   11d   Running   True

NAME                                       AGE   PHASE     IP            NODENAME           READY
virtualmachineinstance.kubevirt.io/vm-00   44h   Running   10.52.0.116   hp-4-tink-system   True
virtualmachineinstance.kubevirt.io/vm-01   10d   Running   10.52.0.82    hp-4-tink-system   True

NAME                            READY   STATUS    RESTARTS   AGE
pod/virt-launcher-vm-00-f5l4w   2/2     Running   0          44h
pod/virt-launcher-vm-01-25tjg   2/2     Running   0          10d

$ kubectl --kubeconfig=testuser.kubeconfig -n isim-dev-blue get vm,vmi,po
Error from server (Forbidden): virtualmachines.kubevirt.io is forbidden: User "testuser" cannot list resource "virtualmachines" in API group "kubevirt.io" in the namespace "isim-dev-blue"
Error from server (Forbidden): virtualmachineinstances.kubevirt.io is forbidden: User "testuser" cannot list resource "virtualmachineinstances" in API group "kubevirt.io" in the namespace "isim-dev-blue"
Error from server (Forbidden): pods is forbidden: User "testuser" cannot list resource "pods" in API group "" in the namespace "isim-dev-blue"
```
