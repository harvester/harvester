package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/guonaihong/gout"
	. "github.com/onsi/ginkgo"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/ssh"

	"github.com/rancher/harvester/pkg/config"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/fuzz"
	"github.com/rancher/harvester/tests/framework/helper"
)

var _ = Describe("verify keypair apis", func() {

	var keypairNamespace string

	BeforeEach(func() {

		// keypair is stored in the same namespace of harvester
		keypairNamespace = config.Namespace

	})

	Context("operate via steve api", func() {

		var keypairsAPI string

		BeforeEach(func() {

			keypairsAPI = helper.BuildAPIURL("v1", "harvester.cattle.io.keypairs")

		})

		Specify("verify the keygen action", func() {

			By("given a random name keypair")
			var keypairName = strings.ToLower(fuzz.String(5))
			var keygen = gout.H{
				"name": keypairName,
			}

			By("when call keygen action")
			var (
				respCode   int
				respBody   []byte
				respHeader struct {
					ContentDisposition string `header:"content-disposition"`
					ContentType        string `header:"content-type"`
					ContentLength      int    `header:"content-length"`
				}
			)
			var err = helper.NewHTTPClient().
				POST(fmt.Sprintf("%s?action=keygen", keypairsAPI)).
				SetJSON(keygen).
				BindBody(&respBody).
				BindHeader(&respHeader).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusOK, "post keygen action", err, respCode, respBody)

			By("then output is a legal private key")
			MustEqual(respHeader.ContentDisposition, fmt.Sprintf("attachment; filename=%s.pem", keypairName))
			MustEqual(respHeader.ContentType, "application/octet-stream")
			MustEqual(respHeader.ContentLength, len(respBody))
			privateKeySigner, err := ssh.ParsePrivateKey(respBody)
			MustNotError(err)
			var publicKey = privateKeySigner.PublicKey()
			var sshPublicKeyFingerprintExpected = ssh.FingerprintLegacyMD5(publicKey)

			By("then the same name keypair is created")
			MustFinallyBeTrue(func() bool {
				var (
					respCode int
					respBody []byte
				)
				err = helper.NewHTTPClient().
					GET(fmt.Sprintf("%s/%s/%s", keypairsAPI, keypairNamespace, keypairName)).
					BindBody(&respBody).
					Code(&respCode).
					Debug().
					Do()
				if ok := CheckRespCodeIs(http.StatusOK, "get keypair", err, respCode, respBody); !ok {
					return false
				}
				validatedStatus := gjson.GetBytes(respBody, "status.conditions.#(type==\"validated\").status").String()
				if validatedStatus != "True" {
					return false
				}
				sshPublicKeyFingerprintActual := gjson.GetBytes(respBody, "status.fingerPrint").String()
				return sshPublicKeyFingerprintActual == sshPublicKeyFingerprintExpected
			})
		})

	})

})
