package v1alpha1

// FleetYAML is the top-level structure of the fleet.yaml file.
// The fleet.yaml file adds options to a bundle. Any directory with a
// fleet.yaml is automatically turned into a bundle.
type FleetYAML struct {
	// Name of the bundle which will be created.
	Name string `json:"name,omitempty"`
	// Labels are copied to the bundle and can be used in a
	// dependsOn.selector.
	Labels map[string]string `json:"labels,omitempty"`
	BundleSpec
	// TargetCustomizations are used to determine how resources should be
	// modified per target. Targets are evaluated in order and the first
	// one to match a cluster is used for that cluster.
	TargetCustomizations []BundleTarget `json:"targetCustomizations,omitempty"`
	// ImageScans are optional and used to update container image
	// references in the git repo.
	ImageScans []ImageScanYAML `json:"imageScans,omitempty"`
	// OverrideTargets overrides targets that are defined in the GitRepo
	// resource. If overrideTargets is provided the bundle will not inherit
	// targets from the GitRepo.
	OverrideTargets []GitTarget `json:"overrideTargets,omitempty"`
}

// ImageScanYAML is a single entry in the ImageScan list from fleet.yaml.
type ImageScanYAML struct {
	// Name of the image scan. Unused.
	Name string `json:"name,omitempty"`
	ImageScanSpec
}
