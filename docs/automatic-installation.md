# Automatic Installation

Harvester can be installed in a mass manner. This document provides an example to do the automatic installation with PXE boot.

## Preparing TFTP and HTTP servers

TFTP or HTTP servers are required to serve boot files. Please ensure these servers are setup correctly before continuing.

We recommend using [iPXE](https://ipxe.org/) to perform the network boot. It has more features than PXELINUX and is likely available in modern NIC cards. If NIC cards don't come with iPXE firmware, iPXE firmware images can be loaded from the TFTP server first. TFTP server is also used to serve iPXE scripts.

Let's assume the HTTP server's IP is `10.100.0.10`, and the HTTP server serves `/usr/share/nginx/html/` folder at path `http://10.100.0.10/`.

## Preparing boot files

- Download the [Harvester ISO](https://github.com/rancher/harvester/tags).

- Serve the ISO file.
  
  Copy or move the ISO file to an appropriate location so it can be downloaded via HTTP server. e.g.,

  ```
  sudo mkdir -p /usr/share/nginx/html/harvester/
  sudo cp /path/to/harvester-amd64.iso /usr/share/nginx/html/harvester/
  ```

- Extract kernel and initrd file from the ISO file.

  ```
  mkdir /tmp/harvester-iso
  mkdir /tmp/harvester-squashfs
  sudo mount -o loop /path/to/harvester-amd64.iso /tmp/harvester-iso
  sudo mount /tmp/harvester-iso/k3os/system/kernel/current/kernel.squashfs /tmp/harvester-squashfs

  sudo cp /tmp/harvester-squashfs/vmlinuz /usr/share/nginx/html/harvester/
  sudo cp /tmp/harvester-iso/k3os/system/kernel/current/initrd /usr/share/nginx/html/harvester/

  sudo umount /tmp/harvester-squashfs
  sudo umount /tmp/harvester-iso
  rm -r /tmp/harvester-iso /tmp/harvester-squashfs
  ```

  Make sure the `vmlinuz` and `initrd` files can be downloaded from the HTTP server.

## Preparing iPXE boot scripts

When performing automatic installation, there are two modes:
- `CREATE`: we are installing a node to construct an initial Harvester cluster.
- `JOIN`: we are installing a node to join an existing Harvester cluster.

### CREATE mode

Create a [Harvester configuration file](./harvester-configuration.md) `config-create.yaml` for `CREATE` mode, modify the values as needed:

```
# cat /usr/share/nginx/html/harvester/config-create.yaml
token: token
os:
  hostname: node1
  ssh_authorized_keys:
  - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDbeUa9A7Kee+hcCleIXYxuaPksn2m4PZTd4T7wPcse8KbsQfttGRax6vxQXoPO6ehddqOb2nV7tkW2mEhR50OE7W7ngDHbzK2OneAyONYF44bmMsapNAGvnsBKe9rNrev1iVBwOjtmyVLhnLrJIX+2+3T3yauxdu+pmBsnD5OIKUrBrN1sdwW0rA2rHDiSnzXHNQM3m02aY6mlagdQ/Ovh96h05QFCHYxBc6oE/mIeFRaNifa4GU/oELn3a6HfbETeBQz+XOEN+IrLpnZO9riGyzsZroB/Y3Ju+cJxH06U0B7xwJCRmWZjuvfFQUP7RIJD1gRGZzmf3h8+F+oidkO2i5rbT57NaYSqkdVvR6RidVLWEzURZIGbtHjSPCi4kqD05ua8r/7CC0PvxQb1O5ILEdyJr2ZmzhF6VjjgmyrmSmt/yRq8MQtGQxyKXZhJqlPYho4d5SrHi5iGT2PvgDQaWch0I3ndEicaaPDZJHWBxVsCVAe44Wtj9g3LzXkyu3k= root@admin
  password: rancher
install:
  mode: create
  mgmt_interface: eth0
  device: /dev/sda
  iso_url: http://10.100.0.10/harvester-amd64.iso

```

For machines that needs to be installed as `CREATE` mode, the following is an iPXE script that boots the kernel with the above config:

```
#!ipxe
kernel http://10.100.0.10/harvester/vmlinuz k3os.mode=install console=ttyS0 console=tty1 harvester.install.automatic=true harvester.install.config_url=http://10.100.0.10/harvester/config-create.yaml
initrd http://10.100.0.10/harvester/initrd
boot
```

### JOIN mode

Create a [Harvester configuration file](./harvester-configuration.md) `config-join.yaml` for `JOIN` mode, modify the values as needed:

```
# cat /usr/share/nginx/html/harvester/config-join.yaml
server_url: https://10.100.0.130:6443
token: token
os:
  hostname: node2
  ssh_authorized_keys:
  - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDbeUa9A7Kee+hcCleIXYxuaPksn2m4PZTd4T7wPcse8KbsQfttGRax6vxQXoPO6ehddqOb2nV7tkW2mEhR50OE7W7ngDHbzK2OneAyONYF44bmMsapNAGvnsBKe9rNrev1iVBwOjtmyVLhnLrJIX+2+3T3yauxdu+pmBsnD5OIKUrBrN1sdwW0rA2rHDiSnzXHNQM3m02aY6mlagdQ/Ovh96h05QFCHYxBc6oE/mIeFRaNifa4GU/oELn3a6HfbETeBQz+XOEN+IrLpnZO9riGyzsZroB/Y3Ju+cJxH06U0B7xwJCRmWZjuvfFQUP7RIJD1gRGZzmf3h8+F+oidkO2i5rbT57NaYSqkdVvR6RidVLWEzURZIGbtHjSPCi4kqD05ua8r/7CC0PvxQb1O5ILEdyJr2ZmzhF6VjjgmyrmSmt/yRq8MQtGQxyKXZhJqlPYho4d5SrHi5iGT2PvgDQaWch0I3ndEicaaPDZJHWBxVsCVAe44Wtj9g3LzXkyu3k= root@admin
  dns_nameservers:
  - 1.1.1.1
  - 8.8.8.8
  password: rancher
install:
  mode: join
  mgmt_interface: eth0
  device: /dev/sda
  iso_url: http://10.100.0.10/harvester/harvester-amd64.iso
```

Note the `mode` is `join` and `server_url` needs to be provided.

For machines that needs to be installed as `JOIN` mode, the following is an iPXE script that boots the kernel with the above config:

```
#!ipxe
kernel http://10.100.0.10/harvester/vmlinuz k3os.mode=install console=ttyS0 console=tty1 harvester.install.automatic=true harvester.install.config_url=http://10.100.0.10/harvester/config-join.yaml
initrd http://10.100.0.10/harvester/initrd
boot
```

**NOTE**: Nodes need to have at least **8G** of RAM because the full ISO file is loaded into tmpfs during the installation.

## Harvester configuration

For more information about Harvester configuration, please refer to the [Harvester configuration](./harvester-configuration.md).

Users can also provide configuration via kernel parameters. For example, to specify the `CREATE` install mode, the user can pass `harvester.install.mode=create` kernel parameter when booting. Values passed through kernel parameters have higher priority than values specified in the config file.
