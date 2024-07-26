# Install Harvester on diskless systems

## Summary

Support Harvester installation to diskless systems

### Related Issues

[#5150](https://github.com/harvester/harvester/issues/5150)

## Motivation

Enterprise users may wish to install Harvester on diskless systems which have boot lun's served from iscsi or san based disk arrays. The HEP details changes need to Harvester to support such possible scenarios

### Goals

Allows users to install and boot Harvester from non-local disks.

### Non-goals [optional]

## Proposal

Introduce additional fields in the harvester installer which allow user to perform the following
* enable and configure multipath for specific vendor / product id
* allow configuration of additional kernel arguments, which are passed to kernel to allow booting of remote disks


### Install and Boot Harvester from remote iscsi array

A Harvester user running enterprise grade hardware without local disks needs to install and configure Harvester. 

Assuming the host has nic with ability to allow boot of remote devices, then the user can already install and configure Harvester on these remote disks. However there is no easy way to configure multipath or kernel arguments need for subsequently booting off the said storage. In this case the user wishes to use a remote `iscsi` disk array.

### API changes

No API changes to Harvester API itself. The changes will be introduced to the harvester-installer where a user can specific additional config arguments via a remote config url.


## Design

The harvester installer will have an additional fields in the `OS` section

```
type OS struct {
    // existing fields have been removed for ease of understanding 
	ExternalStorage           ExternalStorageConfig `json:"externalStorageConfig,omitempty"`
	AdditionalKernelArguments string                `json:"additionalKernelArguments,omitempty"`
}

type ExternalStorageConfig struct {
	Enabled         bool         `json:"enabled,omitempty"`
	MultiPathConfig []DiskConfig `json:"multiPathConfig,omitempty"`
}

type DiskConfig struct {
	Vendor  string `json:"vendor"`
	Product string `json:"product"`
}
```

### Implementation Overview

An end user would configure their host to leverage capable nic to boot off remote iscsi disks. 

Once the hardware nic configuration is complete, the harvester installer should see the iscsi disks in the disk panel

**NOTE:** in case of multipathing the same disk may appear twice

User just needs to choose one of the disks only. In case data needs to be on the same disk please ensure to choose the same disk identifier for the data disk too

User can then configure additional kernel arguments and multipath support via the a config.yaml which would looks as follows:

```
os:
  externalStorageConfig:
    enabled: true
    multiPathConfig:
    - vendor: IET
      product: MediaFiles
  additionalKernelArguments: "rd.iscsi.firmware rd.iscsi.ibft"
```

The installation will proceed as usual, and once complete, the host will be able to boot off iscsi disks, with multipathing enabled.

### Test plan
* Configure a host to boot off a remote iscsi disk, steps may vary based on hardware vendor
* Boot off harvester installer
* Proceed with installation by choosing the remote iscsi disk as installation disk
* Provide additional config as shown above, and complete installation
* Harvester should boot off the remote disk device


### Upgrade strategy

No change to upgrade strategy.

## Note [optional]
