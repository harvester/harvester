package api

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/guonaihong/gout"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/test/framework/fuzz"
)

var _ = Describe("verify image apis", func() {

	var imageNamespace string

	BeforeEach(func() {

		imageNamespace = "default"

	})

	Context("operate via steve api", func() {

		var imagesAPI string

		BeforeEach(func() {

			imagesAPI = fmt.Sprintf("https://localhost:%d/v1/harvester.cattle.io.virtualmachineimage", config.HTTPSListenPort)

		})

		Specify("verify the upload action", func() {

			By("given a random size image")
			var imageDisplayname = fuzz.String(5)
			var imagePath, imageChecksum, err = fuzz.File(10 * fuzz.MB)
			Expect(err).ShouldNot(HaveOccurred())
			var image = struct {
				Namespace   string `form:"namespace"`
				DisplayName string `form:"displayName"`
				Path        string `form:"file" form-file:"true"`
			}{
				Namespace:   imageNamespace,
				DisplayName: imageDisplayname,
				Path:        imagePath,
			}

			By("when call upload action")
			var (
				respCode int
				respBody []byte
			)
			err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				POST(fmt.Sprintf("%s?action=upload", imagesAPI)).
				SetForm(image).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusOK {
				GinkgoT().Errorf("failed to post vmimage, response with %d, %v", respCode, string(respBody))
			}

			By("then the same display name image is created")
			var imageDownloadURL string
			Eventually(func() bool {
				var (
					respCode int
					respBody []byte
				)
				var err = gout.
					New(&http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						},
					}).
					GET(fmt.Sprintf("%ss", imagesAPI)).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				if err != nil {
					GinkgoT().Logf("failed to list vmimage, %v", err)
					return false
				}
				if respCode != http.StatusOK {
					GinkgoT().Logf("failed to list vmimage, response with %d, %v", respCode, string(respBody))
					return false
				}

				var selectImageJSONPath = fmt.Sprintf("data.#(spec.displayName==\"%s\")", imageDisplayname)
				var imageJSON = gjson.GetBytes(respBody, selectImageJSONPath)
				if imageJSON.Get("status.conditions.#(type==\"imported\").status").String() == "True" {
					imageDownloadURL = imageJSON.Get("status.downloadUrl").String()
					return true
				}
				return false
			}, 10, 1).Should(BeTrue())

			By("when download the image from storage")
			err = gout.
				GET(imageDownloadURL).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusOK {
				GinkgoT().Errorf("failed to download image, response with %d, %v", respCode, string(respBody))
			}

			By("then the downloaded image has the same checksum as the source")
			var imageChecksumActualArr = sha256.Sum256(respBody)
			var imageChecksumActual = hex.EncodeToString(imageChecksumActualArr[:])
			Expect(imageChecksumActual).To(Equal(imageChecksum))

		})

	})

})
