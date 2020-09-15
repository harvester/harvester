package template

import (
	"bytes"
	"text/template"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/yaml"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	ctlapisv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
)

var (
	templateTmpl        = template.Must(template.New("template").Parse(initBaseTemplates))
	templateVersionTmpl = template.Must(template.New("templateVersion").Parse(initBaseTemplateVersions))
)

func initData(vmTemplates ctlapisv1alpha1.VirtualMachineTemplateClient, vmTemplateVersions ctlapisv1alpha1.VirtualMachineTemplateVersionClient) error {
	if err := initBaseTemplate(vmTemplates); err != nil {
		return err
	}

	return initBaseTemplateVersion(vmTemplateVersions)
}

func generateYmls(tmpl *template.Template) ([][]byte, error) {
	data := map[string]string{
		"Namespace": config.Namespace,
	}

	templateBuffer := bytes.NewBuffer(nil)
	if err := tmpl.Execute(templateBuffer, data); err != nil {
		return nil, err
	}

	return bytes.Split(templateBuffer.Bytes(), []byte("\n---\n")), nil
}

func initBaseTemplate(vmTemplates ctlapisv1alpha1.VirtualMachineTemplateClient) error {
	ymls, err := generateYmls(templateTmpl)
	if err != nil {
		return err
	}

	for _, yml := range ymls {
		var vmTemplate apisv1alpha1.VirtualMachineTemplate
		if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yml), 1024).Decode(&vmTemplate); err != nil {
			return errors.Wrap(err, "Failed to convert virtualMachineTemplate from yaml to object")
		}

		if _, err := vmTemplates.Create(&vmTemplate); err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "Failed to create virtualMachineTemplate %s/%s", vmTemplate.Namespace, vmTemplate.Name)
		}
	}
	return nil
}

func initBaseTemplateVersion(vmTemplateVersions ctlapisv1alpha1.VirtualMachineTemplateVersionClient) error {
	ymls, err := generateYmls(templateVersionTmpl)
	if err != nil {
		return err
	}

	for _, yml := range ymls {
		var vmTemplateVersion apisv1alpha1.VirtualMachineTemplateVersion
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
apiVersion: harvester.cattle.io/v1alpha1
kind: VirtualMachineTemplate
metadata:
  name: iso-image-base-template
  namespace: {{ .Namespace }}
spec:
  description: Template for booting the virtual machine from an ISO image
---
apiVersion: harvester.cattle.io/v1alpha1
kind: VirtualMachineTemplate
metadata:
  name: raw-image-base-template
  namespace: {{ .Namespace }}
spec:
  description: Template for booting the virtual machine from a qcow2/raw image
---
apiVersion: harvester.cattle.io/v1alpha1
kind: VirtualMachineTemplate
metadata:
  name: windows-iso-image-base-template
  namespace: {{ .Namespace }}
spec:
  description: Template for booting the Windows virtual machine from an ISO image
`

	// windows default resource request refer to windows server docs https://docs.microsoft.com/en-us/windows-server/get-started-19/sys-reqs-19
	initBaseTemplateVersions = `
apiVersion: harvester.cattle.io/v1alpha1
kind: VirtualMachineTemplateVersion
metadata:
  name: iso-image-base-version
  namespace: {{ .Namespace }}
spec:
  templateId: {{ .Namespace }}:iso-image-base-template
  vm:
    running: true
    template:
      spec:
        domain:
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
          resources:
            requests:
              memory: 2048Mi
        networks:
        - name: default
          pod: {}
        volumes:
        - dataVolume:
            name: datavolume-cdrom-disk
          name: cdrom-disk
        - dataVolume:
            name: datavolume-rootdisk
          name: rootdisk
    dataVolumeTemplates:
    - metadata:
        name: datavolume-cdrom-disk
      spec:
        pvc:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
        source:
          http:
            url: ""
    - metadata:
        name: datavolume-rootdisk
      spec:
        pvc:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
        source:
          blank: {}
---
apiVersion: harvester.cattle.io/v1alpha1
kind: VirtualMachineTemplateVersion
metadata:
  name: raw-image-base-version
  namespace: {{ .Namespace }}
spec:
  templateId: {{ .Namespace }}:raw-image-base-template
  vm:
    running: true
    template:
      spec:
        domain:
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
          resources:
            requests:
              memory: 2048Mi
        networks:
        - name: default
          pod: {}
        volumes:
        - dataVolume:
            name: datavolume-rootdisk
          name: rootdisk
    dataVolumeTemplates:
    - metadata:
        name: datavolume-rootdisk
      spec:
        pvc:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
        source:
          http:
            url: ""
---
apiVersion: harvester.cattle.io/v1alpha1
kind: VirtualMachineTemplateVersion
metadata:
  name: windows-iso-image-base-version
  namespace: {{ .Namespace }}
spec:
  templateId: {{ .Namespace }}:windows-iso-image-base-template
  vm:
    running: true
    template:
      spec:
        domain:
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
            requests:
              memory: 2048Mi
        networks:
        - name: default
          pod: {}
        volumes:
        - dataVolume:
            name: datavolume-cdrom-disk
          name: cdrom-disk
        - dataVolume:
            name: datavolume-rootdisk
          name: rootdisk
        - containerDisk:
            image: kubevirt/virtio-container-disk
          name: virtio-container-disk
    dataVolumeTemplates:
    - metadata:
        name: datavolume-cdrom-disk
      spec:
        pvc:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 20Gi
        source:
          http:
            url: ""
    - metadata:
        name: datavolume-rootdisk
      spec:
        pvc:
          accessModes:
          - ReadWriteOnce
          resources:
            requests:
              storage: 32Gi
        source:
          blank: {}
`
)
