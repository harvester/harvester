# Harvester Configuration

Harvester configuration file can be provided during manual or automatic installation to configure various settings. The following is an configuration example:

```yaml
server_url: https://someserver:6443
token: TOKEN_VALUE
os:
  ssh_authorized_keys:
  - ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB...
  - github:username
  hostname: myhost
  modules:
  - kvm
  - nvme
  sysctl:
    kernel.printk: "4 4 1 7"
    kernel.kptr_restrict: "1"
  dns_nameservers:
  - 8.8.8.8
  - 1.1.1.1
  ntp_servers:
  - 0.us.pool.ntp.org
  - 1.us.pool.ntp.org
  wifi:
  - name: home
    passphrase: mypassword
  - name: nothome
    passphrase: somethingelse
  password: rancher
  environment:
    http_proxy: http://myserver
    https_proxy: http://myserver
install:
  mode: create
  mgmtInterface: eth0
  force_efi: true
  device: /dev/vda
  silent: true
  iso_url: http://myserver/test.iso
  poweroff: true
  no_format: true
  debug: true
  tty: ttyS0
```

## Configuration Reference

Below is a reference of all configuration keys.

### `server_url`

The URL of the Harvester server to join as an agent.

This configuration is mandatory when the installation is in `JOIN` mode. It tells the Harvester installer where the main server is.

Example

```yaml
server_url: https://someserver:6443
install:
  mode: join
```




### `token`

The cluster secret or node token. If the value matches the format of a node token it will
automatically be assumed to be a node token. Otherwise it is treated as a cluster secret.

In order for a new node to join the Harvester cluster, the token should match from what server has.

Example

```yaml
token: myclustersecret
```

Or a node token

```yaml
token: "K1074ec55daebdf54ef48294b0ddf0ce1c3cb64ee7e3d0b9ec79fbc7baf1f7ddac6::node:77689533d0140c7019416603a05275d4"
```

### `os.ssh_authorized_keys`

A list of SSH authorized keys that should be added to the default user `rancher`. SSH keys can be obtained from GitHub user accounts by using the format
`github:${USERNAME}`. This is done by downloading the keys from `https://github.com/${USERNAME}.keys`.

Example

```yaml
os:
  ssh_authorized_keys:
  - "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC2TBZGjE+J8ag11dzkFT58J3XPONrDVmalCNrKxsfADfyy0eqdZrG8hcAxAR/5zuj90Gin2uBR4Sw6Cn4VHsPZcFpXyQCjK1QDADj+WcuhpXOIOY3AB0LZBly9NI0ll+8lo3QtEaoyRLtrMBhQ6Mooy2M3MTG4JNwU9o3yInuqZWf9PvtW6KxMl+ygg1xZkljhemGZ9k0wSrjqif+8usNbzVlCOVQmZwZA+BZxbdcLNwkg7zWJSXzDIXyqM6iWPGXQDEbWLq3+HR1qKucTCSxjbqoe0FD5xcW7NHIME5XKX84yH92n6yn+rxSsyUfhJWYqJd+i0fKf5UbN6qLrtd/D"
  - "github:ibuildthecloud"
```

### `os.hostname`

Set the system hostname. This value will be overwritten by DHCP if DHCP supplies a hostname for
the system. If DHCP doesn't offer a hostname and this value is empty, a random hostname will be generated.

Example

```yaml
os:
  hostname: myhostname
```

### `os.modules`

A list of kernel modules to be loaded on start.

Example

```yaml
os:
  modules:
  - kvm
  - nvme
```

### `os.sysctls`

Kernel sysctl to setup on start. These are the same configuration you'd typically find in `/etc/sysctl.conf`.
Must be specified as string values.

```yaml
os:
  sysctl:
    kernel.printk: 4 4 1 7      # the YAML parser will read as a string
    kernel.kptr_restrict: "1"   # force the YAML parser to read as a string
```

### `os.ntp_servers`

**Fallback** ntp servers to use if NTP is not configured elsewhere in the OS.

Example

```yaml
os:
  ntp_servers:
  - 0.us.pool.ntp.org
  - 1.us.pool.ntp.org
```

### `os.dns_nameservers`

**Fallback** DNS name servers to use if DNS is not configured by DHCP or in the OS.

Example

```yaml
os:
  dns_nameservers:
  - 8.8.8.8
  - 1.1.1.1
```

### `os.wifi`

Simple wifi configuration. All that is accepted is `name` and `passphrase`.

Example:

```yaml
os:
  wifi:
  - name: home
    passphrase: mypassword
  - name: nothome
    passphrase: somethingelse
```

### `os.password`

The password for the default user `rancher`. By default there is no password for the `rancher` user.
If you set a password at runtime it will be reset on next boot. The
value of the password can be clear text or an encrypted form. The easiest way to get this encrypted
form is to just change your password on a Linux system and copy the value of the second field from
`/etc/shadow`. You can also encrypt a password using `openssl passwd -1`.

Example

```yaml
os:
  password: "$1$tYtghCfK$QHa51MS6MVAcfUKuOzNKt0"
```

Or clear text

```yaml
os:
  password: supersecure
```


### `os.environment`

Environment variables to be set on k3s and other processes like the boot process.
Primary use of this field is to set the http proxy.

Example

```yaml
os:
  environment:
    http_proxy: http://myserver
    https_proxy: http://myserver
```


### `install.mode`

Harvester installer mode:

- `create`: Creating a new Harvester installer
- `join`: Join an existing Harvester installer. Need to specify `server_url`.

Example

```yaml
install:
  mode: create
```


### `install.mgmtInterface`

The interface that used to build VM fabric network.

Example

```yaml
install:
  mgmtInterface: eth0
```


### `install.force_efi`

Force EFI installation even when EFI is not detected. Default: `false`.


### `install.device`

The device to install the OS.


### `install.silent`

Reserved.


### `install.iso_url`

ISO to download and install from if booting from kernel/vmlinuz and not ISO.


### `install.poweroff`

Shutdown the machine after install instead of rebooting


### `install.no_format`

Do not partition and format, assume layout exists already.


### `install.debug`

Run installation with more logging and configure debug for installed system.


### `install.tty`

The tty device used for console.

Example

```yaml
install:
  tty: ttyS0,115200n8
```
