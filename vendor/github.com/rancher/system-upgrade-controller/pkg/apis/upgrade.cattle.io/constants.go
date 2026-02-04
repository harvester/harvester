package upgrade

import "path"

const (
	// AnnotationTTLSecondsAfterFinished is used to store a fallback value for job.spec.ttlSecondsAfterFinished
	AnnotationTTLSecondsAfterFinished = GroupName + `/ttl-seconds-after-finished`

	// AnnotationIncludeInDigest is used to determine parts of the plan to include in the hash for upgrading
	// The value should be a comma-delimited string corresponding to the sections of the plan.
	// For example, a value of "spec.concurrency,spec.upgrade.envs" will include
	// spec.concurrency and spec.upgrade.envs from the plan in the hash to track for upgrades.
	AnnotationIncludeInDigest = GroupName + `/digest`

	// LabelController is the name of the upgrade controller.
	LabelController = GroupName + `/controller`

	// LabelExclusive is set if the plan should not run concurrent with other plans.
	LabelExclusive = GroupName + `/exclusive`

	// LabelNode is the node being upgraded.
	LabelNode = GroupName + `/node`

	// LabelPlan is the plan being applied.
	LabelPlan = GroupName + `/plan`

	// LabelVersion is the version of the plan being applied.
	LabelVersion = GroupName + `/version`

	// LabelPlanSuffix is used for composing labels specific to a plan.
	LabelPlanSuffix = `plan.` + GroupName
)

func LabelPlanName(name string) string {
	return path.Join(LabelPlanSuffix, name)
}
