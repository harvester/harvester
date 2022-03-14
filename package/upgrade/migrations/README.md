The directory contains drop-ins to fix things when upgrading from a previous version. 

# `./managed_charts/`

Say you need to do something to a ManagedChart if the previous version is `v1.0.0`.
You can create a file `v1.0.0.sh` in `./managed_charts/` directory to apply the fixes you need.

If the upgrade_manifests.sh script detects the system is upgraded from `v1.0.0`, it will call the script with:

```
v1.0.0.sh <chart_name> <chart_manifest>
```

- The `<chart_name>` could be `harvester`, `harvester-crd`, `rancher-monitoring`, etc.
- The `<chart_manifest>` is the manifest file that contains the current managed chart dump.
