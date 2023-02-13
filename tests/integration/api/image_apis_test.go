package api_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

var _ = Describe("verify image APIs", func() {

	var imageNamespace string

	BeforeEach(func() {

		imageNamespace = "default"

	})

	Context("operate via steve API", func() {

		var imageAPI string

		BeforeEach(func() {

			imageAPI = helper.BuildAPIURL("v1", "harvesterhci.io.virtualmachineimages", options.HTTPSListenPort)

		})

		Specify("verify required fields", func() {

			By("create an image with empty display name", func() {

				var image = harvesterv1.VirtualMachineImage{
					ObjectMeta: v1.ObjectMeta{
						GenerateName: "image-",
						Namespace:    imageNamespace,
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						SourceType: harvesterv1.VirtualMachineImageSourceTypeDownload,
						URL:        "http://harvesterhci.io/test.img",
					},
				}
				respCode, respBody, err := helper.PostObject(imageAPI, image)
				MustRespCodeIs(http.StatusUnprocessableEntity, "post image", err, respCode, respBody)
			})

			By("create an image with empty source type", func() {
				var (
					imageDisplayName = fuzz.String(5)
					image            = harvesterv1.VirtualMachineImage{
						ObjectMeta: v1.ObjectMeta{
							GenerateName: "image-",
							Namespace:    imageNamespace,
						},
						Spec: harvesterv1.VirtualMachineImageSpec{
							DisplayName: imageDisplayName,
							URL:         "http://harvesterhci.io/test.img",
						},
					}
				)
				respCode, respBody, err := helper.PostObject(imageAPI, image)
				MustRespCodeIs(http.StatusUnprocessableEntity, "post image", err, respCode, respBody)
			})
		})

		Specify("verify image fields set", func() {

			var (
				imageName        = fuzz.String(5)
				imageDisplayName = fuzz.String(5)
				image            = harvesterv1.VirtualMachineImage{
					ObjectMeta: v1.ObjectMeta{
						Name:      imageName,
						Namespace: imageNamespace,
						Labels: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Description: "test description",
						DisplayName: imageDisplayName,
						SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
						URL:         "http://harvesterhci.io/test.img",
					},
				}

				getImageURL = fmt.Sprintf("%s/%s/%s", imageAPI, imageNamespace, imageName)
				retImage    harvesterv1.VirtualMachineImage
			)

			By("create image", func() {
				respCode, respBody, err := helper.PostObject(imageAPI, image)
				MustRespCodeIs(http.StatusCreated, "post image", err, respCode, respBody)
			})

			By("verify image fields matching", func() {
				image.Spec.StorageClassParameters = util.GetImageDefaultStorageClassParameters()
				respCode, respBody, err := helper.GetObject(getImageURL, &retImage)
				MustRespCodeIs(http.StatusOK, "get image", err, respCode, respBody)
				Expect(retImage.Labels).To(BeEquivalentTo(image.Labels))
				Expect(retImage.Annotations).To(BeEquivalentTo(image.Annotations))
				Expect(retImage.Spec).To(BeEquivalentTo(image.Spec))
			})
		})

		Specify("verify image fields set by yaml", func() {

			var (
				imageName        = fuzz.String(5)
				imageDisplayName = fuzz.String(5)
				image            = harvesterv1.VirtualMachineImage{
					ObjectMeta: v1.ObjectMeta{
						Name:      imageName,
						Namespace: imageNamespace,
						Labels: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Description: "test description",
						DisplayName: imageDisplayName,
						SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
						URL:         "http://harvesterhci.io/test.img",
					},
				}

				getImageURL = fmt.Sprintf("%s/%s/%s", imageAPI, imageNamespace, imageName)
				retImage    harvesterv1.VirtualMachineImage
			)

			By("create image", func() {
				respCode, respBody, err := helper.PostObjectByYAML(imageAPI, image)
				MustRespCodeIs(http.StatusCreated, "post image", err, respCode, respBody)
			})

			By("verify image fields matching", func() {
				image.Spec.StorageClassParameters = util.GetImageDefaultStorageClassParameters()
				respCode, respBody, err := helper.GetObject(getImageURL, &retImage)
				MustRespCodeIs(http.StatusOK, "get image", err, respCode, respBody)
				Expect(retImage.Labels).To(BeEquivalentTo(image.Labels))
				Expect(retImage.Annotations).To(BeEquivalentTo(image.Annotations))
				Expect(retImage.Spec).To(BeEquivalentTo(image.Spec))
			})
		})

		Specify("verify update and delete images", func() {
			var (
				imageName        = fuzz.String(5)
				imageDisplayName = fuzz.String(5)

				image = harvesterv1.VirtualMachineImage{
					ObjectMeta: v1.ObjectMeta{
						Name:      imageName,
						Namespace: imageNamespace,
						Labels: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Description: "test description",
						DisplayName: imageDisplayName,
						SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
						URL:         "http://harvesterhci.io/test.img",
					},
				}

				toUpdateImage = harvesterv1.VirtualMachineImage{
					ObjectMeta: v1.ObjectMeta{
						Name:      imageName,
						Namespace: imageNamespace,
						Labels: map[string]string{
							"test.harvesterhci.io": "for-test-update",
						},
						Annotations: map[string]string{
							"test.harvesterhci.io": "for-test-update",
						},
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						Description: "test description update",
						DisplayName: imageDisplayName,
						SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
						URL:         "http://harvesterhci.io/test.img",
					},
				}

				respCode int
				respBody []byte
				err      error
				imageURL = fmt.Sprintf("%s/%s/%s", imageAPI, imageNamespace, imageName)
				retImage harvesterv1.VirtualMachineImage
			)

			By("create image")
			respCode, respBody, err = helper.PostObject(imageAPI, image)
			MustRespCodeIs(http.StatusCreated, "post image", err, respCode, respBody)

			By("update image")
			// Do retries on update conflicts
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(imageURL, &retImage)
				MustRespCodeIs(http.StatusOK, "get image", err, respCode, respBody)
				toUpdateImage.ResourceVersion = retImage.ResourceVersion
				toUpdateImage.Kind = retImage.Kind
				toUpdateImage.APIVersion = retImage.APIVersion
				toUpdateImage.Spec.StorageClassParameters = retImage.Spec.StorageClassParameters

				respCode, respBody, err = helper.PutObject(imageURL, toUpdateImage)
				MustNotError(err)
				Expect(respCode).To(BeElementOf([]int{http.StatusOK, http.StatusConflict}))
				return respCode == http.StatusOK
			}, 1*time.Minute, 1*time.Second)

			By("then the image is updated")
			respCode, respBody, err = helper.GetObject(imageURL, &retImage)
			MustRespCodeIs(http.StatusOK, "get image", err, respCode, respBody)
			Expect(retImage.Labels).To(BeEquivalentTo(toUpdateImage.Labels))
			Expect(retImage.Annotations).To(BeEquivalentTo(toUpdateImage.Annotations))
			Expect(retImage.Spec).To(BeEquivalentTo(toUpdateImage.Spec))

			By("delete the image")
			respCode, respBody, err = helper.DeleteObject(imageURL)
			MustRespCodeIn("delete image", err, respCode, respBody, http.StatusOK, http.StatusNoContent)

			By("then the image is deleted")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(imageURL, nil)
				MustNotError(err)
				return respCode == http.StatusNotFound
			})
		})

		Specify("verify init fails with invalid url", func() {

			var (
				imageName        = fuzz.String(5)
				imageDisplayName = fuzz.String(5)
				image            = harvesterv1.VirtualMachineImage{
					ObjectMeta: v1.ObjectMeta{
						Name:      imageName,
						Namespace: imageNamespace,
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						DisplayName: imageDisplayName,
						SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
						URL:         "http://harvesterhci.io/test.img",
					},
				}

				getImageURL = fmt.Sprintf("%s/%s/%s", imageAPI, imageNamespace, imageName)
				retImage    harvesterv1.VirtualMachineImage
			)

			By("create image")
			respCode, respBody, err := helper.PostObject(imageAPI, image)
			MustRespCodeIs(http.StatusCreated, "post image", err, respCode, respBody)

			By("then the Initialized condition is false")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err := helper.GetObject(getImageURL, &retImage)
				MustRespCodeIs(http.StatusOK, "get image", err, respCode, respBody)
				return harvesterv1.ImageInitialized.IsFalse(retImage)
			}, 1*time.Minute, 1*time.Second)
		})

		Specify("verify init fails with invalid url by yaml", func() {

			var (
				imageName        = fuzz.String(5)
				imageDisplayName = fuzz.String(5)
				image            = harvesterv1.VirtualMachineImage{
					ObjectMeta: v1.ObjectMeta{
						Name:      imageName,
						Namespace: imageNamespace,
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						DisplayName: imageDisplayName,
						SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
						URL:         "http://harvesterhci.io/test.img",
					},
				}

				getImageURL = fmt.Sprintf("%s/%s/%s", imageAPI, imageNamespace, imageName)
				retImage    harvesterv1.VirtualMachineImage
			)

			By("create image", func() {
				respCode, respBody, err := helper.PostObjectByYAML(imageAPI, image)
				MustRespCodeIs(http.StatusCreated, "post image", err, respCode, respBody)
			})

			By("then the Initialized condition is false")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err := helper.GetObject(getImageURL, &retImage)
				MustRespCodeIs(http.StatusOK, "get image", err, respCode, respBody)
				return harvesterv1.ImageInitialized.IsFalse(retImage)
			}, 1*time.Minute, 1*time.Second)
		})

		Specify("verify image initialization succeeds", func() {

			var (
				imageName        = fuzz.String(5)
				imageDisplayName = fuzz.String(5)
				cirrosURL        = "https://download.cirros-cloud.net/0.5.1/cirros-0.5.1-x86_64-disk.img"
				image            = harvesterv1.VirtualMachineImage{
					ObjectMeta: v1.ObjectMeta{
						Name:      imageName,
						Namespace: imageNamespace,
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						DisplayName: imageDisplayName,
						SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
						URL:         cirrosURL,
					},
				}

				getImageURL = fmt.Sprintf("%s/%s/%s", imageAPI, imageNamespace, imageName)
				retImage    harvesterv1.VirtualMachineImage
			)

			By("create cirros image", func() {
				respCode, respBody, err := helper.PostObject(imageAPI, image)
				MustRespCodeIs(http.StatusCreated, "post image", err, respCode, respBody)
			})

			By("then the Initialized condition is true")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err := helper.GetObject(getImageURL, &retImage)
				MustRespCodeIs(http.StatusOK, "get image", err, respCode, respBody)
				Expect(harvesterv1.ImageInitialized.IsFalse(retImage)).NotTo(BeTrue())
				return harvesterv1.ImageInitialized.IsTrue(retImage)
			}, 1*time.Minute, 1*time.Second)
		})
	})

})
