package util

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ActionPermission is the Kubernetes permission a user needs to run a Harvester API action.
// When Subresource is set, the check applies to the action's target object
// (for example, update on virtualmachines/start for the VM named in the request).
// When Subresource is empty, the check applies to Resource in the target's namespace
// (for example, create on virtualmachinebackups).
type ActionPermission struct {
	Verb        string
	Group       string
	Resource    string
	Subresource string
}

// AccessSetResource returns the resource string used by RBAC rules and the steve AccessSet,
// for example "virtualmachines/start".
func (p ActionPermission) AccessSetResource() string {
	if p.Subresource == "" {
		return p.Resource
	}
	return p.Resource + "/" + p.Subresource
}

// TargetsObject reports whether the permission is checked against the action's target object name.
func (p ActionPermission) TargetsObject() bool {
	return p.Subresource != ""
}

const (
	kubevirtSubresourcesGroup = "subresources.kubevirt.io"
	kubevirtGroup             = "kubevirt.io"
)

var virtualMachineGR = schema.GroupResource{Group: kubevirtGroup, Resource: "virtualmachines"}

// actionPermissions maps API actions to the permission that authorizes them, instead of
// requiring update on the whole resource. Actions not listed here keep requiring update
// on the target resource. The mapping follows the KubeVirt subresource permissions in
// the kubevirt.io:edit cluster role, so a role can grant, for example, power operations
// without granting edits to the VM spec.
var actionPermissions = map[schema.GroupResource]map[string]ActionPermission{
	virtualMachineGR: {
		"start":               {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachines", Subresource: "start"},
		"stop":                {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachines", Subresource: "stop"},
		"forceStop":           {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachines", Subresource: "stop"},
		"restart":             {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachines", Subresource: "restart"},
		"softreboot":          {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachineinstances", Subresource: "softreboot"},
		"pause":               {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachineinstances", Subresource: "pause"},
		"unpause":             {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachineinstances", Subresource: "unpause"},
		"addVolume":           {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachines", Subresource: "addvolume"},
		"removeVolume":        {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachines", Subresource: "removevolume"},
		"insertCdRomVolume":   {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachines", Subresource: "addvolume"},
		"ejectCdRomVolume":    {Verb: VerbUpdate, Group: kubevirtSubresourcesGroup, Resource: "virtualmachines", Subresource: "removevolume"},
		"migrate":             {Verb: VerbCreate, Group: kubevirtGroup, Resource: "virtualmachineinstancemigrations"},
		"findMigratableNodes": {Verb: VerbCreate, Group: kubevirtGroup, Resource: "virtualmachineinstancemigrations"},
		"abortMigration":      {Verb: VerbDelete, Group: kubevirtGroup, Resource: "virtualmachineinstancemigrations"},
		"backup":              {Verb: VerbCreate, Group: "harvesterhci.io", Resource: "virtualmachinebackups"},
		"snapshot":            {Verb: VerbCreate, Group: "harvesterhci.io", Resource: "virtualmachinebackups"},
	},
}

// LookupActionPermission returns the permission that authorizes the given action on the given
// resource. ok is false when the action has no dedicated permission and requires update on
// the resource.
func LookupActionPermission(gr schema.GroupResource, action string) (ActionPermission, bool) {
	p, ok := actionPermissions[gr][action]
	return p, ok
}
