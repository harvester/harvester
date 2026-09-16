# Harvester Addons

This directory is the single home for Harvester addon definitions. Addons are organized by delivery model:

```text
addons/
├── built-in/                 # packaged into the Harvester ISO
├── standalone/               # installed after Harvester installation (not bundled in the ISO)
└── packaging/                # build inputs and packaging scripts
```

Each addon directory contains a `metadata.yaml` packaging/maturity contract and a manifest (`addon-template.yaml` for built-in addons, `addon.yaml` for standalone addons). Delivery model and maturity are independent: `stage` describes support status, while the parent directory describes how the addon is delivered.

See [pkg/addons/render](../pkg/addons/render) for the generator library and [cmd/addon-generator](../cmd/addon-generator) for the CLI that assembles, renders, and validates addons from this directory.

## metadata.yaml contract

```yaml
name: <addon-name>
namespace: <k8s-namespace>
stage: experimental | preview | ga
deprecated: true | false   # optional, defaults to false
```

- `stage` reflects maturity/support commitment and is independent of the addon delivery category.
- Addons under `built-in/` carry `addon-template.yaml`, a fragment of the rancherd bootstrap template assembled at build time.
- Addons under `standalone/` carry `addon.yaml` and are installed after Harvester installation rather than being bundled in the ISO; install with:
  ```
  kubectl apply -f https://raw.githubusercontent.com/harvester/harvester/<version>/addons/standalone/<name>/addon.yaml
  ```
  where `<version>` is a release tag (e.g. `v1.10.0`) or matching branch; `master` for the dev version.
- `deprecated: true` addons must be under `standalone/`; retirement starts by removing the addon from the ISO. `stage` keeps its pre-retirement value. An addon deprecated in release N is removed from the repository in release N+1 (its directory and manifest deleted).

Labels (`addon.harvesterhci.io/{experimental,preview,ga,deprecated}`) are derived from `metadata.yaml` by the generator; hand-written label drift fails CI.

## Catalog

### Built-in addons

| Addon | Stage |
|---|---|
| [vm-import-controller](built-in/vm-import-controller/metadata.yaml) | ga |
| [pcidevices-controller](built-in/pcidevices-controller/metadata.yaml) | ga |
| [rancher-logging](built-in/rancher-logging/metadata.yaml) | ga |
| [rancher-monitoring](built-in/rancher-monitoring/metadata.yaml) | ga |
| [nvidia-driver-toolkit](built-in/nvidia-driver-toolkit/metadata.yaml) | ga |
| [harvester-seeder](built-in/harvester-seeder/metadata.yaml) | ga |
| [kubeovn-operator](built-in/kubeovn-operator/metadata.yaml) | experimental |
| [descheduler](built-in/descheduler/metadata.yaml) | experimental |

### Standalone addons

| Addon | Stage |
|---|---|
| [harvester-csi-driver-lvm](standalone/harvester-csi-driver-lvm/metadata.yaml) | experimental |
| [harvester-upgrade-manager](standalone/harvester-upgrade-manager/metadata.yaml) | experimental |
| [harvester-vm-dhcp-controller](standalone/harvester-vm-dhcp-controller/metadata.yaml) | experimental |
| [rancher-k3k](standalone/rancher-k3k/metadata.yaml) | experimental |
| [rancher-vcluster](standalone/rancher-vcluster/metadata.yaml) | experimental |
| [suse-observability-agent](standalone/suse-observability-agent/metadata.yaml) | experimental |

## Other files

- `version_info` — bash contract of chart/image versions, sourced by build scripts.
- `packaging/hack/` — chart patch and image-check scripts used by the ISO builder.
- `packaging/config/templates/patch/` — chart patch payloads for `rancher-monitoring`/`rancher-logging`.
