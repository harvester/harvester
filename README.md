# Nexus

A React + TypeScript Harvester UI prototype for generating Kubernetes manifests, provisioning advanced storage backends, and exploring multi-cluster deployment workflows.

## Nexus release overview

Nexus is the updated Harvester fork with:

- Extended storage backend support: iSCSI, GlusterFS, Longhorn, OpenEBS, Portworx, NVMe-oF, RDMA, Ceph, ZFS, NFS, SMB, and local storage.
- Networking and service mesh support: Istio, Linkerd, Cilium, Service, Ingress, and NetworkPolicy.
- Security and compliance scaffolding: RBAC, Pod Security Standards, service accounts, and workload annotations.
- Observability tooling templates: Prometheus monitoring, Fluentd/Loki/Splunk logging.
- GitOps support: ArgoCD, Flux, Jenkins X integration manifests.
- Multi-cluster provisioning: federated deployment templates and cluster targeting.

## What is included

- Wizard-driven workload and manifest configuration.
- Storage selection for local, NFS, SMB, Ceph, NVMe-oF, RDMA, ZFS, iSCSI, GlusterFS, Longhorn, OpenEBS, and Portworx.
- Auto-generated `Deployment`, `StatefulSet`, `DaemonSet`, `Job`, and `CronJob` manifests.
- PVC, Service, Ingress, NetworkPolicy, RBAC, monitoring, logging, GitOps, and multi-cluster manifest generation.
- Service mesh integration support for Istio, Linkerd, and Cilium.
- YAML editor preview with dark mode styling.

## Quick start

1. Install dependencies: `npm install`
2. Run the dev server: `npm run dev`
3. Open the browser at `http://localhost:4173`

## Verified build

- Production build passes with `npm run build`.
- The generated bundle is ready for local demo and review.

## Demo installation

This prototype is intended as a front-end Nexus demo for Harvester-style workload generation. After launching the app locally, use the wizard to select storage, networking, security, monitoring, GitOps, and multi-cluster options, then review generated manifests in the built-in YAML editor.

### Run locally

1. Clone or navigate to the project folder.
2. Install dependencies: `npm install`
3. Start the app: `npm run dev`
4. Open the browser at `http://localhost:4173`

### GitHub branch and pull request

The updated Nexus version is available on the `nexus` branch in the forked repository:

- `https://github.com/sggr57a/harvester/tree/nexus`
- Pull request: `https://github.com/sggr57a/harvester/pull/1`

### Run locally

1. Clone or navigate to the project folder.
2. Install dependencies: `npm install`
3. Start the app: `npm run dev`
4. Open the browser at `http://localhost:4173`

### GitHub branch and pull request

The updated Nexus version is available on the `nexus` branch in the forked repository:

- `https://github.com/sggr57a/harvester/tree/nexus`

A pull request can be created from this branch to merge Nexus back into the main repository.

## Next steps

- Add Kubernetes API validation and live preview.
- Add manifest apply / test runner using `kubectl` or client libraries.
- Add virtual cluster support (`vcluster` / multi-cluster).
- Add editor enhancements using Monaco or CodeMirror.
- Add storage backend templates for real CSI drivers.
