# Provide Identity to Running VMs via Kubernetes Service Accounts

## Summary

Harvester users often need to run workloads inside a virtual machine (VM) that must interact with the underlying Kubernetes cluster (for example, to query the Kubernetes API for pods, services, ConfigMaps, or Secrets). Previously, there was no first-class way to hand a Kubernetes identity to a running VM through the UI.

This enhancement lets users mount a Kubernetes ServiceAccount into a VM as a filesystem using virtio-fs. The ServiceAccount's token, CA certificate, and namespace are projected into a directory inside the guest, so applications running in the VM can authenticate against the Kubernetes API using the mounted credentials. This reuses the "[filesystem disk for VM](https://github.com/harvester/harvester/issues/9762)" capability that also supports mounting Secrets and ConfigMaps.

Because virtio-fs projects the ServiceAccount as a live filesystem, credential rotation and RBAC permission changes are reflected inside the VM without requiring a VM restart.

### Related Issues

- Feature: https://github.com/harvester/harvester/issues/10426
- Parent (filesystem disk for VM): https://github.com/harvester/harvester/issues/9762
- Backend PR (virtio-fs for Secret, ConfigMap and ServiceAccount): https://github.com/harvester/harvester/pull/9981
- UI issue: https://github.com/harvester/harvester/issues/9896

## Motivation

### Goals

- Allow a user to attach a Kubernetes ServiceAccount to a VM as a filesystem through the Harvester UI.
- Project the ServiceAccount token, CA certificate, and namespace into the VM so guest applications can authenticate to the Kubernetes API.
- Reflect ServiceAccount credential rotation changes inside the VM without a VM restart.

### Non-goals [optional]

- Providing identity via the KubeVirt "disk" (block/ISO) ServiceAccount projection method. Only the filesystem (virtio-fs) method is in scope.
- Managing the lifecycle of ServiceAccounts, Roles, or RoleBindings. Users are responsible for creating the ServiceAccount and granting the appropriate RBAC permissions.


## Proposal

### User Stories

#### Story 1 - Provide a cluster identity to an in-VM application

Before this enhancement, a user who wanted a VM workload to talk to the Kubernetes API had to manually copy tokens and CA certificates into the guest, or use the KubeVirt disk-based ServiceAccount method, which is not exposed in the Harvester UI. This is error-prone and does not handle credential rotation.

After this enhancement, the user creates a ServiceAccount (with suitable RBAC) and attaches it to the VM as a filesystem in the Harvester UI. The VM sees the ServiceAccount token, CA certificate, and namespace under a mount point and can immediately authenticate against the Kubernetes API.

#### Story 2 - Rotate credentials / update permissions without restart

Before this enhancement, changing a VM workload's cluster permissions or rotating its token typically required rebuilding or restarting the VM.

After this enhancement, because the ServiceAccount is mounted via virtio-fs, updating the ServiceAccount's RBAC or rotating its token is reflected inside the running VM without a restart.

### User Experience In Detail

Prerequisites:

- The KubeVirt feature gate `EnableVirtioFsConfigVolumes` must be enabled in the cluster.
- The user must have a ServiceAccount in the same namespace as the VM, with the desired RBAC (Role/RoleBinding) granting the permissions the in-VM workload needs.

Steps:

1. In the Harvester UI, create or edit a VM.
2. Go to the filesystem tab and choose a ServiceAccount filesystem mount, selecting the target ServiceAccount and paste the mount path in cloud init user data in Advanced tab. 
3. Start the VM.
4. Harvester attaches the ServiceAccount as a virtio-fs filesystem to the VM.After the guest OS mounts it, the directory contains `token`, `ca.crt`, and `namespace`.
5. Applications in the VM authenticate to the Kubernetes API using these files, for example:

   ```bash
   TOKEN="$(sudo cat /mnt/serviceaccount/token)"
   sudo curl --cacert /mnt/serviceaccount/ca.crt \
     -H "Authorization: Bearer ${TOKEN}" \
     https://kubernetes.default.svc/api
   ```

6. Updating the ServiceAccount RBAC or rotating its token is reflected inside the running VM without a restart.


Note. Mounting service account inside the guest OS (paste mount path such as `/mnt/serviceaccount` in user data) is the VM user's responsibility and is outside Harvester's visibility.

### API changes

No new Harvester CRD or API. The VM (`VirtualMachine`) spec is populated with:

- A `filesystems` entry using `virtiofs: {}` on the VM's `spec.template.spec.domain.devices`.
- A corresponding `spec.template.spec.volumes` entry of type `serviceAccount` referencing the target ServiceAccount by name.

Example VM spec fragment:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: vm-sa
  namespace: sa-virtiofs-test
spec:
  template:
    spec:
      domain:
        devices:
          filesystems:
            - name: appserviceaccountfs
              virtiofs: {}
      volumes:
        - name: appserviceaccountfs
          serviceAccount:
            serviceAccountName: my-serviceaccount
```

## Design

### Implementation Overview

Backend (already merged in PR #9981):

- Support virtio-fs projection for Secret, ConfigMap, and ServiceAccount volume sources in Harvester VMs.
- The backend only requires the KubeVirt feature gate `EnableVirtioFsConfigVolumes` to be enabled; no additional Harvester controller changes are required for the projection itself.

Frontend (UI issue #9896):

- The VM edit/create UI exposes attaching a ServiceAccount (as well as Secret and ConfigMap) as a filesystem mount.
- The UI generates the `devices.filesystems[*].virtiofs` entry and the matching `volumes[*].serviceAccount.serviceAccountName` in the VM spec.


### Test plan

Preparation:

1. Deploy v1.9.0 Harvester.
2. Confirm the KubeVirt feature gate `EnableVirtioFsConfigVolumes` is enabled.

ServiceAccount test (based on PR #9981 "Virtiofs with serviceaccount"):

1. Create a namespace, a ServiceAccount (e.g. `vm-sa`), and a Role/RoleBinding granting read access to core resources (pods, services, configmaps, secrets) in that namespace.
2. Create a VM and select a ServiceAccount as a filesystem volume (mount path, e.g `/mnt/appserviceaccountfs`). 

3. Paste the below mount path in VM Advanced tab user data  
   ```
   #cloud-config
   runcmd:
     - mkdir -p /mnt/appserviceaccountfs
     - mount -t virtiofs appserviceaccountfs /mnt/appserviceaccountfs
   ```

4. Start the VM and open its console.
5. Verify the virtio-fs mount and its content inside the VM:

   ```bash
   sudo mount | grep virtiofs
   sudo ls -al /mnt/serviceaccount
   sudo cat /mnt/serviceaccount/namespace
   sudo cat /mnt/serviceaccount/token
   ```

6. Verify Kubernetes API access from inside the VM:

   ```bash
   TOKEN="$(sudo cat /mnt/serviceaccount/token)"
   sudo curl --cacert /mnt/serviceaccount/ca.crt \
     -H "Authorization: Bearer ${TOKEN}" \
     https://kubernetes.default.svc/api

   sudo curl --cacert /mnt/serviceaccount/ca.crt \
     -H "Authorization: Bearer ${TOKEN}" \
     https://kubernetes.default.svc/api/v1/namespaces/<namespace>/pods
   ```

7. Update the ServiceAccount's RBAC (e.g. extend to `deployments`, `statefulsets` in `apps`) and verify the new permissions take effect inside the running VM without a restart:

   ```bash
   TOKEN="$(sudo cat /mnt/serviceaccount/token)"
   sudo curl --cacert /mnt/serviceaccount/ca.crt \
     -H "Authorization: Bearer ${TOKEN}" \
     https://kubernetes.default.svc/apis/apps/v1/namespaces/<namespace>/deployments
   ```

The same flow applies to Secret and ConfigMap filesystem mounts, verifying that updated data content is visible inside the VM without a restart.

### Upgrade strategy

No special upgrade action is required. The feature depends on the KubeVirt feature gate `EnableVirtioFsConfigVolumes` being enabled which introduced in harvester v1.9.0 release.

## Note [optional]

- The ServiceAccount, Role, and RoleBinding must be created and managed by the user;
- Harvester only projects the ServiceAccount into the VM.
- Limitation: VMs with filesystem mounts do not support live migration.
- If the ServiceAccount has `kubernetes.io/enforce-mountable-secrets: "true"`, ensure any Secret volumes the VM references (for example the cloud-init Secret) are listed in the ServiceAccount's `secrets`, otherwise the VM launcher pod may be rejected.
