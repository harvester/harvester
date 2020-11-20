# K3OS Patches

## Workflows for updating K3OS patches

1. Clone k3os repo
```
git clone -b ${K3OS_VERSION} https://github.com/rancher/k3os.git /tmp/k3os
```
2. Apply the patches
```
cd /tmp/k3os
git am $PATH_TO_YOUR_HARVESTER/scripts/iso/k3os-patches/*.patch
```
3. Do modifications on K3OS
```
# write some files
git add/commit ...
```
4. Save the patches
```
# n is the number of commits for generating the patches
git format-patch HEAD -n
```
5. Update the patches in `scripts/k3os-patches`.