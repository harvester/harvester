package api_test

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/guonaihong/gout"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/ssh"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

var _ = Describe("verify keypair APIs", func() {

	var keypairNamespace string

	BeforeEach(func() {

		// keypair is stored in the same namespace of harvester
		keypairNamespace = options.Namespace

	})

	Context("operate via steve API", func() {

		var keypairsAPI string

		BeforeEach(func() {

			keypairsAPI = helper.BuildAPIURL("v1", "harvesterhci.io.keypairs", options.HTTPSListenPort)

		})

		Specify("verify required fields", func() {

			By("create a keypair with name missing", func() {
				rsaKey, err := util.GeneratePrivateKey(2048)
				MustNotError(err)
				publicKey, err := util.GeneratePublicKey(&rsaKey.PublicKey)
				MustNotError(err)
				var keypair = harvesterv1.KeyPair{
					ObjectMeta: v1.ObjectMeta{
						Namespace: keypairNamespace,
					},
					Spec: harvesterv1.KeyPairSpec{
						PublicKey: string(publicKey),
					},
				}
				respCode, respBody, err := helper.PostObject(keypairsAPI, keypair)
				MustRespCodeIs(http.StatusUnprocessableEntity, "post keypair", err, respCode, respBody)
			})

			By("create a keypair with public key missing", func() {
				var keypair = harvesterv1.KeyPair{
					ObjectMeta: v1.ObjectMeta{
						Name:      fuzz.String(5),
						Namespace: keypairNamespace,
					},
					Spec: harvesterv1.KeyPairSpec{},
				}
				respCode, respBody, err := helper.PostObject(keypairsAPI, keypair)
				MustRespCodeIs(http.StatusUnprocessableEntity, "post keypair", err, respCode, respBody)
			})
		})

		Specify("verify validation for invalid public key", func() {

			By("create a keypair with invalid public key")
			var keypair = harvesterv1.KeyPair{
				ObjectMeta: v1.ObjectMeta{
					Name:      fuzz.String(5),
					Namespace: keypairNamespace,
				},
				Spec: harvesterv1.KeyPairSpec{
					PublicKey: "invalid test public key",
				},
			}
			respCode, respBody, err := helper.PostObject(keypairsAPI, keypair)
			MustRespCodeIs(http.StatusUnprocessableEntity, "post keypair", err, respCode, respBody)
		})

		Specify("verify keypair creation and deletion", func() {

			rsaKey, err := util.GeneratePrivateKey(2048)
			MustNotError(err)
			publicKey, err := util.GeneratePublicKey(&rsaKey.PublicKey)
			MustNotError(err)
			pk, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
			MustNotError(err)
			expectedFingerPrint := ssh.FingerprintLegacyMD5(pk)

			var (
				keypairName = fuzz.String(5)
				keypair     = harvesterv1.KeyPair{
					ObjectMeta: v1.ObjectMeta{
						Name:      keypairName,
						Namespace: keypairNamespace,
					},
					Spec: harvesterv1.KeyPairSpec{
						PublicKey: string(publicKey),
					},
				}
				retKeypair harvesterv1.KeyPair
				keypairURL = fmt.Sprintf("%s/%s/%s", keypairsAPI, keypairNamespace, keypairName)
			)
			By("create a keypair")
			respCode, respBody, err := helper.PostObject(keypairsAPI, keypair)
			MustRespCodeIs(http.StatusCreated, "post keypair", err, respCode, respBody)

			By("then creation succeeded")
			respCode, respBody, err = helper.GetObject(keypairURL, &retKeypair)
			MustRespCodeIs(http.StatusOK, "get keypair", err, respCode, respBody)
			Expect(retKeypair.Spec).To(BeEquivalentTo(keypair.Spec))

			By("then fingerprint is set")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(keypairURL, &retKeypair)
				MustRespCodeIs(http.StatusOK, "get keypair", err, respCode, respBody)
				MustNotEqual(harvesterv1.KeyPairValidated.IsFalse(retKeypair), true)
				return retKeypair.Status.FingerPrint == expectedFingerPrint
			})

			By("delete the keypair")
			respCode, respBody, err = helper.DeleteObject(keypairURL)
			MustRespCodeIn("delete keypair", err, respCode, respBody, http.StatusOK, http.StatusNoContent)

			By("then the keypair is deleted")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(keypairURL, nil)
				MustNotError(err)
				return respCode == http.StatusNotFound
			})
		})

		Specify("verify keypair creation by yaml", func() {

			rsaKey, err := util.GeneratePrivateKey(2048)
			MustNotError(err)
			publicKey, err := util.GeneratePublicKey(&rsaKey.PublicKey)
			MustNotError(err)
			pk, _, _, _, err := ssh.ParseAuthorizedKey(publicKey)
			MustNotError(err)
			expectedFingerPrint := ssh.FingerprintLegacyMD5(pk)

			var (
				keypairName = fuzz.String(5)
				keypair     = harvesterv1.KeyPair{
					ObjectMeta: v1.ObjectMeta{
						Name:      keypairName,
						Namespace: keypairNamespace,
					},
					Spec: harvesterv1.KeyPairSpec{
						PublicKey: string(publicKey),
					},
				}
				retKeypair harvesterv1.KeyPair
				keypairURL = fmt.Sprintf("%s/%s/%s", keypairsAPI, keypairNamespace, keypairName)
			)
			By("create a keypair by yaml")
			respCode, respBody, err := helper.PostObjectByYAML(keypairsAPI, keypair)
			MustRespCodeIs(http.StatusCreated, "post keypair", err, respCode, respBody)

			By("then creation succeeded")
			respCode, respBody, err = helper.GetObject(keypairURL, &retKeypair)
			MustRespCodeIs(http.StatusOK, "get keypair", err, respCode, respBody)
			Expect(retKeypair.Spec).To(BeEquivalentTo(keypair.Spec))

			By("then fingerprint is set")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err = helper.GetObject(keypairURL, &retKeypair)
				MustRespCodeIs(http.StatusOK, "get keypair", err, respCode, respBody)
				MustNotEqual(harvesterv1.KeyPairValidated.IsFalse(retKeypair), true)
				return retKeypair.Status.FingerPrint == expectedFingerPrint
			})

		})

		Specify("verify the keygen action", func() {

			By("given a random name keypair")
			var keypairName = strings.ToLower(fuzz.String(5))
			var keygen = gout.H{
				"name":      keypairName,
				"namespace": keypairNamespace,
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
