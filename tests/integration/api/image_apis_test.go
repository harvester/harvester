package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	"github.com/tidwall/gjson"

	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/fuzz"
	"github.com/rancher/harvester/tests/framework/helper"
)

var _ = Describe("verify image apis", func() {

	var imageNamespace string

	BeforeEach(func() {

		imageNamespace = "default"

	})

	Context("operate via steve api", func() {

		var imageAPI string

		BeforeEach(func() {

			imageAPI = helper.BuildAPIURL("v1", "harvester.cattle.io.virtualmachineimage")

		})

		Specify("verify the upload action", func() {

			By("given a random size image")
			imageDisplayName := fuzz.String(5)
			imagePath, imageChecksum, err := fuzz.File(10 * fuzz.MB)
			MustNotError(err)

			By("when call upload action")
			var (
				respCode int
				respBody []byte
				image    = struct {
					Namespace   string `form:"namespace"`
					DisplayName string `form:"displayName"`
					Path        string `form:"file" form-file:"true"`
				}{
					Namespace:   imageNamespace,
					DisplayName: imageDisplayName,
					Path:        imagePath,
				}
			)
			err = helper.NewHTTPClient().
				POST(fmt.Sprintf("%s?action=upload", imageAPI)).
				SetForm(image).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusOK, "post image", err, respCode, respBody)

			By("then the same display name image is created")
			var imageDownloadURL string
			MustFinallyBeTrue(func() bool {
				var (
					respCode int
					respBody []byte
				)
				err := helper.NewHTTPClient().GET(fmt.Sprintf("%ss", imageAPI)).
					BindBody(&respBody).
					Code(&respCode).
					Do()

				if ok := CheckRespCodeIs(http.StatusOK, "list image", err, respCode, respBody); !ok {
					return false
				}

				selectImageJSONPath := fmt.Sprintf("data.#(spec.displayName==\"%s\")", imageDisplayName)
				imageJSON := gjson.GetBytes(respBody, selectImageJSONPath)
				if imageJSON.Get("status.conditions.#(type==\"imported\").status").String() == "True" {
					imageDownloadURL = imageJSON.Get("status.downloadUrl").String()
					return true
				}
				return false
			})

			By("when download the image from storage")
			err = helper.NewHTTPClient().
				GET(imageDownloadURL).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusOK, "download image", err, respCode, respBody)

			By("then the downloaded image has the same checksum as the source")
			imageChecksumActualArr := sha256.Sum256(respBody)
			imageChecksumActual := hex.EncodeToString(imageChecksumActualArr[:])
			MustEqual(imageChecksumActual, imageChecksum)
		})

	})

})
