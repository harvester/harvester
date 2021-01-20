package api_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/fuzz"
	"github.com/rancher/harvester/tests/framework/helper"
)

var _ = Describe("verify volume APIs", func() {

	var namespace string

	BeforeEach(func() {

		namespace = "default"

	})

	Context("operate via steve API", func() {

		var volumeAPI string

		BeforeEach(func() {

			volumeAPI = helper.BuildAPIURL("v1", "cdi.kubevirt.io.datavolumes", options.HTTPSListenPort)

		})

		Specify("verify required fileds for volumes", func() {

			var (
				volumeName = fuzz.String(5)
				volumeMode = corev1.PersistentVolumeFilesystem
			)

			By("create a volume with name missing", func() {
				var volume = cdiv1beta1.DataVolume{
					ObjectMeta: v1.ObjectMeta{
						Namespace: namespace,
					},
					Spec: cdiv1beta1.DataVolumeSpec{
						Source: cdiv1beta1.DataVolumeSource{
							Blank: &cdiv1beta1.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							VolumeMode:  &volumeMode,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				}
				respCode, respBody, err := helper.PostObject(volumeAPI, volume)
				MustRespCodeIs(http.StatusUnprocessableEntity, "post volume", err, respCode, respBody)
			})

			By("create a volume with size missing", func() {
				var volume = cdiv1beta1.DataVolume{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
					},
					Spec: cdiv1beta1.DataVolumeSpec{
						Source: cdiv1beta1.DataVolumeSource{
							Blank: &cdiv1beta1.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							VolumeMode:  &volumeMode,
						},
					},
				}
				respCode, respBody, err := helper.PostObject(volumeAPI, volume)
				MustRespCodeIs(http.StatusUnprocessableEntity, "post volume", err, respCode, respBody)
			})
		})

		Specify("verify volume fields set", func() {

			var (
				volumeName = fuzz.String(5)
				volumeMode = corev1.PersistentVolumeFilesystem
				volume     = cdiv1beta1.DataVolume{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
						Labels: map[string]string{
							"test.harvester.cattle.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvester.cattle.io": "for-test",
						},
					},
					Spec: cdiv1beta1.DataVolumeSpec{
						Source: cdiv1beta1.DataVolumeSource{
							Blank: &cdiv1beta1.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							VolumeMode:  &volumeMode,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				}
				getVolumeURL = fmt.Sprintf("%s/%s/%s", volumeAPI, namespace, volumeName)
				retVolume    cdiv1beta1.DataVolume
			)

			By("create an empty source volume")
			respCode, respBody, err := helper.PostObject(volumeAPI, volume)
			MustRespCodeIs(http.StatusCreated, "post volume", err, respCode, respBody)

			By("then the fields are set correctly")
			respCode, respBody, err = helper.GetObject(getVolumeURL, &retVolume)
			MustRespCodeIs(http.StatusOK, "get volume", err, respCode, respBody)
			Expect(retVolume.Labels).To(BeEquivalentTo(retVolume.Labels))
			Expect(retVolume.Annotations).To(BeEquivalentTo(retVolume.Annotations))
			Expect(retVolume.Spec).To(BeEquivalentTo(retVolume.Spec))
		})

		Specify("verify volume fields set by yaml", func() {

			var (
				volumeName = fuzz.String(5)
				volumeMode = corev1.PersistentVolumeFilesystem
				volume     = cdiv1beta1.DataVolume{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
						Labels: map[string]string{
							"test.harvester.cattle.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvester.cattle.io": "for-test",
						},
					},
					Spec: cdiv1beta1.DataVolumeSpec{
						Source: cdiv1beta1.DataVolumeSource{
							Blank: &cdiv1beta1.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							VolumeMode:  &volumeMode,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				}
				getVolumeURL = fmt.Sprintf("%s/%s/%s", volumeAPI, namespace, volumeName)
				retVolume    cdiv1beta1.DataVolume
			)

			By("create an empty source volume")
			respCode, respBody, err := helper.PostObjectByYAML(volumeAPI, volume)
			MustRespCodeIs(http.StatusCreated, "post volume", err, respCode, respBody)

			By("then the fields are set correctly")
			respCode, respBody, err = helper.GetObject(getVolumeURL, &retVolume)
			MustRespCodeIs(http.StatusOK, "get volume", err, respCode, respBody)
			Expect(retVolume.Labels).To(BeEquivalentTo(retVolume.Labels))
			Expect(retVolume.Annotations).To(BeEquivalentTo(retVolume.Annotations))
			Expect(retVolume.Spec).To(BeEquivalentTo(retVolume.Spec))
		})

		Specify("verify update and delete volumes", func() {
			var (
				volumeName = fuzz.String(5)
				volumeMode = corev1.PersistentVolumeFilesystem
				volume     = cdiv1beta1.DataVolume{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
						Labels: map[string]string{
							"test.harvester.cattle.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvester.cattle.io": "for-test",
						},
					},
					Spec: cdiv1beta1.DataVolumeSpec{
						Source: cdiv1beta1.DataVolumeSource{
							Blank: &cdiv1beta1.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							VolumeMode:  &volumeMode,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				}

				toUpdateVolume = cdiv1beta1.DataVolume{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
						Labels: map[string]string{
							"test.harvester.cattle.io": "for-test-update",
						},
						Annotations: map[string]string{
							"test.harvester.cattle.io": "for-test-update",
						},
					},
					// data volume spec is not updatable
					Spec: volume.Spec,
				}
				volumeURL = fmt.Sprintf("%s/%s/%s", volumeAPI, namespace, volumeName)
				retVolume cdiv1beta1.DataVolume
			)
			By("create volume")
			respCode, respBody, err := helper.PostObject(volumeAPI, volume)
			MustRespCodeIs(http.StatusCreated, "post volume", err, respCode, respBody)

			By("update volume")
			// Do retries on update conflicts
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(volumeURL, &retVolume)
				MustRespCodeIs(http.StatusOK, "get volume", err, respCode, respBody)
				toUpdateVolume.ResourceVersion = retVolume.ResourceVersion
				toUpdateVolume.Kind = retVolume.Kind
				toUpdateVolume.APIVersion = retVolume.APIVersion

				respCode, respBody, err = helper.PutObject(volumeURL, toUpdateVolume)
				MustNotError(err)
				Expect(respCode).To(BeElementOf([]int{http.StatusOK, http.StatusConflict}), string(respBody))
				return respCode == http.StatusOK
			}, 1*time.Minute, 1*time.Second)

			By("then the volume is updated")
			respCode, respBody, err = helper.GetObject(volumeURL, &retVolume)
			MustRespCodeIs(http.StatusOK, "get volume", err, respCode, respBody)
			Expect(retVolume.Labels).To(BeEquivalentTo(toUpdateVolume.Labels))
			Expect(retVolume.Annotations).To(BeEquivalentTo(toUpdateVolume.Annotations))
			Expect(retVolume.Spec).To(BeEquivalentTo(toUpdateVolume.Spec))

			By("delete the volume")
			respCode, respBody, err = helper.DeleteObject(volumeURL)
			MustRespCodeIn("delete volume", err, respCode, respBody, http.StatusOK, http.StatusNoContent)

			By("then the volume is deleted")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(volumeURL, nil)
				MustNotError(err)
				return respCode == http.StatusNotFound
			})
		})

		Specify("verify blank volumes", func() {

			var (
				volumeName = fuzz.String(5)
				volumeMode = corev1.PersistentVolumeFilesystem
				volume     = cdiv1beta1.DataVolume{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
					},
					Spec: cdiv1beta1.DataVolumeSpec{
						Source: cdiv1beta1.DataVolumeSource{
							Blank: &cdiv1beta1.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							VolumeMode:  &volumeMode,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				}
				getVolumeURL = fmt.Sprintf("%s/%s/%s", volumeAPI, namespace, volumeName)
				retVolume    cdiv1beta1.DataVolume
			)

			By("create an empty source volume")
			respCode, respBody, err := helper.PostObject(volumeAPI, volume)
			MustRespCodeIs(http.StatusCreated, "post volume", err, respCode, respBody)

			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(getVolumeURL, &retVolume)
				MustRespCodeIs(http.StatusOK, "get volume", err, respCode, respBody)

				return retVolume.Status.Phase == cdiv1beta1.Succeeded
			}, 2*time.Minute, 2*time.Second)
		})

		Specify("verify VM image source volumes", func() {

			var imageDownloadURL string

			By("Prepare VM image for volumes", func() {

				var (
					imageAPI         = helper.BuildAPIURL("v1", "harvester.cattle.io.virtualmachineimage", options.HTTPSListenPort)
					imageName        = fuzz.String(5)
					imageDisplayName = fuzz.String(5)
					cirrosURL        = "https://download.cirros-cloud.net/0.5.1/cirros-0.5.1-x86_64-disk.img"
					image            = v1alpha1.VirtualMachineImage{
						ObjectMeta: v1.ObjectMeta{
							Name:      imageName,
							Namespace: namespace,
						},
						Spec: v1alpha1.VirtualMachineImageSpec{
							DisplayName: imageDisplayName,
							URL:         cirrosURL,
						},
					}
					getImageURL = fmt.Sprintf("%s/%s/%s", imageAPI, namespace, imageName)
					retImage    v1alpha1.VirtualMachineImage
				)

				respCode, respBody, err := helper.PostObject(imageAPI, image)
				MustRespCodeIs(http.StatusCreated, "post image", err, respCode, respBody)
				MustFinallyBeTrue(func() bool {
					respCode, respBody, err := helper.GetObject(getImageURL, &retImage)
					MustRespCodeIs(http.StatusOK, "get image", err, respCode, respBody)
					Expect(v1alpha1.ImageImported.IsFalse(retImage)).NotTo(BeTrue())
					if retImage.Status.DownloadURL != "" {
						imageDownloadURL = retImage.Status.DownloadURL
					}
					return v1alpha1.ImageImported.IsTrue(retImage)
				}, 1*time.Minute, 1*time.Second)
			})

			var (
				volumeName = fuzz.String(5)
				volumeMode = corev1.PersistentVolumeFilesystem
				volume     = cdiv1beta1.DataVolume{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
					},
					Spec: cdiv1beta1.DataVolumeSpec{
						Source: cdiv1beta1.DataVolumeSource{
							HTTP: &cdiv1beta1.DataVolumeSourceHTTP{
								URL: imageDownloadURL,
							},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							VolumeMode:  &volumeMode,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				}
				getVolumeURL = fmt.Sprintf("%s/%s/%s", volumeAPI, namespace, volumeName)
				retVolume    cdiv1beta1.DataVolume
			)

			By("create an VM image source volume")
			respCode, respBody, err := helper.PostObject(volumeAPI, volume)
			MustRespCodeIs(http.StatusCreated, "post volume", err, respCode, respBody)

			By("then succeeded")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(getVolumeURL, &retVolume)
				MustRespCodeIs(http.StatusOK, "get volume", err, respCode, respBody)

				return retVolume.Status.Phase == cdiv1beta1.Succeeded
			}, 2*time.Minute, 2*time.Second)
		})
	})
})
