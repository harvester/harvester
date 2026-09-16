# Harvester Addons

This directory is the single home for Harvester addons: each addon owns a
directory containing a `metadata.yaml` packaging/maturity contract, a manifest
(`addon-template.yaml` if built-in, `addon.yaml` otherwise), and a `README.md`.

See [pkg/addons/render](../pkg/addons/render) for the generator library and
[cmd/addon-generator](../cmd/addon-generator) for the CLI that assembles,
renders, and validates addons from this directory.

## metadata.yaml contract

```yaml
name: <addon-name>
namespace: <k8s-namespace>
stage: experimental | preview | ga
builtIn: true | false
deprecated: true | false   # optional, defaults to false
```

- `stage` reflects maturity/support commitment and is independent of `builtIn`.
- `builtIn: true` addons are packaged into the ISO and carry `addon-template.yaml`,
  a fragment of the rancherd bootstrap template assembled at build time.
- `builtIn: false` addons are not packaged into the ISO; install with:
  ```
  kubectl apply -f https://raw.githubusercontent.com/harvester/harvester/<version>/addons/<name>/addon.yaml
  ```
  where `<version>` is a release tag (e.g. `v1.10.0`) or matching branch;
  `master` for the dev version.
- `deprecated: true` addons must have `builtIn: false`; retirement starts by
  removing the addon from the ISO. `stage` keeps its pre-retirement value.

Labels (`addon.harvesterhci.io/{experimental,preview,ga,deprecated}`) are
derived from `metadata.yaml` by the generator; hand-written label drift fails CI.

## Catalog

| Addon | Stage | Built-in |
|---|---|---|
| [vm-import-controller](vm-import-controller/README.md) | ga | yes |
| [pcidevices-controller](pcidevices-controller/README.md) | ga | yes |
| [rancher-logging](rancher-logging/README.md) | ga | yes |
| [rancher-monitoring](rancher-monitoring/README.md) | ga | yes |
| [nvidia-driver-toolkit](nvidia-driver-toolkit/README.md) | ga | yes |
| [harvester-seeder](harvester-seeder/README.md) | ga | yes |
| [kubeovn-operator](kubeovn-operator/README.md) | experimental | yes |
| [descheduler](descheduler/README.md) | experimental | yes |
| [harvester-csi-driver-lvm](harvester-csi-driver-lvm/README.md) | experimental | no |
| [harvester-upgrade-manager](harvester-upgrade-manager/README.md) | experimental | no |
| [harvester-vm-dhcp-controller](harvester-vm-dhcp-controller/README.md) | experimental | no |
| [rancher-k3k](rancher-k3k/README.md) | experimental | no |
| [rancher-vcluster](rancher-vcluster/README.md) | experimental | no |
| [suse-observability-agent](suse-observability-agent/README.md) | experimental | no |

## Other files

- `version_info` — bash contract of chart/image versions, sourced by build scripts.
- `hack/` — chart patch and image-check scripts used by the ISO builder.
- `config/templates/patch/` — chart patch payloads for `rancher-monitoring`/`rancher-logging`.
