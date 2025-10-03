# Harvester Upgrade Enhancement

## Summary

This enhancement aims to improve the Harvester Upgrade experience by deprecating the Upgrade Repository virtual machine and replacing it with a set of Upgrade Repository pods running on each node behind a unified endpoint. The default image-preloading behavior will be changed to a local-first manner, and all nodes can do the preload independently and simultaneously. Also, a stand-alone Harvester Upgrade Manager will be introduced to manage the (almost) entire upgrade process.

The Harvester Upgrade enhancement, denoted as *Upgrade V2* hereafter, introduced here will involve the following main components:

- Upgrade Shim: A small portion of code embedded in the Harvester main program, i.e., the controller manager, that is responsible for bootstrapping the upgrade environment
- Upgrade Repository: A set of pods running on each node behind a unified endpoint to replace the original Upgrade Repository functionalities
- Upgrade Manager: A dedicated controller manager released separately that is responsible for the Harvester Upgrade mechanics

### Related Issues

- https://github.com/harvester/harvester/issues/7101
- https://github.com/harvester/harvester/issues/7112

## Motivation

**Clunky Upgrade Repository Virtual Machine**

Every Harvester Upgrade instance runs an Upgrade Repository virtual machine (hereafter referred to as *the upgrade-repo VM*) for different upgrade-relevant components to query data. This VM is necessary because Harvester is released as ISO images. Booting up a virtual machine from the ISO image is the most straightforward way. So far, no known way to spin up pods that mount an ISO image directly exists.

However, the VM-based approach has several drawbacks:

- Virtual machines typically have a larger footprint than pods regarding startup time and resource usage.
  - Populating a volume (and its replicas) from the backing image is a time-consuming process
  - Starting up a virtual machine means going through a complete OS boot-up process, which is more than necessary for the upgrade-repo use case
  - The already-precious resources are consumed by the OS stack of the upgrade-repo VM
- Hard to scale and ensure resiliency during the upgrade. If the only upgrade-repo VM is down, the upgrade process will be blocked.
- Though virtual machines can be live-migrated, they still have limitations (for instance, https://github.com/harvester/harvester/issues/7096). Besides, under the upgrade-repo context, it's an overkill.

The Upgrade Repository, in its essence, is just an HTTP server serving files. It can be simplified to make the entire workflow streamlined.

**Self-upgrade Paradox of the Embedded Upgrade Controller Manager**

The current embedded upgrade controller manager has a bootstrap problem. Let's say the user initiates the upgrade from v1.5.2 to v1.6.0:

- Early upgrade phases, e.g., the image-preload phase, execute with the current version (v1.5.2)
- Controller manager binary gets replaced (upgraded to v1.6.0) at the apply-manifests phase
- Later phases run with the new controller version (v1.6.0)

As a result, bugs in early-phase logic require a two-release fix cycle. With the example above, the error-prone behaviors can only be rectified when upgrading from v1.6.0.

**Cumbersome Maintenance Burden Imposed by Rancher-managed Self-import RKE2 Cluster**

Harvester leverages Rancher Provisioning V2 framework (v2prov) to manage the local RKE2 cluster. The framework requires the Rancher System Agent to be installed and running on each node. Instructions are then distributed to the system agents through machine plan Secrets. Rancher utilizes both the annotations and data sections of the machine plan Secret to facilitate cluster node life-cycle management, including upgrades, in a cloud-native way. Specifically, the Harvester Upgrade relies on the hook mechanism exposed by the v2prov functionality to realize pre- and post-draining tasks, which include evacuating user workloads and upgrading the node operating system.

The fact is that we usually find Harvester Upgrade broken due to subtle behavior changes introduced in newer Rancher versions. As a result, we put a lot of effort into maintenance, including testing and patching, to ensure Harvester Upgrade works as expected. Moreover, it adds difficulty when it comes to cluster salvation. In summary, we aim to alleviate the maintenance and operational burdens by decoupling our reliance on Rancher v2prov.

### Goals

- Improve upgrade speed and reliability
- Maintain zero-downtime for user workloads
- Minimize breaking changes to existing user experience
- Reduce maintenance overhead

### Non-goals [optional]

- Support rollback action if upgrade fails

## Proposal

To achieve these goals, we will:

- Deprecate the VM-based Upgrade Repository and replace it with a pod-based solution
- Resolve the two-release fix cycle paradox by decoupling the upgrade controller from Harvester releases
- Remove dependency on Rancher v2prov and migrate to System Upgrade Controller (SUC)

### User Stories

Since there are no significant changes from the user's perspective, the user stories for the Harvester Upgrade remain essentially the same. Additionally, the combinations are limited because the Harvester Upgrade is designed to be initiated with a single click. The only variation is in the formation of the cluster being upgraded.

#### Story 1

As a cluster administrator, I want to upgrade my Harvester cluster to the latest version, so that I can access new features, bug fixes, and security patches.

### User Experience in Detail

The upgrade workflow remains consistent with previous versions, with steps outlined below:

1. Administrator checks current cluster version and available upgrades
   - CLI: `kubectl get settings.harvesterhci.io server-version`
   - UI: Navigate to the cluster dashboard page and look for the "Version" string
1. Administrator reviews upgrade prerequisites and release notes
   - Check support matrix
   - Review breaking changes and known issues
   - Verify cluster status with the pre-flight check script
1. Administrator initiates upgrade process
   - CLI: `kubectl create -f upgradeplan.yaml`
   - UI: Click "Upgrade" button, select target version
1. Administrator monitors upgrade progress
   - Real-time status in the Upgrade modal on the UI dashboard
   - CLI: `kubectl get upgradeplans.management.harvesterhci.io <upgradeplan-name> -w`
1. Administrator validates post-upgrade functionality
   - Verify all VMs are running
   - Check storage and networking
   - Confirm new features are available and bug fixes are applied
1. Administrator handles any issues
   - Access to upgrade logs
   - Start another round of upgrade after the cluster becomes stable
   - Support contact information

### API Changes

Introducing new APIs: **UpgradePlan** and **Version**. This new UpgradePlan CRD (Custom Resource Definition) is designed to work closely with the Version CRD. The Version CRD is almost identical to the previous version, as the surrounding functions do not change significantly. The existing Upgrade Responder mechanic can be preserved, albeit with a slight change to the CRD API group. Both CRDs will be under the `management.harvesterhci.io` API group and be cluster-scoped. Being cluster-scoped not only fits the semantics as both APIs are relevant to upgrading the entire cluster, but not just for any specific component, but also provides a better mechanism when it comes to resource purge. In Upgrade V1, both CRDs are namespaced-scope, which results in the inability to cascade delete the owned resources that reside in namespaces other than `harvester-system`. We had no choice but to write dedicated code to actively wipe them out when deletion happens to Upgrade CRs.

The UpgradePlan CRD looks like the following:

```go
const (
	// Available means the UpgradePlan is ready to be reconciled.
	UpgradePlanAvailable string = "Available"
	// Progressing means the cluster is currently applying the UpgradePlan.
	UpgradePlanProgressing string = "Progressing"
	// Degraded means the progress of the upgrade is stalled due to issues.
	UpgradePlanDegraded string = "Degraded"
)

const (
	NodeStateImagePreloading     string = "ImagePreloading"
	NodeStateImagePreloaded      string = "ImagePreloaded"
	NodeStateKubernetesUpgrading string = "KubernetesUpgrading"
	NodeStateKubernetesUpgraded  string = "KubernetesUpgraded"
	NodeStateOSUpgrading         string = "OSUpgrading"
	NodeStateOSUpgraded          string = "OSUpgraded"
)

const (
	UpgradePlanPhaseInitializing       UpgradePlanPhase = "Initializing"
	UpgradePlanPhaseInitialized        UpgradePlanPhase = "Initialized"
	UpgradePlanPhaseISODownloading     UpgradePlanPhase = "ISODownloading"
	UpgradePlanPhaseISODownloaded      UpgradePlanPhase = "ISODownloaded"
	UpgradePlanPhaseRepoCreating       UpgradePlanPhase = "RepoCreating"
	UpgradePlanPhaseRepoCreated        UpgradePlanPhase = "RepoCreated"
	UpgradePlanPhaseMetadataPopulating UpgradePlanPhase = "MetadataPopulating"
	UpgradePlanPhaseMetadataPopulated  UpgradePlanPhase = "MetadataPopulated"
	UpgradePlanPhaseImagePreloading    UpgradePlanPhase = "ImagePreloading"
	UpgradePlanPhaseImagePreloaded     UpgradePlanPhase = "ImagePreloaded"
	UpgradePlanPhaseClusterUpgrading   UpgradePlanPhase = "ClusterUpgrading"
	UpgradePlanPhaseClusterUpgraded    UpgradePlanPhase = "ClusterUpgraded"
	UpgradePlanPhaseNodeUpgrading      UpgradePlanPhase = "NodeUpgrading"
	UpgradePlanPhaseNodeUpgraded       UpgradePlanPhase = "NodeUpgraded"
	UpgradePlanPhaseCleaningUp         UpgradePlanPhase = "CleaningUp"
	UpgradePlanPhaseCleanedUp          UpgradePlanPhase = "CleanedUp"

	UpgradePlanPhaseSucceeded UpgradePlanPhase = "Succeeded"
	UpgradePlanPhaseFailed    UpgradePlanPhase = "Failed"
)

type NodeUpgradeStatus struct {
	State   string `json:"state,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// UpgradePlanPhase defines what overall phase UpgradePlan is in
type UpgradePlanPhase string

type UpgradePlanPhaseTransitionTimestamp struct {
	Phase                    UpgradePlanPhase `json:"phase"`
	PhaseTransitionTimestamp metav1.Time      `json:"phaseTransitionTimestamp"`
}

type ReleaseMetadata struct {
	Harvester            string `json:"harvester,omitempty"`
	HarvesterChart       string `json:"harvesterChart,omitempty"`
	OS                   string `json:"os,omitempty"`
	Kubernetes           string `json:"kubernetes,omitempty"`
	Rancher              string `json:"rancher,omitempty"`
	MonitoringChart      string `json:"monitoringChart,omitempty"`
	MinUpgradableVersion string `json:"minUpgradableVersion,omitempty"`
}

// UpgradePlanSpec defines the desired state of UpgradePlan
type UpgradePlanSpec struct {
	// version refers to the corresponding version resource in the same namespace.
	// +required
	Version string `json:"version"`

	// upgrade can be specified to opt for any other specific upgrade image. If not provided, the version resource name is used.
	// For instance, specifying "dev" for the field can go for the "rancher/harvester-upgrade:dev" image.
	// +optional
	Upgrade *string `json:"upgrade,omitempty"`

	// force indicates the UpgradePlan will be forcibly applied, ignoring any pre-upgrade check failures. Default to "false".
	// +optional
	Force *bool `json:"force,omitempty"`

	// mode represents the manipulative style of the UpgradePlan. Can be either of "automatic" or "interactive". Default to "automatic".
	// +optional
	Mode *string `json:"mode,omitempty"`
}

// UpgradePlanStatus defines the observed state of UpgradePlan.
type UpgradePlanStatus struct {
	// conditions represent the current state of the UpgradePlan resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// isoImageID refers to the namespaced name of the VM image that will be used for the upgrade.
	// +optional
	ISOImageID *string `json:"isoImageID,omitempty"`

	// nodeStatuses reflect each node's upgrade status for node specific tasks.
	// +mapType=atomic
	// +optional
	NodeUpgradeStatuses map[string]NodeUpgradeStatus `json:"nodeUpgradeStatuses,omitempty"`

	// phase shows what overall phase the UpgradePlan resource is in.
	Phase UpgradePlanPhase `json:"phase,omitempty"`

	// phaseTransitionTimestamp is the timestamp of when the last phase change occurred.
	// +listType=atomic
	// +optional
	PhaseTransitionTimestamps []UpgradePlanPhaseTransitionTimestamp `json:"phaseTransitionTimestamps,omitempty"`

	// previousVersion is the Harvester version before upgrade.
	// +optional
	PreviousVersion *string `json:"previousVersion,omitempty"`

	// releaseMetadata reflects the essential metadata extracted from the artifact.
	// +optional
	ReleaseMetadata *ReleaseMetadata `json:"releaseMetadata,omitempty"`

	// version is the snapshot of the associated Version resource
	// +optional
	Version *VersionSpec `json:"version,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=up;ups
// +kubebuilder:printcolumn:name="VERSION",type="string",JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="AVAILABLE",type=string,JSONPath=`.status.conditions[?(@.type=='Available')].status`
// +kubebuilder:printcolumn:name="PROGRESSING",type=string,JSONPath=`.status.conditions[?(@.type=='Progressing')].status`
// +kubebuilder:printcolumn:name="DEGRADED",type=string,JSONPath=`.status.conditions[?(@.type=='Degraded')].status`

// UpgradePlan is the Schema for the upgradeplans API
type UpgradePlan struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of UpgradePlan
	// +required
	Spec UpgradePlanSpec `json:"spec"`

	// status defines the observed state of UpgradePlan
	// +optional
	Status UpgradePlanStatus `json:"status,omitempty,omitzero"`
}
```

The Version CRD looks like the following (preserving almost all the fields from previous `harvesterhci.io` Version CRD):

```go

// VersionSpec defines the desired state of Version
type VersionSpec struct {
	// +required
	ISODownloadURL string `json:"isoURL"`

	// +optional
	ISOChecksum *string `json:"isoChecksum,omitempty"`

	// +optional
	ReleaseDate *string `json:"releaseDate,omitempty"`

	// +optional
	Tags []string `json:"tags,omitempty"`
}

// VersionStatus defines the observed state of Version.
type VersionStatus struct {
	// conditions represent the current state of the Version resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="ISOURL",type="string",JSONPath=`.spec.isoURL`

// Version is the Schema for the versions API
type Version struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Version
	// +required
	Spec VersionSpec `json:"spec"`

	// status defines the observed state of Version
	// +optional
	Status VersionStatus `json:"status,omitempty,omitzero"`
}
```

## Design

The overall architecture of Harvester Upgrade V2 is as follows.

### Upgrade Shim

Upgrade Shim should be the least frequently changed and the most stable part of the entire Harvester Upgrade mechanics. This implies it should be as simple as possible, avoiding any sophisticated approaches. This part also lives with other controllers in the Harvester main program. Upgrade Shim is responsible for the following tasks:

1. Downloading the ISO image
1. (?) Checking the eligibility of the upgrade in terms of the to and from versions
1. Deploying Upgrade Repository
1. Bootstrapping Upgrade Manager
   1. (Optional) Preloading the Upgrade Manager container image
   1. Deploying Upgrade Manager

### Upgrade Repository

Upgrade Repository is the warehouse that is built on the shipped ISO image. It serves other upgrade-relevant components, not just artifacts, but also metadata. The proposed design is to replace the upgrade-repo VM with a set of upgrade-repo pods that are behind a consistent service endpoint, providing a highly available in-cluster upgrade-repo service. This can be further enhanced by increasing speed and saving traffic through the introduction of Spegel.

> [!NOTE]
> Spegel cannot replace the Upgrade Repository entirely because, in the end, container images and other artifacts have to somehow appear on one of the nodes to be distributed among others through Spegel.

#### Plan 1 - Static Pods with BackingImage Disk Files

The main idea is to leverage static pods, Longhorn BackingImage disk files, and hostPath volumes. The `minNumberOfCopies` field of the BackingImage for the ISO image is set to the number of nodes of the cluster (subject to change) to distribute the disk files to all nodes. By mounting the BackingImage disk file directly to a specific path on the host filesystem for each node, static pods on each node can access the ISO image content using the hostPath volume.

Plan 1 is efficient since it leverages Longhorn's BackingImage subsystem to distribute the ISO images in parallel. The initial scaffolding is rapid because the ISO image should be ready on each node by then. However, the drawbacks include interfering with the host filesystem, handling tedious teardown code, the intolerance of unpersistent mountpoints to host reboots, reliance on the System Upgrade Controller for static pod deployment, and high dependency on the Longhorn BackingImage subsystem's inner workings.

#### Plan 2 - Deployment with RWX Volume

Although ROX (ReadOnlyMany) access mode is more suitable here, it is currently unsupported by Longhorn. The alternative way is to use the RWX volume that is implemented via NFS under the hood. This plan aims to create an upgrade-repo Deployment with multiple replica pods that mount the same shared RWX volume backed by Longhorn. The leader pod's initial container (the downloader) downloads the ISO image from the BackingImage download endpoint. It places the downloaded image in the Longhorn shared RWX volume, accessible by other upgrade-repo pods. The sidecar containers (the mounters) then mount the ISO image in the shared volume to another bidirectional shared mount that is only shared across containers in the same pod. The main containers (the HTTP servers) serve the files from that shared path.

Plan 2 does not mess with the host filesystem and is more robust even under the chaos of an upgrading cluster. The drawback is unnecessary compression/decompression back and forth overhead, resulting in a longer preparation time compared to the previous VM-based implementation. Also, the share manager pod of the RWX volume becomes the SPOF (Single Point of Failure) of the entire upgrade-repo architecture.

### Upgrade Manager Preloader

This is another glue layer with the same position as the Upgrade Shim, but it does fewer things. It simply preloads the Upgrade Manager container image to each node, allowing Upgrade Manager to start without friction. Upgrade Shim is responsible for kick-starting the preloader when users initiate upgrades, i.e., create Upgrade CRs.

Technically, Upgrade Shim creates a Plan CR for System Upgrade Controller to create Jobs running on each node.

### Upgrade Manager

The promotion of Upgrade Manager is to eliminate the two-version fix cycle paradox. Even further, its duty is deliberately stretched to cover most of the Harvester Upgrade mechanics to serve the same purpose. Besides, upgrade-related logic is only needed during upgrades and is rarely used at other times, so it does not need to exist all the time. That's the decisive reason not to deploy Upgrade Manager as a permanent deployment.

The introduction of Upgrade Manager is the most crucial part of the Harvester Upgrade V2 enhancement. It not only separates the upgrade reconcile loops from the bulky Harvester controller manager, but also changes how node-upgrade used to work in Upgrade V1. It encompasses two stages to give us enough time to incorporate all the changes and make them take effect.

#### Stage 1 - Transition Period (version N-1 to N upgrades, where N is the debut minor version of Harvester Upgrade V2)

The inner workings of Upgrade Manager in this stage will be essentially the same as before; the significant difference is that it is built and packaged separately from the primary Harvester artifact. It will also have its own Deployment in contrast to the main Harvester controller manager Deployment. The main concerns will be:

- How Upgrade Manager is bootstrapped
- How Upgrade Manager is torn down

The decision is straightforward: since Upgrade Shim initiates Upgrade Manager, to ensure workflow symmetry, it should also be performed by Upgrade Shim for teardown.

> [!NOTE]
> The N-to-N upgrades, including patch version upgrades, are deemed stage 1. For instance, the following upgrade paths are the case:
>
> - v1.7.0 -> v1.7.0
> - v1.7.0 -> v1.7.1
> - v1.7.1 -> v1.7.1

#### Stage 2 - Complete Upgrade V2 (version N to N+1 upgrades, where N is the debut minor version of Harvester Upgrade V2)

Harvester Upgrade V2 is fully unlocked in this stage. The cluster-upgrade phase largely remains the same. In the node-upgrade phase, Rancher v2prov will be dropped, and Harvester Upgrade will no longer rely on the embedded Rancher and Rancher System Agent on each node. Instead, System Upgrade Controller becomes a crucial companion.

The highlights in the original node-upgrade phase are:

1. User workloads, i.e., VMs, evacuation, corresponds to the **pre-drain** stage
1. Node drain and RKE2 upgrade, corresponds to the **drain** stage
1. OS upgrade, corresponds to the **post-drain** stage

Since the hook mechanism in Rancher v2prov is no longer usable, Upgrade Manager relies on Plan CRs backed by System Upgrade Controller to execute node-upgrade tasks. One of the hidden advantages of this change is that it allows the operating system upgrade to be separated from the overall upgrade, thus enabling true zero-downtime (for nodes) upgrades. Another one is that it unblocks the way for users to have granular control over the node-upgrade order through node selectors. The new node-upgrade phase looks like the following:

- **RKE2-upgrade** stage
  1. User workloads, i.e., VMs, evacuation, achieved by `prepare`
  1. Node drain and RKE2 upgrade, achieved by `upgrade`
- **OS-upgrade** stage (skippable)
  1. User workloads, i.e., VMs, evacuation, achieved by `prepare`
  1. Node drain and OS upgrade, achieved by `upgrade`

The challenge of switching to SUC Plans is that the node-upgrade phase is no longer executed as a control loop, but as a one-time task. Upgrade Manager needs to reconcile Plans instead of machine-plan Secrets, and there will be no other entities, such as the embedded Rancher, which abstracts the lifecycle management of downstream clusters, to organize the upgrade nuances.

> [!IMPORTANT]
> The infinite-retry vs. fail-fast will be a key topic for discussion.

### Implementation Overview

Overview on how the enhancement will be implemented.

#### Upgrade Shim

Upgrade Shim reconciles the Version and Upgrade CRs.

Upgrade Shim does the following when a Version CR gets created:

1. Download the ISO image and creates the Upgrade Repository Deployment with its replicas set to zero

Upgrade Shim does the following when an Upgrade CR gets created (if the associated Version CR can be found):

1. Scale the Upgrade Repository Deployment to appropriate replicas
1. Run the Upgrade Manager preloader
1. Create the Upgrade Manager Deployment with its replicas set to zero; the container image used is sourced from the Upgrade CR (so it's possible to run an off-version Upgrade Manager)

#### Upgrade Repository

Upgrade Repository serves upgrade-related metadata and artifacts.

1. The image downloader downloads the ISO image served on the Longhorn BackingImage download endpoint to the mounted Longhorn RWX volume (only leader will do it)
1. The image mounters mount the ISO image from the RWX volume to the exact mount point where the other `emptyDir` volumes, whose mount propagation is set to bidirectional, are mounted.
1. The HTTP servers mount the same `emptyDir` volumes and serve files within the path

The following manifests demonstrate how Upgrade Repository pods can be created (not complete yet, use with caution):

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: hvst-upgrade-repo-image-store
  namespace: harvester-system
spec:
  accessModes:
  - ReadWriteMany
  volumeMode: Filesystem
  resources:
    requests:
      storage: 10Gi
  storageClassName: longhorn-static
---
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    management.cattle.io/scale-available: "3"
  labels:
    app.kubernetes.io/name: harvester-upgrade
    app.kubernetes.io/component: upgrade-repo
  name: hvst-upgrade-repo
  namespace: harvester-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: harvester-upgrade
      app.kubernetes.io/component: upgrade-repo
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: harvester-upgrade
        app.kubernetes.io/component: upgrade-repo
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - upgrade-repo
            topologyKey: kubernetes.io/hostname
      containers:
      - image: rancher/harvester-upgrade:v1.5.2-rc2
        name: iso-mounter
        command:
        - sh
        - -c
        - |
          mount -o loop,ro /iso/harvester.iso /share-mount
          echo "harvester.iso mounted successfully"
          trap "umount -v /iso/harvester.iso; exit 0" EXIT
          while true; do sleep 30; done
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /iso
          name: iso
        - mountPath: /share-mount
          name: share-mount
          mountPropagation: Bidirectional
      - image: nginx
        name: repo
        ports:
        - containerPort: 80
        livenessProbe:
          exec:
            command:
            - sh
            - -c
            - |
              cat /usr/share/nginx/html/harvester-release.yaml 2>&1 /dev/null
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /harvester-release.yaml
            port: 80
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 5
          successThreshold: 1
          failureThreshold: 1
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /usr/share/nginx/html
          name: share-mount
          mountPropagation: Bidirectional
      initContainers:
      - image: rancher/harvester-upgrade:v1.5.2-rc2
        name: iso-downloader
        command:
        - sh
        - -c
        - |
          set -e

          WORK_DIR="/iso"
          LOCK_FILE="leader.lock"
          READY_FLAG="harvester.iso.ready"

          if mkdir "$WORK_DIR"/"$LOCK_FILE" 2>/dev/null; then
            trap "rmdir $WORK_DIR/$LOCK_FILE; rm -vf $WORK_DIR/$READY_FLAG; exit 1" EXIT

            echo "$POD_NAME is the leader, start preparing the ISO image..."
            curl -sSfL http://longhorn-backend.longhorn-system:9500/v1/backingimages/$BI_IMAGE_NAME/download -o harvester.iso.gz

            echo "Download completed, extracting harvester.iso..."
            gzip -dc harvester.iso.gz > "$WORK_DIR"/harvester.iso

            echo "harvester.iso is ready"
            touch "$WORK_DIR"/"$READY_FLAG"

            trap - EXIT
          else
            echo "$POD_NAME is a follower, waiting for harvester.iso downloaded..."

            if [ -f "$WORK_DIR"/"$READY_FLAG" ]; then
              echo "harvester.iso already exists"
            else
              until [ -f "$WORK_DIR"/"$READY_FLAG" ]; do
                echo "harvester.iso is not ready yet, waiting..."
                sleep 10
              done
              echo "harvester.iso is ready"
            fi
          fi
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: BI_IMAGE_NAME
          value: vmi-43ba8c47-5963-4287-902d-7e09d00d9462
        volumeMounts:
        - mountPath: /iso
          name: iso
      volumes:
      - name: iso
        persistentVolumeClaim:
          claimName: hvst-upgrade-repo-image-store
      - name: share-mount
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: upgrade-repo
  name: hvst-upgrade-repo
  namespace: harvester-system
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app.kubernetes.io/name: harvester-upgrade
    app.kubernetes.io/component: upgrade-repo
```

#### Upgrade Manager

Upgrade Manager reconciles Upgrade CRs.

1. **Image-preload** phase
   1. Create image-preload Plans (same as Upgrade V1)
1. **Cluster-upgrade** phase
   1. Decouple the cluster itself with the embedded Rancher Manager by removing the `kubernetesVersion` and `rkeConfig` fields from the `local` provisioning cluster CR
   1. Wait until the cluster becomes stable again
   1. Upgrade Rancher with `helm upgrade rancher rancher --repo=https://releases.rancher.com/server-charts/latest --namespace=cattle-system --version=2.12.0 --reset-then-reuse-values --set=rancherImageTag=v2.12.0`
   1. Wait until all the relevant components settled, including
      - rancher
      - rancher-webhook
      - capi-controller-manager
      - system-upgrade-controller
      - fleet-controller
      - fleet-agent
   1. Upgrade Harvester Cluster Repo with `kubectl -n cattle-system patch deployment harvester-cluster-repo --type=json -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value":"rancher/harvester-cluster-repo:v1.6.0"}]'`
   1. Force update the `harvester-charts` ClusterRepo with `kubectl patch clusterrepo harvester-charts --type=json -p='[{"op": "replace", "path": "/spec/forceUpdate", "value": "2025-09-01T09:48:37Z"}]'`
   1. Upgrade Harvester CRD chart with `kubectl -n fleet-local patch managedcharts harvester-crd --type=json -p '[{"op": "replace", "path": "/spec/version", "value": "1.6.0"}]'`
   1. Wait until the Harvester CRD ManagedChart finish the upgrade
   1. Upgrade Harvester chart with `kubectl -n fleet-local patch managedcharts harvester --type=json -p '[{"op": "replace", "path": "/spec/version", "value": "1.6.0"}]'`
   1. Wait until the Harvester ManagedChart finish the upgrade
      - Harvester and its family components
      - KubeVirt
      - Longhorn
1. **Node-upgrade** phase
   1. Upgrade the management nodes with a set crafted Plan custom resources using the System Upgrade Controller
      1. Evacuate all the user workloads on the to-be-upgraded node
      1. Drain and cordon the node
      1. Upgrade RKE2 for the node
      1. Upgrade the OS for the node
   1. Upgrade the witness node using the same method
   1. Upgrade the worker nodes using the same method
   1. Clean up the Plan custom resources

The following manifests demonstrate what the SUC Plan for the RKE2-upgrade stage looks like (not complete yet, use with caution):

```yaml
apiVersion: upgrade.cattle.io/v1
kind: Plan
metadata:
  name: server-plan
  namespace: cattle-system
  labels:
    rke2-upgrade: server
spec:
  concurrency: 1
  nodeSelector:
    matchExpressions:
    - {key: node-role.kubernetes.io/control-plane, operator: In, values: ["true"]}
  tolerations:
  - key: CriticalAddonsOnly
    operator: Equal
    value: "true"
    effect: NoExecute
  serviceAccountName: system-upgrade-controller
  cordon: true
  drain:
    force: true
  upgrade:
    image: rancher/rke2-upgrade
  version: v1.33.3-rke2r1
---
apiVersion: upgrade.cattle.io/v1
kind: Plan
metadata:
  name: agent-plan
  namespace: cattle-system
  labels:
    rke2-upgrade: agent
spec:
  concurrency: 1
  nodeSelector:
    matchExpressions:
    - {key: kubernetes.io/os, operator: In, values: ["linux"]}
    - {key: node-role.kubernetes.io/control-plane, operator: NotIn, values: ["true"]}
  prepare:
    args:
    - prepare
    - server-plan
    image: rancher/rke2-upgrade
  serviceAccountName: system-upgrade-controller
  cordon: true
  drain:
    force: true
  upgrade:
    image: rancher/rke2-upgrade
  version: v1.33.3-rke2r1
```

### Test Plan

Although there are two stages for Upgrade Manager, the **Transition** stage and the **Complete Upgrade V2** stage, the differences are intrinsic and should be inconceivable to users. The test plan should be the same as before. As a side note, there should be no differences in single-node cluster upgrades and others, unlike Upgrade V1, thanks to the unification of the inner workings of the node-upgrade phase. The test plan should cover the following cluster formations:

- Single-node cluster
- Two-node cluster (one management node and one worker node)
- Three-node cluster (three management node)
- Three-node cluster with one witness node (two management nodes and one witness node)
- Four-node cluster (three management nodes and one worker node)

The detailed steps are as follows (taking upgrading to v1.5.2 as an example):

1. Create a Harvester cluster
1. Get the ISO image and put it in a place that can be downloaded by the cluster through HTTP
1. Create the Version CR via `kubectl create`
   ```yaml
   apiVersion: management.harvesterhci.io/v1beta1
   kind: Version
   metadata:
     name: v1.5.2
   spec:
     isoChecksum: 'fe189148de9921b2d9c3966c8bf10d7dbb7a3acbdb26e4b1669cbf1cff5cf96bdd7821217f8e92d10f82aa948fe3a1411070bc6ea9bbbf1dfb5c1689896bca8e'
     isoURL: https://releases.rancher.com/harvester/v1.5.2/harvester-v1.5.2-amd64.iso
     releaseDate: '20250917'
   ```
1. Click the "Upgrade" button on the Harvester dashboard
   Or, instead, create the UpgradePlan CR via `kubectl create`
   ```yaml
   apiVersion: management.harvesterhci.io/v1beta1
   kind: UpgradePlan
   metadata:
     generateName: hvst-upgrade-
   spec:
     version: v1.5.2  # refer to the name of the Version CR
   ```
1. After the upgrade process completes successfully, check whether the components are all upgraded (comparing with the metadata file packaged in the ISO image)
   ```shell
   $ isoinfo -J -i harvester-v1.5.2-amd64.iso -x /harvester-release.yaml
   harvester: v1.5.2
   harvesterChart: 1.5.2
   installer: 745386f
   os: Harvester v1.5.2
   kubernetes: v1.32.7+rke2r1
   rancher: v2.11.3
   monitoringChart: 105.1.2+up61.3.2
   loggingChart: 105.2.0+up4.10.0
   kubevirt: 1.4.1-150600.5.21.2
   minUpgradableVersion: 'v1.4.2'
   rancherDependencies:
     fleet:
       chart: 106.1.2+up0.12.4
       app: 0.12.4
     fleet-crd:
       chart: 106.1.2+up0.12.4
       app: 0.12.4
     rancher-webhook:
       chart: 106.0.3+up0.7.3
       app: 0.7.3
   ```

### Upgrade Strategy

Harvester Upgrade V2 will be gradually rolled out in multiple (hopefully two) Harvester minor releases to meet the development bandwidth and ensure smooth upgrade experiences.

**The first phase** will focus on shipping the code where we deprecate the Upgrade Repository VM and extract the upgrade controller to Upgrade Manager, leaving Upgrade Shim the only upgrade-relevant part in the Harvester main program. The Upgrade V2 path is not followed in this phase; upgrade clusters still go down the Upgrade V1 path.

**The second phase** is where the new Upgrade Shim, Repository, and Manager take over the upgrade procedure. The upgrade-relevant APIs, Upgrade and Version CRDs, are the same as before. The inner workings of Upgrade Manager are essentially the same as before. This is because this phase occurs during patch version upgrades, and no new APIs or features can be introduced. The Upgrade V2 enhancement also follows the rules.

**The third phase** is where Upgrade V2 unleashes its full potential. Upgrade Shim and Repository largely remain the same as in the second phase; however, Upgrade Manager works differently internally, especially in the node-upgrade part. Besides, since we use the new version Upgrade Manager in almost the entire upgrade procedure, it becomes possible to introduce any changes at an early stage of an upgrade. This is even true for introducing the Upgrade V2 API.

For instance, if Harvester Upgrade V2 is first introduced in Harvester v1.7.0, it can only take effect for upgrades to v1.7.1 and v1.8.x versions from v1.7.0. To be precise, the upgrades to v1.7.y from v1.7.x goes down the semi-Upgrade V2 path, with Upgrade Manager, which works pretty much the same as the old upgrade controller. Finally, clusters upgrade to v1.7.x from v1.6.x still goes down the Upgrade V1 path.

In summary, we still need to handle the (hopefully) last time two-release fix cycle paradox by planning the rollout strategy carefully.

> [!IMPORTANT]
> It's totally feasible to drop the entire third phase if we realize dropping Rancher v2prov is impossible due to other reasons. The rollout strategy is deliberately designed to be flexible.

## Note [optional]

### Clarification of the Reliance of Rancher System Agent

The Rancher System Agent is a system agent that works with Rancher Manager and is responsible for executing node-level tasks, including one-time instructions and regular probes. It plays a crucial part in the Harvester Upgrade mechanics because, in essence, Rancher v2 Provisioning, which Harvester Upgrade leverages, relies on the Rancher System Agent to upgrade Kubernetes runtime, i.e., RKE2. However, as proposed by this HEP, we plan to drop the dependence on Rancher Manager in Upgrade V2; therefore, the Rancher System Agent is no longer of use. The Rancher System Agent on each Harvester node won't disappear by itself. We will need a way to clean it up.

Another significant aspect is installation. Do we rely on Rancher System Agent to bootstrap clusters? What are the impacts if we drop it entirely? Do we need to develop a new installation method to fill the gap left by the decommissioning of the Rancher System Agent?

It appears that we do rely on the Rancher System Agent to bootstrap nodes except for the initial one. If we remove the Rancher System Agent, there might not be issues in day 2 management; however, we cannot live without it when it comes to cluster bootstrapping. If we leave it as is, the Rancher System Agent generates numerous logs containing error messages (because it is no longer able to communicate with the Rancher Manager) and does nothing.

```
...
Sep 01 07:30:49 charlie-1-tink-system rancher-system-agent[9119]: time="2025-09-01T07:30:49Z" level=fatal msg="[K8s] received nil secret that was nil, stopping"
Sep 01 07:30:49 charlie-1-tink-system systemd[1]: rancher-system-agent.service: Main process exited, code=exited, status=1/FAILURE
Sep 01 07:30:49 charlie-1-tink-system systemd[1]: rancher-system-agent.service: Failed with result 'exit-code'.
Sep 01 07:30:54 charlie-1-tink-system systemd[1]: rancher-system-agent.service: Scheduled restart job, restart counter is at 1.
Sep 01 07:30:54 charlie-1-tink-system systemd[1]: Stopped Rancher System Agent.
Sep 01 07:30:54 charlie-1-tink-system systemd[1]: Started Rancher System Agent.
Sep 01 07:30:54 charlie-1-tink-system rancher-system-agent[5738]: time="2025-09-01T07:30:54Z" level=info msg="Rancher System Agent version v0.3.12 (e4876a6) is starting"
Sep 01 07:30:54 charlie-1-tink-system rancher-system-agent[5738]: time="2025-09-01T07:30:54Z" level=info msg="Using directory /var/lib/rancher/agent/work for work"
Sep 01 07:30:54 charlie-1-tink-system rancher-system-agent[5738]: time="2025-09-01T07:30:54Z" level=info msg="Starting remote watch of plans"
Sep 01 07:30:54 charlie-1-tink-system rancher-system-agent[5738]: time="2025-09-01T07:30:54Z" level=fatal msg="error while connecting to Kubernetes cluster: the server has asked for the client to provide credentials"
Sep 01 07:30:54 charlie-1-tink-system systemd[1]: rancher-system-agent.service: Main process exited, code=exited, status=1/FAILURE
Sep 01 07:30:54 charlie-1-tink-system systemd[1]: rancher-system-agent.service: Failed with result 'exit-code'.
```

### Extensibility of Upgrade V2

Considering that numerous patches are being introduced in various phases throughout each version's upgrade path, a thoughtful extension framework must be devised.
