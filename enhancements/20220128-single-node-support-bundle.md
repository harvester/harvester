# Add single node support bundle feature

## Summary

We can add a command to collect a single-node support bundle to reduce the issue resolution time.

A single-node support bundle is handy in these cases:
- Troubleshooting a failed installation.
- Troubleshooting a failed bootstrapped node. e.g., a node can't join a Harvester cluster.
- When the Harvester backend or Kubernetes runtime has problems.

### Related Issues

- https://github.com/harvester/harvester/issues/1864

## Motivation

### Goals

- A user can run a command on a node to generate a node support bundle.
- [Node bundle collector](https://github.com/rancher/support-bundle-kit/blob/d1c9e6da233b5280cc78b69df55293307b5e0848/hack/collector-harvester) in the current cluster-level support bundle feature should use this command to collect node bundles.


## Proposal

### User Stories

#### Including a node support bundle when reporting a failed installation.

- https://github.com/harvester/harvester/issues/1865
- https://github.com/harvester/harvester/issues/1863

Taking the above issues as examples, we need to ask users for more information to help them. Sometimes, it takes several rounds to get helpful information. A node support bundle should contain general information to troubleshoot a failed installation and we can add the instruction to generate it in the issue reporting template.


#### Including a node support bundle when reporting a failed bootstrapped node.

A node support bundle is also helpful to troubleshoot a non-bootstrapped node. e.g., a node can't join a Harvester cluster.


### User Experience In Detail

The user runs a command to generate a node support bundle:

```
# harvester-supportconfig
```

The command output contains the generated file path, e.g.:

```
...
Creating Tar Ball

==[ DONE ]===================================================================
  Log file tar ball: /var/log/scc_node1_220128_0552.txz
  Log file size:     6.4M
  Log file md5sum:   580031fe2943d43ecf83bfe6c8db6cf5-f
=============================================================================
```

### API changes

No API changes.

## Design

### Implementation Overview

We leverage the `supportconfig` command in [openSUSE supportutils](https://github.com/openSUSE/supportutils) package.

The `supportconfig` command is the standard utility to collect system information for SUSE distributions. It already contains much useful information and some basic health checks, so we don't need to reinvent wheels. The following text contains the file list in a `supportconfig` tarball, we can see there are many components:

```shell
node1:/tmp/scc_node1_220128_0552 # ls
basic-environment.txt	open-files.txt
basic-health-check.txt	pam.txt
boot.txt		print.txt
cimom.txt		proc.txt
crash.txt		rpm.txt
cron.txt		rpm-verify.txt
dhcp.txt		samba.txt
dns.txt			sar.txt
docker			security-apparmor.txt
docker.txt		security-audit.txt
env.txt			shell_history.txt
etc.txt			slert.txt
fs-autofs.txt		slp.txt
fs-btrfs.txt		smt.txt
fs-diskio.txt		ssh.txt
fs-files.txt		sssd.txt
fs-iscsi.txt		summary.xml
fs-smartmon.txt		supportconfig.txt
fs-softraid.txt		sysconfig.txt
hardware.txt		sysfs.txt
ldap.txt		systemd.txt
lvm.txt			transactional-update.txt
memory.txt		tuned.txt
messages.txt		udev.txt
modules.txt		updates.txt
mpio.txt		web.txt
nfs.txt			y2log.txt
ocfs2.txt
```

The components can be included or excluded according to our needs.

The `supportconfig` command can be extended via plugins. We add new plugins to collect various component information:
 - `harvester`: collect installer logs, cloud-init files, and other useful information that is not included in standard supportconfig modules.
- `rke2`: collect RKE2 configs and logs.
- `rancherd`: collect rancherd configs, plans, and logs.
- `rancher-system-agent`: collect `rancher-system-agent` configs, plans, and logs.

Take `harvester` plugin as the example: a generated node support bundle contains a file `plugin-harvester.txt` that contains information:

```
#==[ Plugin ]=======================================#
# Output: /usr/lib/supportconfig/plugins/harvester

#==[ Command ]======================================#
# /usr/lib/supportconfig/plugins/harvester

#==[ Section Header ]===============================#
# Harvester: installed mode


#==[ Section Header ]===============================#
# Luet packages

#==[ Command ]======================================#
# /usr/bin/luet search --installed
> toolchain/luet-cosign-0.0.10-3
 -> Category: toolchain
 -> Name: luet-cosign
 -> Version: 0.0.10-3
 -> Description:
 -> Repository: system
 -> Uri:
 -> Installed: true
```

The plugin creates a `harvester` directory to store configs and logs:

```
# find harvester/
harvester/
harvester/oem
harvester/oem/99_custom.yaml
harvester/oem/harvester.config
harvester/console.log
```

Similar mechanisms apply to other plugins as well.

Note the config files might contain sensitive information like passwords. We need to remove them when generating a support bundle.


### Test plan

- Booting a Harvester ISO and changing to TTY2. Run `harvester-supportconfig` to generate a node support bundle. The bundle should contain node information.
- After installation, log in to the node and run `harvester-supportconfig` command to generate a node support bundle. The bundle should contain node information.
- Use Harvester GUI to generate a support bundle. In the `nodes` directory, there should exist node support bundles for all Harvester nodes.

### Upgrade strategy

The new command will be included in the next version of the OS. It's available after upgrading Harvester to the next version.

