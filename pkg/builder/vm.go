package builder

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	defaultVMGenerateName = "harv-"
	defaultVMNamespace    = "default"

	defaultVMCPUCores = 1
	defaultVMMemory   = "256Mi"

	HarvesterAPIGroup                                     = "harvesterhci.io"
	LabelAnnotationPrefixHarvester                        = HarvesterAPIGroup + "/"
	LabelKeyVirtualMachineCreator                         = LabelAnnotationPrefixHarvester + "creator"
	LabelKeyVirtualMachineName                            = LabelAnnotationPrefixHarvester + "vmName"
	AnnotationKeyVirtualMachineSSHNames                   = LabelAnnotationPrefixHarvester + "sshNames"
	AnnotationKeyVirtualMachineWaitForLeaseInterfaceNames = LabelAnnotationPrefixHarvester + "waitForLeaseInterfaceNames"
	AnnotationKeyVirtualMachineDiskNames                  = LabelAnnotationPrefixHarvester + "diskNames"
	AnnotationKeyImageID                                  = LabelAnnotationPrefixHarvester + "imageId"

	AnnotationPrefixCattleField = "field.cattle.io/"
	LabelPrefixHarvesterTag     = "tag.harvesterhci.io/"
	AnnotationKeyDescription    = AnnotationPrefixCattleField + "description"
)

type VMBuilder struct {
	VirtualMachine             *kubevirtv1.VirtualMachine
	SSHNames                   []string
	WaitForLeaseInterfaceNames []string
}

func NewVMBuilder(creator string) *VMBuilder {
	vmLabels := map[string]string{
		LabelKeyVirtualMachineCreator: creator,
	}
	objectMeta := metav1.ObjectMeta{
		Namespace:    defaultVMNamespace,
		GenerateName: defaultVMGenerateName,
		Labels:       vmLabels,
		Annotations:  map[string]string{},
	}
	runStrategy := kubevirtv1.RunStrategyHalted
	cpu := &kubevirtv1.CPU{
		Cores: defaultVMCPUCores,
	}
	resources := kubevirtv1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(defaultVMMemory),
			corev1.ResourceCPU:    *resource.NewQuantity(defaultVMCPUCores, resource.DecimalSI),
		},
	}
	template := &kubevirtv1.VirtualMachineInstanceTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: vmLabels,
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				CPU: cpu,
				Devices: kubevirtv1.Devices{
					Disks:      []kubevirtv1.Disk{},
					Interfaces: []kubevirtv1.Interface{},
				},
				Resources: resources,
			},
			Affinity: &corev1.Affinity{},
			Networks: []kubevirtv1.Network{},
			Volumes:  []kubevirtv1.Volume{},
		},
	}

	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: objectMeta,
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &runStrategy,
			Template:    template,
		},
	}
	return &VMBuilder{
		VirtualMachine:             vm,
		SSHNames:                   []string{},
		WaitForLeaseInterfaceNames: []string{},
	}
}

func (v *VMBuilder) Name(name string) *VMBuilder {
	v.VirtualMachine.ObjectMeta.Name = name
	v.VirtualMachine.ObjectMeta.GenerateName = ""
	v.VirtualMachine.Spec.Template.ObjectMeta.Labels[LabelKeyVirtualMachineName] = name
	return v
}

func (v *VMBuilder) Namespace(namespace string) *VMBuilder {
	v.VirtualMachine.ObjectMeta.Namespace = namespace
	return v
}

func (v *VMBuilder) MachineType(machineType string) *VMBuilder {
	v.VirtualMachine.Spec.Template.Spec.Domain.Machine = &kubevirtv1.Machine{
		Type: machineType,
	}
	return v
}

func (v *VMBuilder) HostName(hostname string) *VMBuilder {
	v.VirtualMachine.Spec.Template.Spec.Hostname = hostname
	return v
}

func (v *VMBuilder) Description(description string) *VMBuilder {
	if v.VirtualMachine.ObjectMeta.Annotations == nil {
		v.VirtualMachine.ObjectMeta.Annotations = map[string]string{}
	}
	v.VirtualMachine.ObjectMeta.Annotations[AnnotationKeyDescription] = description
	return v
}

func (v *VMBuilder) Labels(labels map[string]string) *VMBuilder {
	if v.VirtualMachine.ObjectMeta.Labels == nil {
		v.VirtualMachine.ObjectMeta.Labels = labels
	}
	for key, value := range labels {
		v.VirtualMachine.ObjectMeta.Labels[key] = value
	}
	return v
}

func (v *VMBuilder) Annotations(annotations map[string]string) *VMBuilder {
	if v.VirtualMachine.ObjectMeta.Annotations == nil {
		v.VirtualMachine.ObjectMeta.Annotations = annotations
	}
	for key, value := range annotations {
		v.VirtualMachine.ObjectMeta.Annotations[key] = value
	}
	return v
}

func (v *VMBuilder) Memory(memory string) *VMBuilder {
	if len(v.VirtualMachine.Spec.Template.Spec.Domain.Resources.Limits) == 0 {
		v.VirtualMachine.Spec.Template.Spec.Domain.Resources.Limits = corev1.ResourceList{}
	}
	v.VirtualMachine.Spec.Template.Spec.Domain.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(memory)
	return v
}

func (v *VMBuilder) CPU(cores int) *VMBuilder {
	v.VirtualMachine.Spec.Template.Spec.Domain.CPU.Cores = uint32(cores)
	if len(v.VirtualMachine.Spec.Template.Spec.Domain.Resources.Limits) == 0 {
		v.VirtualMachine.Spec.Template.Spec.Domain.Resources.Limits = corev1.ResourceList{}
	}
	v.VirtualMachine.Spec.Template.Spec.Domain.Resources.Limits[corev1.ResourceCPU] = *resource.NewQuantity(int64(cores), resource.DecimalSI)
	return v
}

func (v *VMBuilder) EvictionStrategy(liveMigrate bool) *VMBuilder {
	if liveMigrate {
		evictionStrategy := kubevirtv1.EvictionStrategyLiveMigrate
		v.VirtualMachine.Spec.Template.Spec.EvictionStrategy = &evictionStrategy
	}
	return v
}

func (v *VMBuilder) Affinity(affinity *corev1.Affinity) *VMBuilder {
	if affinity == nil {
		return v.DefaultPodAntiAffinity()
	}

	v.VirtualMachine.Spec.Template.Spec.Affinity = affinity
	return v
}

func (v *VMBuilder) DefaultPodAntiAffinity() *VMBuilder {
	podAffinityTerm := corev1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      LabelKeyVirtualMachineCreator,
					Operator: metav1.LabelSelectorOpExists,
				},
			},
		},
		TopologyKey: corev1.LabelHostname,
	}
	return v.PodAntiAffinity(podAffinityTerm, true, 100)
}

func (v *VMBuilder) PodAntiAffinity(podAffinityTerm corev1.PodAffinityTerm, soft bool, weight int32) *VMBuilder {
	podAffinity := &corev1.PodAntiAffinity{}
	if soft {
		podAffinity.PreferredDuringSchedulingIgnoredDuringExecution = []corev1.WeightedPodAffinityTerm{
			{
				Weight:          weight,
				PodAffinityTerm: podAffinityTerm,
			},
		}
	} else {
		podAffinity.RequiredDuringSchedulingIgnoredDuringExecution = []corev1.PodAffinityTerm{
			podAffinityTerm,
		}
	}
	v.VirtualMachine.Spec.Template.Spec.Affinity.PodAntiAffinity = podAffinity
	return v
}

func (v *VMBuilder) Run(start bool) *VMBuilder {
	runStrategy := kubevirtv1.RunStrategyHalted
	if start {
		runStrategy = kubevirtv1.RunStrategyRerunOnFailure
	}
	v.VirtualMachine.Spec.RunStrategy = &runStrategy
	return v
}

func (v *VMBuilder) RunStrategy(runStrategy kubevirtv1.VirtualMachineRunStrategy) *VMBuilder {
	v.VirtualMachine.Spec.RunStrategy = &runStrategy
	return v
}

func (v *VMBuilder) VM() (*kubevirtv1.VirtualMachine, error) {
	if v.VirtualMachine.Spec.Template.ObjectMeta.Annotations == nil {
		v.VirtualMachine.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	sshNames, err := json.Marshal(v.SSHNames)
	if err != nil {
		return nil, err
	}
	v.VirtualMachine.Spec.Template.ObjectMeta.Annotations[AnnotationKeyVirtualMachineSSHNames] = string(sshNames)

	waitForLeaseInterfaceNames, err := json.Marshal(v.WaitForLeaseInterfaceNames)
	if err != nil {
		return nil, err
	}
	v.VirtualMachine.Spec.Template.ObjectMeta.Annotations[AnnotationKeyVirtualMachineWaitForLeaseInterfaceNames] = string(waitForLeaseInterfaceNames)

	return v.VirtualMachine, nil
}

func (v *VMBuilder) Update(vm *kubevirtv1.VirtualMachine) *VMBuilder {
	v.VirtualMachine = vm
	return v
}
