# goiscsi
A portable Go module for iscsi related operations such as discovery and login

## Features
The following features are supported:
* Discover iSCSI targets provided by a specific portal, optionally log into each target
* Discover the iSCSI Initiators defined on the local system
* Log into a specific portal/target
* Log out of a specific portal/target
* Rescan all connected iSCSI sessions

## Implementation options
Two implementations of the `goiscsi.ISCSIinterface` exist; one is for Linux based systems and one is a mock
implementation. When instantiating an implementation of the `goiscsi.ISCSIinterface` interface, the factories 
accept a `map[string]string` option that allows the user to set specific key/values within the implementation.

The `goiscsi.ISCSIinterface` is defined as:
```go
type ISCSIinterface interface {
	// Discover the targets exposed via a given portal
	// returns an array of ISCSITarget instances
	DiscoverTargets(address string, login bool) ([]ISCSITarget, error)

	// Get a list of iSCSI initiators defined in a specified file
	// To use the system default file of "/etc/iscsi/initiatorname.iscsi", provide a filename of ""
	GetInitiators(filename string) ([]string, error)

	// Log into a specified target
	PerformLogin(target ISCSITarget) error

	// Log out of a specified target
	PerformLogout(target ISCSITarget) error

	// Rescan current iSCSI sessions
	PerformRescan() error

	// generic implementations
	isMock() bool
	getOptions() map[string]string
}
```

Many operations deal with iSCSI Targets via a type of `goiscsi.ISCSITarget`, defined as:
```go
type ISCSITarget struct {
	Portal   string
	GroupTag string
	Target   string
}
```

#### LinuxISCSI
When instantiating a Linux implementation via `goiscsi.NewLinuxISCSI` the following options are available

| Key                | Meaning                                                                                 |
|--------------------|-----------------------------------------------------------------------------------------|
| chrootDirectory    | Run `iscsiadm` in a chrooted environment with the root set to this value.               |
|                    | Default is to not chroot                                                                |

#### MockISCSI
When instantiating a mock implementation via `goiscsi.NewMockISCSI`, the follwoing options are available:

| Key                | Meaning                                                                                                   |
|--------------------|-----------------------------------------------------------------------------------------------------------|
| numberOfInitiators | Defines the number of initiators that will be returned via the `GetInitiators` method.<br/>Default is "1" |
| numberOfTargets    | Defines the number of targets that will be returned via the `DiscoverTargets` method.<br/>Default is "1"  |                                                                           

## Usage examples
The following example will instantiate a Linux based iSCSI client and Discover the targets exposed via the portal at `address`

```go
import (
    "errors"
    
    "github.com/dell/goiscsi"
)

func printTargets(address string) {
    var c goiscsi.ISCSIinterface
    c := goiscsi.NewLinuxISCSI(map[string]string{})
    targets, err := c.DiscoverTargets(address, false)
    if err != nil {
        return
    }
    for _, t := range targets {
        fmt.Printf("Found target: %s", tgt.Target)
    }
}
```

The following example will instantiate a Mock iSCSI client, set the number of targets to 3, and Discover the mocked targets

```go
import (
    "errors"
    
    "github.com/dell/goiscsi"
)

func printTargets(address string) {
    var c goiscsi.ISCSIinterface
    opts := make(map[string]string, 0)
    opts[goiscsi.MockNumberOfTargets] = "3"
    c := goiscsi.NewMockISCSI(opts)
    targets, err := c.DiscoverTargets(address, false)
    if err != nil {
        return
    }
    for _, t := range targets {
        fmt.Printf("Found target: %s", tgt.Target)
    }
}
```

