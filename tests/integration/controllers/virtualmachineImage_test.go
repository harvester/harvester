package controllers

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

// tests to verify immutability of VirtualMachineImage leveraging custom BackingImageName
// functionality. The validation is performed by openapi schema checks.
// as a result this needs a real apiserver

const (
	// all vmimage objects created in the test use these key/values to help
	// speed up cleanup of vmimage objects only used by this test
	testImageKey   = "test-image-key"
	testImageValue = "test-image-value"
	defaultNS      = "default"
)

var _ = ginkgo.Describe("verify helm chart is create and addon gets to desired state", func() {
	var vmImageController ctlharvesterv1.VirtualMachineImageController

	ginkgo.BeforeEach(func() {
		// Initialize the vmImageController before each test
		vmImageController = scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	})

	ginkgo.AfterEach(func() {
		// Cleanup all VirtualMachineImage objects created during the test
		vmImageList, err := vmImageController.List(defaultNS, metav1.ListOptions{
			LabelSelector: testImageKey + "=" + testImageValue,
		})
		if err == nil {
			for _, vmImage := range vmImageList.Items {
				_ = vmImageController.Delete(vmImage.Namespace, vmImage.Name, &metav1.DeleteOptions{})
			}
		}
	})

	ginkgo.It("verify backingimage validation", func() {

		ginkgo.By("test cdi image failure when specifying BackingImageName", func() {
			vmImage := &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cdi-image",
					Namespace: defaultNS,
					Labels: map[string]string{
						testImageKey: testImageValue,
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType:       v1beta1.VirtualMachineImageSourceTypeDownload,
					URL:              "http://example.com/test-image.iso",
					BackingImageName: "test-backing-image",
					Backend:          v1beta1.VMIBackendCDI,
				},
			}

			_, err := vmImageController.Create(vmImage)
			gomega.Expect(err).To(gomega.HaveOccurred())
		})

		ginkgo.By("test backingimage failure when trying to update BackingImageName", func() {
			vmImage := &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-image",
					Namespace: defaultNS,
					Labels: map[string]string{
						testImageKey: testImageValue,
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType:       v1beta1.VirtualMachineImageSourceTypeDownload,
					URL:              "http://example.com/test-image.iso",
					BackingImageName: "test-backing-image",
					Backend:          v1beta1.VMIBackendBackingImage,
				},
			}

			vmImageObj, err := vmImageController.Create(vmImage)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			vmImageObj.Spec.BackingImageName = "updated-backing-image"
			_, err = vmImageController.Update(vmImageObj)
			gomega.Expect(err).To(gomega.HaveOccurred())

		})

		ginkgo.By("test backingimage failure when trying to add BackingImageName", func() {
			vmImage := &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-image-no-backing-image",
					Namespace: defaultNS,
					Labels: map[string]string{
						testImageKey: testImageValue,
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType: v1beta1.VirtualMachineImageSourceTypeDownload,
					URL:        "http://example.com/test-image.iso",
					Backend:    v1beta1.VMIBackendBackingImage,
				},
			}

			vmImageObj, err := vmImageController.Create(vmImage)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			vmImageObj.Spec.BackingImageName = "updated-backing-image"
			_, err = vmImageController.Update(vmImageObj)
			gomega.Expect(err).To(gomega.HaveOccurred())

		})

		ginkgo.By("test backingimage failure when trying to remove BackingImageName", func() {
			vmImage := &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-image-with-backing-image",
					Namespace: defaultNS,
					Labels: map[string]string{
						testImageKey: testImageValue,
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType:       v1beta1.VirtualMachineImageSourceTypeDownload,
					URL:              "http://example.com/test-image.iso",
					Backend:          v1beta1.VMIBackendBackingImage,
					BackingImageName: "test-backing-image",
				},
			}

			vmImageObj, err := vmImageController.Create(vmImage)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			vmImageObj.Spec.BackingImageName = ""
			_, err = vmImageController.Update(vmImageObj)
			gomega.Expect(err).To(gomega.HaveOccurred())

		})

		ginkgo.By("test backingimage failure when BackingImageName exceeds 40 characters", func() {
			vmImage := &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-image-with-long-backing-image-name",
					Namespace: defaultNS,
					Labels: map[string]string{
						testImageKey: testImageValue,
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType:       v1beta1.VirtualMachineImageSourceTypeDownload,
					URL:              "http://example.com/test-image.iso",
					Backend:          v1beta1.VMIBackendBackingImage,
					BackingImageName: "this-is-a-very-long-backing-image-name-exceeding-40-characters",
				},
			}

			_, err := vmImageController.Create(vmImage)
			gomega.Expect(err).To(gomega.HaveOccurred())

		})

		ginkgo.By("test backingimage failure when BackingImageName is not DNS1123 compliant", func() {
			vmImage := &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-image-with-invalid-backing-image-name",
					Namespace: defaultNS,
					Labels: map[string]string{
						testImageKey: testImageValue,
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType:       v1beta1.VirtualMachineImageSourceTypeDownload,
					URL:              "http://example.com/test-image.iso",
					Backend:          v1beta1.VMIBackendBackingImage,
					BackingImageName: "Invalid_Backing_Image_Name",
				},
			}

			_, err := vmImageController.Create(vmImage)
			gomega.Expect(err).To(gomega.HaveOccurred())

		})
	})

})
