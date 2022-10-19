package data

import (
	"bytes"
	"text/template"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

var (
	templateTmpl        = template.Must(template.New("template").Parse(initBaseTemplates))
	templateVersionTmpl = template.Must(template.New("templateVersion").Parse(initBaseTemplateVersions))
)

func createTemplates(mgmt *config.Management, namespace string) error {
	templates := mgmt.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate()
	templateVersions := mgmt.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion()
	if err := initBaseTemplate(templates, namespace); err != nil {
		return err
	}

	return initBaseTemplateVersion(templateVersions, namespace)
}

func generateYmls(tmpl *template.Template, namespace string) ([][]byte, error) {
	data := map[string]string{
		"Namespace": namespace,
	}

	templateBuffer := bytes.NewBuffer(nil)
	if err := tmpl.Execute(templateBuffer, data); err != nil {
		return nil, err
	}

	return bytes.Split(templateBuffer.Bytes(), []byte("\n---\n")), nil
}

func initBaseTemplate(vmTemplates ctlharvesterv1.VirtualMachineTemplateClient, namespace string) error {
	ymls, err := generateYmls(templateTmpl, namespace)
	if err != nil {
		return err
	}

	for _, yml := range ymls {
		var vmTemplate harvesterv1.VirtualMachineTemplate
		if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yml), 1024).Decode(&vmTemplate); err != nil {
			return errors.Wrap(err, "Failed to convert virtualMachineTemplate from yaml to object")
		}

		if _, err := vmTemplates.Create(&vmTemplate); err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "Failed to create virtualMachineTemplate %s/%s", vmTemplate.Namespace, vmTemplate.Name)
		}
	}
	return nil
}

func initBaseTemplateVersion(vmTemplateVersions ctlharvesterv1.VirtualMachineTemplateVersionClient, namespace string) error {
	ymls, err := generateYmls(templateVersionTmpl, namespace)
	if err != nil {
		return err
	}

	for _, yml := range ymls {
		var vmTemplateVersion harvesterv1.VirtualMachineTemplateVersion
		if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yml), 1024).Decode(&vmTemplateVersion); err != nil {
			return errors.Wrap(err, "Failed to convert virtualMachineTemplateVersion from yaml to object")
		}

		if _, err := vmTemplateVersions.Create(&vmTemplateVersion); err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "Failed to create virtualMachineTemplateVersion %s/%s", vmTemplateVersion.Namespace, vmTemplateVersion.Name)
		}
	}
	return nil
}

var (
	initBaseTemplates = `
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineTemplate
metadata:
  name: iso-image-base-template
  namespace: {{ .Namespace }}
spec:
  description: Template for booting the virtual machine from an ISO image
---
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineTemplate
metadata:
  name: raw-image-base-template
  namespace: {{ .Namespace }}
spec:
  description: Template for booting the virtual machine from a qcow2/raw image
---
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineTemplate
metadata:
  name: windows-raw-image-base-template
  namespace: {{ .Namespace }}
spec:
  description: Template for booting the virtual machine from a windows qcow2/raw image
---
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineTemplate
metadata:
  name: windows-iso-image-base-template
  namespace: {{ .Namespace }}
spec:
  description: Template for booting the Windows virtual machine from an ISO image
`

	// windows default resource request refer to windows server docs https://docs.microsoft.com/en-us/windows-server/get-started-19/sys-reqs-19
	initBaseTemplateVersions = `
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineTemplateVersion
metadata:
  name: iso-image-base-version
  namespace: {{ .Namespace }}
spec:
  templateId: {{ .Namespace }}/iso-image-base-template
  vm:
    metadata:
      annotations:
        harvesterhci.io/volumeClaimTemplates: |-
          [{
            "metadata": {
              "name": "pvc-cdrom-disk",
              "annotations": {
                "harvesterhci.io/imageId": ""
              }
            },
            "spec":{
              "accessModes": ["ReadWriteMany"],
              "resources":{
                "requests":{
                  "storage": "10Gi"
                }
              },
              "volumeMode": "Block"
            }
          },
          {
            "metadata": {
              "name": "pvc-rootdisk"
            },
            "spec":{
              "accessModes": ["ReadWriteMany"],
              "resources":{
                "requests":{
                  "storage": "10Gi"
                }
              },
              "volumeMode": "Block"
            }
          }]
    spec:
      runStrategy: RerunOnFailure
      template:
        spec:
          evictionStrategy: LiveMigrate
          domain:
            features:
              acpi:
                enabled: true
            cpu:
              cores: 1
            devices:
              disks:
              - cdrom:
                  bus: sata
                  readonly: true
                name: cdrom-disk
                bootOrder: 2
              - disk:
                  bus: virtio
                name: rootdisk
                bootOrder: 1
              interfaces:
              - name: default
                masquerade: {}
                model: virtio
            resources:
              limits:
                memory: 2048Mi
                cpu: 1
          networks:
          - name: default
            pod: {}
          volumes:
          - persistentVolumeClaim:
              claimName: pvc-cdrom-disk
            name: cdrom-disk
          - persistentVolumeClaim:
              claimName: pvc-rootdisk
            name: rootdisk
          - name: cloudinitdisk
            cloudInitNoCloud:
                userData: |
                  #cloud-config
                  package_update: true
                  packages:
                    - qemu-guest-agent
                  runcmd:
                    - - systemctl
                      - enable
                      - --now
                      - qemu-guest-agent.service
---
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineTemplateVersion
metadata:
  name: raw-image-base-version
  namespace: {{ .Namespace }}
spec:
  templateId: {{ .Namespace }}/raw-image-base-template
  vm:
    metadata:
      annotations:
        harvesterhci.io/volumeClaimTemplates: |-
          [{
            "metadata": {
              "name": "pvc-rootdisk",
              "annotations": {
                "harvesterhci.io/imageId": ""
              }
            },
            "spec":{
              "accessModes": ["ReadWriteMany"],
              "resources":{
                "requests":{
                  "storage": "10Gi"
                }
              },
              "volumeMode": "Block"
            }
          }]
    spec:
      runStrategy: RerunOnFailure
      template:
        spec:
          evictionStrategy: LiveMigrate
          domain:
            features:
              acpi:
                enabled: true
            cpu:
              cores: 1
            devices:
              disks:
              - disk:
                  bus: virtio
                name: rootdisk
                bootOrder: 1
              interfaces:
              - name: default
                masquerade: {}
                model: virtio
            resources:
              limits:
                memory: 2048Mi
                cpu: 1
          networks:
          - name: default
            pod: {}
          volumes:
          - persistentVolumeClaim:
              claimName: pvc-rootdisk
            name: rootdisk
          - name: cloudinitdisk
            cloudInitNoCloud:
                userData: |
                  #cloud-config
                  package_update: true
                  packages:
                    - qemu-guest-agent
                  runcmd:
                    - - systemctl
                      - enable
                      - --now
                      - qemu-guest-agent.service
---
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineTemplateVersion
metadata:
  name: windows-raw-image-base-version
  namespace: {{ .Namespace }}
spec:
  templateId: {{ .Namespace }}/windows-raw-image-base-template
  vm:
    metadata:
      labels:
        harvesterhci.io/os: windows
      annotations:
        harvesterhci.io/reservedMemory: 256Mi
        harvesterhci.io/volumeClaimTemplates: |-
          [{
            "metadata": {
              "name": "pvc-rootdisk",
              "annotations": {
                "harvesterhci.io/imageId": ""
              }
            },
            "spec":{
              "accessModes": ["ReadWriteMany"],
              "resources":{
                "requests":{
                  "storage": "32Gi"
                }
              },
              "volumeMode": "Block"
            }
          }]
    spec:
      runStrategy: RerunOnFailure
      template:
        spec:
          evictionStrategy: LiveMigrate
          domain:
            features:
              acpi:
                enabled: true
            cpu:
              cores: 1
            devices:
              disks:
              - disk:
                  bus: virtio
                name: rootdisk
                bootOrder: 1
              interfaces:
              - name: default
                masquerade: {}
                model: virtio
            resources:
              limits:
                memory: 2048Mi
                cpu: 1
          networks:
          - name: default
            pod: {}
          volumes:
          - persistentVolumeClaim:
              claimName: pvc-rootdisk
            name: rootdisk
---
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineTemplateVersion
metadata:
  name: windows-iso-image-base-version
  namespace: {{ .Namespace }}
spec:
  templateId: {{ .Namespace }}/windows-iso-image-base-template
  vm:
    metadata:
      labels:
        harvesterhci.io/os: windows
      annotations:
        harvesterhci.io/reservedMemory: 256Mi
        harvesterhci.io/volumeClaimTemplates: |-
          [{
            "metadata": {
              "name": "pvc-cdrom-disk",
              "annotations": {
                "harvesterhci.io/imageId": ""
              }
            },
            "spec":{
              "accessModes": ["ReadWriteMany"],
              "resources":{
                "requests":{
                  "storage": "20Gi"
                }
              },
              "volumeMode": "Block"
            }
          },
          {
            "metadata": {
              "name": "pvc-rootdisk"
            },
            "spec":{
              "accessModes": ["ReadWriteMany"],
              "resources":{
                "requests":{
                  "storage": "32Gi"
                }
              },
              "volumeMode": "Block"
            }
          }]
    spec:
      runStrategy: RerunOnFailure
      template:
        spec:
          evictionStrategy: LiveMigrate
          domain:
            features:
              acpi:
                enabled: true
            cpu:
              cores: 1
            devices:
              disks:
              - cdrom:
                  bus: sata
                name: cdrom-disk
                bootOrder: 1
              - disk:
                  bus: virtio
                name: rootdisk
                bootOrder: 2
              - cdrom:
                  bus: sata
                name: virtio-container-disk
              interfaces:
              - name: default
                model: e1000
                masquerade: {}
              inputs:
              - bus: usb
                name: tablet
                type: tablet
            resources:
              limits:
                memory: 2048Mi
                cpu: 1
          networks:
          - name: default
            pod: {}
          volumes:
          - persistentVolumeClaim:
              claimName: pvc-cdrom-disk
            name: cdrom-disk
          - persistentVolumeClaim:
              claimName: pvc-rootdisk
            name: rootdisk
          - containerDisk:
              image: registry.suse.com/suse/vmdp/vmdp:2.5.3
              imagePullPolicy: IfNotPresent
            name: virtio-container-disk
`
)
