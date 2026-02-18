# Customize Support Bundle File Name

## Summary

Currently, support bundle file names follow the format `supportbundle_c2eb6186-525f-411e-91ee-e88ed090d66d_2025-09-17T03-39-17Z`. For users managing multiple clusters, these auto-generated names make it difficult to identify and distinguish support bundles when reporting issues.

### Related Issues

https://github.com/harvester/harvester/issues/9257

## Motivation

### Goals

- Make support bundle files easily recognizable

### Non-goals [optional]

- Define a cluster name (https://github.com/harvester/harvester/issues/4738). 
  Defining a display name for the support bundle is a more straightforward solution for this issue.

## Proposal

### User Stories

Users can specify a display name when creating a support bundle. For example, if a user inputs the display name `testing`, the support bundle file will be named `supportbundle_testing_2025-09-17T03-39-17Z`. The naming template is `supportbundle_{display_name}_{time}`. If no display name is provided, the file will use a randomly generated name as before.

### API changes

#### Support Bundle CR

A new field will be added to the CR:

```go
type SupportBundleSpec struct {
    // +optional
	DisplayName string `json:"displayName"`
}
```

#### Frontend

The frontend will create a new optional input box for the display name.

## Design

### Implementation Overview

The implementation is straightforward:

- Users input a display name via the frontend.
- The backend passes the display name to the support bundle manager.


### Test Plan

- When a display name is provided, the support bundle file name should contain the display name.
- When no display name is provided, the file name should follow the previous naming convention.

### Upgrade strategy

None.

## Note [optional]

None.
