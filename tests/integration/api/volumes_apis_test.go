package api_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

var _ = Describe("verify volume APIs", func() {

	var namespace string

	BeforeEach(func() {

		namespace = "default"

	})

	Context("operate via steve API", func() {

		var volumeAPI string

		BeforeEach(func() {

			volumeAPI = helper.BuildAPIURL("v1", "persistentvolumeclaims", options.HTTPSListenPort)

		})

		Specify("verify required fileds for volumes", func() {

			var (
				volumeName = fuzz.String(5)
				volumeMode = corev1.PersistentVolumeFilesystem
			)

			By("create a volume with name missing", func() {
				var volume = corev1.PersistentVolumeClaim{
					ObjectMeta: v1.ObjectMeta{
						Namespace: namespace,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						VolumeMode:  &volumeMode,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				}
				respCode, respBody, err := helper.PostObject(volumeAPI, volume)
				MustRespCodeIs(http.StatusUnprocessableEntity, "post volume", err, respCode, respBody)
			})

			By("create a volume with size missing", func() {
				var volume = corev1.PersistentVolumeClaim{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						VolumeMode:  &volumeMode,
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
				volume     = corev1.PersistentVolumeClaim{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
						Labels: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						VolumeMode:  &volumeMode,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				}
				getVolumeURL = fmt.Sprintf("%s/%s/%s", volumeAPI, namespace, volumeName)
				retVolume    corev1.PersistentVolumeClaim
			)

			By("create an empty volume")
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
				volume     = corev1.PersistentVolumeClaim{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
						Labels: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						VolumeMode:  &volumeMode,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				}
				getVolumeURL = fmt.Sprintf("%s/%s/%s", volumeAPI, namespace, volumeName)
				retVolume    corev1.PersistentVolumeClaim
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
				volume     = corev1.PersistentVolumeClaim{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
						Labels: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
						Annotations: map[string]string{
							"test.harvesterhci.io": "for-test",
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						VolumeMode:  &volumeMode,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				}

				toUpdateVolume = corev1.PersistentVolumeClaim{
					ObjectMeta: v1.ObjectMeta{
						Name:      volumeName,
						Namespace: namespace,
						Labels: map[string]string{
							"test.harvesterhci.io": "for-test-update",
						},
						Annotations: map[string]string{
							"test.harvesterhci.io": "for-test-update",
						},
					},
				}
				volumeURL = fmt.Sprintf("%s/%s/%s", volumeAPI, namespace, volumeName)
				retVolume corev1.PersistentVolumeClaim
			)
			By("create volume")
			respCode, respBody, err := helper.PostObject(volumeAPI, volume)
			MustRespCodeIs(http.StatusCreated, "post volume", err, respCode, respBody)

			By("update volume")
			// Do retries on update conflicts
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(volumeURL, &retVolume)
				MustRespCodeIs(http.StatusOK, "get volume", err, respCode, respBody)
				retVolume.Labels = toUpdateVolume.Labels
				retVolume.Annotations = toUpdateVolume.Annotations

				respCode, respBody, err = helper.PutObject(volumeURL, retVolume)
				MustNotError(err)
				Expect(respCode).To(BeElementOf([]int{http.StatusOK, http.StatusConflict}), string(respBody))
				return respCode == http.StatusOK
			}, 1*time.Minute, 1*time.Second)

			By("then the volume is updated")
			respCode, respBody, err = helper.GetObject(volumeURL, &retVolume)
			MustRespCodeIs(http.StatusOK, "get volume", err, respCode, respBody)
			Expect(retVolume.Labels).To(BeEquivalentTo(toUpdateVolume.Labels))
			Expect(retVolume.Annotations).To(BeEquivalentTo(toUpdateVolume.Annotations))

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
	})
})
