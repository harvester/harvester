package utils

import (
	"encoding/base64"
	"log"
)

func Base64DecodeCertificate(clientCertificate string) (pfx []byte) {
	if clientCertificate != "" {
		out := make([]byte, base64.StdEncoding.DecodedLen(len(clientCertificate)))
		n, err := base64.StdEncoding.Decode(out, []byte(clientCertificate))
		if err != nil {
			log.Fatalf("test.NewConnection(): could not decode value of CLIENT_CERTIFICATE: %v", err)
		}
		pfx = out[:n]
	}
	return
}
