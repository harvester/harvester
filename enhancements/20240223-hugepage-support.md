# Title

Hugepage Support DRAFT

## Summary

Changes should be made to the Harvester installer, the Harvester web UI, and the underlying VM support
to make it easy for users to enable use of Linux "hugepages" and to configure associated parameters.
The purpose of this would be to enable customers to take advantage of potential performance gains
that might be possible by making use of this feature.

### Related Issues

See the following github issues:
* https://github.com/harvester/harvester/issues/5006
* https://github.com/harvester/harvester/issues/5062

## Motivation

### Background

The Linux huge page support is a somewhat obscure feature whose
use and behavior is not obvious for most users;
therefore some explanation seems useful
before stating the specific things that we propose to do
with Harvester to enable use of huge pages.

#### What Huge Pages Are

The Linux kernel normally manages physical memory in units of the processor's smallest native page frame size.
For many processors, including `x86_64` and `arm64`, this is 4KiB.
This size is determined by the granularity of the processor MMU's page table entry.
For modern `x86_64` processors, the memory management hardware also supports
page sizes of 2Mib and 1GiB.

Modern `arm64` processors support 16KiB and 64KiB in addition to the base 4KiB page size.

The Linux kernel has the ability to make use of these larger units of memory.
In the linux kernel, these larger pages are referred to as "huge pages".

#### Benefits and Drawbacks of Huge Pages

Using a larger page size removes one or more layers from the page table hierarchy,
meaning that the processor has to load fewer page table entries to learn
the mapping of a given virtual address to its underlying hardware address.
This means fewer "overhead" memory accesses to fetch page table entries,
and less space occupied in the processor translation lookaside buffer (TLB).

For _some_ workloads this can be of significant performance benefit.

The flip side of this is that using larger page sizes means
the kernel does not have as small a granularity of allocation
for either virtual or physical memory.
This means that the kernel has less flexibility
when allocating memory, and can result in less efficient use of system memory.

For _some_ workloads, this can result in significantly _lower_ performance.

There is no simple formula for knowing which should be used for best results.
The kernel has no algorithm for determining the right choice dynamically.
Some application vendors do have recommendations, based on reference workloads.

The reality is, however, that the "best answer" will depend heavily
on the specific workload of a specific application mix in a specific situation.

#### Huge Pages in Harvester

The Linux kernel has the ability to use this,
and Linux kernel builds normally used in Harvester have the associated features
enabled at compile time,
but _use_ of the features is disabled by default
so that normally only 4KiB pages are used.

Some users and potential users of Harvester have requested the ability
to turn on use of this feature.

It has also been claimed that the Longhorn storage system could benefit
from the availability of this feature.

Given that the kernel itself has the ability to support this,
it seems reasonable to provide a way for users to enable its use.

### Linux Parameters and Features

There are several aspects to the Linux support for hugepages:
1. Allocation of RAM for hugepage use.
2. Interfaces for programs to make explicit use of hugepages.
3. A Kernel facility to make implicit use of hugepages.

Each of these is discussed briefly below.

Further details of hugepage support may be found in Linux man pages and Linux kernel documentation
as listed at the end of this document.

#### Allocation of RAM for hugepage use

Normally, all allocatable RAM is managed in the uniform standard page size.
For x86_64 and ARM architectures, this unit is 4KiB.
This is the smallest size page that the hardware MMU supports
in its page tables.

To make use of the larger page sizes,
the kernel must be told to set aside some amount of RAM
for that purpose.

#### Interfaces for programs to make explicit use of hugepages



### Goals

The goals of this enhancement are:

* To provide a means for users to configure hugepages on a node, on a harvester cluster as a whole, and for individual VMs
* To provide a means for users to specify associated parameters
* To provide basic sanity checks on parameter values (which page size(s) to use, how many huge pages to preallocate).
    * Page sizes will only be checked to ensure they are valid choices for the machine.
	* Numbers of huge pages will only be checked for validity and that they are within the hardware limits of the machine.
* To provide _enablement_ in the hypervisor for use of the hugepages feature, should VMs or Harvester components (like Longhorn) choose to use it.

The intent will be to provide access to the underlying configuration mechanisms
so that users who understand their potential benefits and consequences can make use of them.

### Non-goals [optional]

It is **not** a goal of this enhancement to do any of the following:

* When performing sanity checking of parameter values, to validate that choices made are actually beneficial.
* To automatically default to a "best choice" configuration.
* To recommend a "best "choice" configuration.
* To apply configuration change to guest OSes running *inside VMs* so that they make use of hugepages.

It will be up to the users to determine for themselves whether or not to use Linux "hugepages"
and what associated parameter values to choose.

It will be up to the administrators of the OSes in each individual VM to configure their software to use huge pages.

## Proposal

To provide hugepages support, the following approach is proposed, in two phases:

1. Host OS support
2. Virtual Machine support

Details of each phase are discussed below.

### Host OS Support

The Harvester installer program will be modified as follows
* At an appropriate stage, a dialog panel will be displayed asking the user whether they want to enable use of hugepages.  If so, then the user will be queried for values of relevant parameters:
    * For each hugepage size supported by the kernel on the system's platform, the user will be able to specify how many pages of that size should be preallocated.  The size chosen will be checked for basic sanity (for example, specifying numbers of pages that exceed the system's total RAM size) but no attempt will be made to check "how good" the choice is.
	* The user will be able to indicate which of the available hugepage sizes is the "default".
	* There will also be a box where the users can indicate whether they do or do not want the "transparent_hugepages" feature enabled.
	* The dialog will contain a disclaimer message indicating that
	    * The choices they make may _decrease_ system performance, and may even make the system unusuable.
		* The only way to know is to test specific settings with specific workloads.
	* The dialog may contain brief summaries of what the parameters do.
* TBD: a recovery method must be provided for the event where their parameter choices pass the sanity checks but make the system unusable.
* TBD: Question: should the Harvester UI provide a means for altering the settings for a node while the system is running?  I'm inclined to think there should be, so that the user will not have to completely re-install a node if they find the settings they chose have undesirable results.
* TBD: Question: should we force all nodes to have the same setting, or allow each node to have its own settings?
* TBD: Question: if nodes may have different settings, can we using tagging or some other means to allow the user to affect VM scheduling so that certain workloads can preferentially be run on a node with hugepages turned on (or conversely, to be scheduled on a node with hugepages turned off)?

The Harvester upgrader will be modified as follows, if necessary
* The upgrade procedure will preserve previously chosen hugepage settings
* TBD: Question: if "sanity" conditions change for valid parameters (e.g. if the base Harvester runtime increases its memory footprint), should the upgrade procedure do anything?
    * Should it adjust the parameters automatically (perhaps with a message)?
	* Should there be a dialog instead?

### Virtual Machine Support

The qemu command used to run VMs supports command line options that enable the running VM to have its pages backed by host hugepages.

Details TBD.


### User Stories

Detail the things that people will be able to do if this enhancement is implemented. A good practise is including a comparsion of what user cannot do before the enhancement implemented, why user would want an enhancement and what user need to do after, to make it clear why the enhancement beneficial to the user.

The experience details should be in the `User Experience In Detail` later.

#### Story 1

TBD

#### Story 2

TBD

### User Experience In Detail

Detail what user need to do to use this enhancement. Include as much detail as possible so that people can understand the "how" of the system. The goal here is to make this feel real for users without getting bogged down.

TBD

### API changes

TBD.  Probably extensions to configuration YAML structures to provide a place for specifying hugepages parameter values.

## Design

Rough sketch, details TBD.

For the first phase, there is a part which makes changes in the Harvester installer,
and there is a part which makes changes in the Harverster upgrade process to avoid erasing
the configuration choices applied in the installer.

The installer changes will have a dialog during installation,
at which point the user may choose to enable hugepages support.
If the user chooses to enable it, then values of certain parameters
will be requested, namely

* The amount of memory or number of pages to pre-allocate for each supported "hugepage" size.
    * For x86_64 systems, the choices would be 2MiB and 1GiB.
	* For ARM systems, the choices would be 16KiB and 64KiB.  (Verify this).
	* Note: the available choices for a given architecture can be queried at runtime, so we don't have to hard code knowledge of x86_64 and ARM
	* Basic sanity check(s) of the chosen values will be done, only to ensure that the values chosen are sane
* Whether or not to enable the "transparent hugepages" feature.
* A disclaimer should be displayed stating that poor choices of these parameter values can result in a system that performs poorly or may even be non-functional.

It will default to being disabled, in which case values for the specific parameters will not be requested.

The Harvester upgrade process will be modified to make sure that it does not overwrite parameter values set before the upgrade.

Probably, the dashboard UI should be enhanced to allow the user to change the parameter values without completely re-installing.

For the second phase, the dashboard UI should be enhanced to allow the user to enable hugepages use with individual VMs, and for each VM, values of parameters should be queried.  These values will be stored in the configuration YAML for the VM.  When starting a VM, these values will be passed through to kubevirt/libvirt in a suitable manner to "turn on" whatever QEMU/KVM features are required to allow the running VM to make use of this feature.

### Implementation Overview

Overview on how the enhancement will be implemented.

TBD

### Test plan

Integration test plan.

TBD

### Upgrade strategy

Anything that requires if user want to upgrade to this enhancement

TBD

## Notes

Additional notes.

### References

* Linux kernel document "HugeTLB Pages" Documentation/admin-guide/mm/hugetlbpage.rst
* Linux man page `mmap(2)`
* Linux man page `alloc_hugepages(2)`
* Linux man page `memfd_create(2)`
