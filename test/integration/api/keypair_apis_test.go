package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/guonaihong/gout"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/ssh"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/test/framework/fuzz"
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

			keypairsAPI = fmt.Sprintf("https://localhost:%d/v1/harvester.cattle.io.keypairs", config.HTTPSListenPort)

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
			var err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				POST(fmt.Sprintf("%s?action=keygen", keypairsAPI)).
				SetJSON(keygen).
				BindBody(&respBody).
				BindHeader(&respHeader).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusOK {
				GinkgoT().Errorf("failed to post keygen action, response with %d, %v", respCode, string(respBody))
			}

			By("then output is a legal private key")
			Expect(respHeader.ContentDisposition).To(Equal(fmt.Sprintf("attachment; filename=%s.pem", keypairName)))
			Expect(respHeader.ContentType).To(Equal("application/octet-stream"))
			Expect(respHeader.ContentLength).To(Equal(len(respBody)))
			privateKeySigner, err := ssh.ParsePrivateKey(respBody)
			Expect(err).ShouldNot(HaveOccurred())
			var publicKey = privateKeySigner.PublicKey()
			var sshPublicKeyFingerprintExpected = ssh.FingerprintLegacyMD5(publicKey)

			By("then the same name keypair is created")
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
					GET(fmt.Sprintf("%s/%s/%s", keypairsAPI, keypairNamespace, keypairName)).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				if err != nil {
					GinkgoT().Logf("failed to get keypair, %v", err)
					return false
				}
				if respCode != http.StatusOK {
					GinkgoT().Logf("failed to get keypair, response with %d, %v", respCode, string(respBody))
					return false
				}
				if gjson.GetBytes(respBody, "status.conditions.#(type==\"validated\").status").String() == "True" {
					var sshPublicKeyFingerprintActual = gjson.GetBytes(respBody, "status.fingerPrint").String()
					return sshPublicKeyFingerprintActual == sshPublicKeyFingerprintExpected
				}
				return false
			}, 10, 1).Should(BeTrue())

		})

	})

})
