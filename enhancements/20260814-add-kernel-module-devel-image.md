# Add harvester-kernel-module-devel image

## Summary

We need a way of building additional kernel modules to support third party storage drivers for use on Harvester. We currently have https://hub.docker.com/r/rancher/harvester-nvidia-driver-toolkit/ which includes gcc and the kernel source, and is tagged for every Harvester release, but this image is designed specifically for automatically building the nvidia drivers. We need to create a simliar image (kernel source and build tools, tagged for every Harvester release), but that can be used in a generic fashion to build arbitrary kernel modules.

### Related Issues

https://github.com/harvester/harvester/issues/11263

## Motivation

### Goals

- Allow third parties to build kernel modules for a given Harvester release that they can then give them to users to install.
- Allow third parties to provide configuration (e.g. a DaemonSet manifest) that will build and load additional kernel modules for a given harvester cluster at runtime.
- Allow users to build additional kernel modules from the Harvester kernel source that aren't necessarily supported by SUSE, but might be useful for them (e.g. to enable obsolete NICs, or to add additional device mapper drivers that aren't included in the default SUSE kernel)

### Non-goals [optional]

- Add custom kernel modules to the Harvester initramfs (that's part of the immmutable OS image and can't be changed).
- Install custom kernel modules under `/lib/modules/*` (again, that's part of the immutable OS image).

## Proposal

### User Stories

#### Story 1

I'm a storage vendor and I need to be able build out-of-tree kernel modules in support of my storage solution, that will work with a given Harvester release. I intend to build these modules myself and then let Harvester users download them to use on their Harvester clusters.

#### Story 2

I'm a storage vendor and I need to be able build out-of-tree kernel modules in support of my storage solution, that will work with a given Harvester release. Rather than building the modules myself and offering them for download, I intend the modules to be built at runtime on any Harvester cluster that needs them.

#### Story 3

I need to build and load in-tree kernel modules that aren't included in the default Harvester kernel from SUSE. Maybe I have [obsolete NICs that still actually work](https://github.com/harvester/harvester/issues/9846), or perhaps I want to experiment with [dm-vdo](https://docs.kernel.org/admin-guide/device-mapper/vdo.html).

### User Experience In Detail

#### Building And Loading Out-of-Tree Kernel Modules At Runtime

Create a DaemonSet that will download the relevant module source, build it, and load the module. For example, to build and load the [DRBD](https://linbit.com/drbd/) kernel module:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: drbd-builder
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: drbd-builder
  template:
    metadata:
      labels:
        app.kubernetes.io/name: drbd-builder
    spec:
      containers:
      - name: pause
        image: registry.k8s.io/pause
      initContainers:
      - name: builder
        # change this to point to an actual tagged release (e.g. v1.9-YYYYMMDD) that matches your harvester version
        image: rancher/harvester-kernel-module-devel:sle-micro-head
        securityContext:
          privileged: true
        command:
        - sh
        - -c
        - |
          if lsmod | grep -q '^drbd' ; then
              echo "drbd kernel module is already loaded"
              exit 0
          fi
          curl -Lsf -o - https://pkg.linbit.com/downloads/drbd/9/drbd-9.3.3.tar.gz | tar -xzf -
          cd drbd-9.3.3/
          make -C drbd all KDIR=/usr/src/linux
          modprobe --allow-unsupported ./drbd/build-current/drbd.ko && echo "drbd module loaded"
```

Note that the above hard-codes a specific DRBD version, and won't work in air-gapped environments. As such it should not be considered exacly what one might want in production, it's just an example to demonstrate the concept.

#### Building Kernel Modules In Advance to be Loaded During System Boot

Using DRBD as an example again, start by building the kernel module. Replace `sle-micro-head` in the below with an actual tagged release to match your harvester version:

```
> docker run --rm -it --name drbd-builder rancher/harvester-kernel-module-devel:sle-micro-head
# curl -Lsf -o - https://pkg.linbit.com/downloads/drbd/9/drbd-9.3.3.tar.gz | tar -xzf -
# cd drbd-9.3.3/
# make -C drbd all KDIR=/usr/src/linux
```

Then, in another terminal, copy the module binary out of the container:

```
> docker cp drbd-builder:/drbd-9.3.3/drbd/build-current/drbd.ko .
```

From there, the kernel module can be copied to `/var/lib/third-party/` on each Harvester node, then loaded at boot time by adding a CloudInit CR similar to the following:

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: drbd-loader
spec:
  matchSelector:
    harvesterhci.io/managed: "true"
  filename: 99_drbd_loader
  contents: |
    stages:
      initramfs:
        - name: "load drbd.ko"
          commands:
          - modprobe --allow-unsupported /var/lib/third-party/drbd.ko
```

#### Building and Loading In-Tree Kernel Modules

As the kernel source is included in the `rancher/harvester-kernel-module-devel` image, any desired in-tree kernel module can be built, for example:

```
> docker run --rm -it rancher/harvester-kernel-module-devel:sle-micro-head
# cd /usr/src/linux/drivers/md/dm-vdo
# make -C /usr/src/linux M=$PWD
[...]
# ls dm-vdo.ko
dm-vdo.ko
```

These can then be copied out of the container and loaded with a CloudInit CR as describe above, or automated with a DaemonSet.

The same is true for modules that aren't built as part of the default kernel, but you'll potentially need to reconfigure the kernel build first (think: `make config`, or the rather nicer `make menuconfig`, but to run the latter you'll first need to `zypper in ncurses-devel`).

#### Secure Boot / Module Signing

On hosts with secure boot enabled, kernel modules must be digially signed in order to load, and the signing key's certificate needs to be loaded into the host's trust store. Without this, you will see errors similar to the following:

```
# modprobe --allow-unsupported ./drbd.ko
modprobe: ERROR: could not insert 'drbd': Key was rejected by service

# dmesg|tail
[  625.228390] [  T51097] Loading of unsigned module is rejected
```

A signing key and certificate can be generated with the following command:

```
openssl req -x509 -new -nodes -utf8 -sha256 -days 36500 -batch -outform DER \
    -out signing_key.x509 -keyout signing_key.pem -config - <<EOF
[ req ]
default_bits = 4096
distinguished_name = req_distinguished_name
prompt = no
string_mask = utf8only
x509_extensions = myexts

[ req_distinguished_name ]
# update the below as desired for your site
#O = Unspecified company
CN = My kernel module signing key
#emailAddress = unspecified.user@unspecified.company

[ myexts ]
basicConstraints=critical,CA:FALSE
keyUsage=digitalSignature
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid
extendedKeyUsage=codeSigning
EOF
```

The private key `signing_key.pem` must be kept secure. Do not leave it lying around on random Harvester hosts.

The certificate `signing_key.x509` needs to be loaded onto every host that will be loading your signed modules. Run the following command:

```
# mokutil --import /root/signing_key.x509
input password:
input password again:
```

Then reboot, access the console, and when prompted select "Perform MOK Management" and go through the prompts to import the key.

Kernel modules can be signed by running the following command from inside the `rancher/harvester-kernel-module-devel` image:

```
# /usr/src/linux/scripts/sign-file sha256 /path/to/signing_key.pem /path/to/signing_key.x509 /path/to/module/to/be/signed.ko
```

The Piraeus Datastore project has a clever way of using Kubernetes secrets to pass signing keys to a module loader (see https://piraeus.io/docs/v2.11.0/how-to/secure-boot/). A similar approach could be used in general for anyone building kernel modules using a DaemonSet and our `rancher/harvester-kernel-module-devel` image. The manual step to enrol the MOK remains necessary on each node (this is unavoidable due to the nature of secure boot).

#### If Additional Software is Needed at Build Time

The `rancher/harvester-kernel-module-devel` is preconfigured with the publicly avaiable SLE-BCI/16.0 software repositories, so any additional tools that may be required at build time can be installed in the running container from there.

## Design

### Implementation Overview

- Add `mokutil` to baseos on OBS (see https://build.opensuse.org/requests/1370974).
- Add `kernel-default-devel`, `suse-build-key` and `patch` to baseos-headers on OBS (see https://build.opensuse.org/requests/1370816).
- Make `os2` build and publish `rancher/harvester-kernel-module-devel` (see https://github.com/harvester/os2/pull/271).
- Add `osImage` to `/etc/harvester-release.yaml` (see https://github.com/harvester/harvester/pull/11409).

### Test plan

Build and load some kernel modules as described [above](#user-experience-in-detail).

### Upgrade strategy

When upgrading from one Harvester version to another, users of custom built kernel modules may need to rebuild them, if the new version of Harvester includes an updated kernel.
