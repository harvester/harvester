package tls

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	// Sample RSA certificate
	// generated from https://www.selfsignedcertificate.com/
	rsaPublicCertificate = `-----BEGIN CERTIFICATE-----
MIIC1TCCAb2gAwIBAgIJAKeDcYV7SJD7MA0GCSqGSIb3DQEBBQUAMBoxGDAWBgNV
BAMTD3d3dy5leGFtcGxlLmNvbTAeFw0yMTExMjUwOTE2MDdaFw0zMTExMjMwOTE2
MDdaMBoxGDAWBgNVBAMTD3d3dy5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEB
BQADggEPADCCAQoCggEBAMUdnn5u/HpBqn8WO435TNohFKNXhu9oJVGzimn2o+oF
d6L6TOVA2dGZUUuv4pA8VyOmOSeXCsjqOTjrBk4Jf8pgaO+6AXH9i5LOnYSKMdZh
rsMKA6rcLk/qqotyJVmA82jOkniqWMJPjfjbj931o7os6Q2GvABxQ66cw+Lg9XVf
13Fnsm9jLOosqUfbJyQFPD0JRABAnlmDF4jcmYRxZn2Rp+iN7DK7aMkitouWarXq
Fq6+yFzsrgADxqQaLggCjbC4wJ60r1NH8wJ3pEfGT47uQ7nTpXwVfNjXzSdIs6+l
O5uQQyFjy/aqI75iVSzoUVXsPX8FIdnnnHEg3kbNsUUCAwEAAaMeMBwwGgYDVR0R
BBMwEYIPd3d3LmV4YW1wbGUuY29tMA0GCSqGSIb3DQEBBQUAA4IBAQCM74fili8Q
ivT/mw8rUX6i/QA271BLZvGqoZJiHvOYc73nhYU5ssX5M77Mm3dS3bIIAT106Ms4
PMmd1NAXeytF8jAfHc8yTaZZD6MOQMEV5ZNyw9GgBS7XB9WceSW4zALM/iVIuKvd
2PzXHMe/liYhW7KQkE1Laze/I0WY1jB7VEjlSRmxlKSJ9HHYsukGSTvZrzT7ql3X
OEikx/YABhBq9wy5m0UClM4QDEsmIGgDLMwag3n0yguqYD6P0mVU6pyG5JydkHnh
OsuVP6DdQFneu/Vpmf6yuadMCZR/mV6DFO2dwJXAPZ2mgRRqXCZd9oYuR8yhzWMQ
I9IvUdWBn7dq
-----END CERTIFICATE-----`
	rsaPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAxR2efm78ekGqfxY7jflM2iEUo1eG72glUbOKafaj6gV3ovpM
5UDZ0ZlRS6/ikDxXI6Y5J5cKyOo5OOsGTgl/ymBo77oBcf2Lks6dhIox1mGuwwoD
qtwuT+qqi3IlWYDzaM6SeKpYwk+N+NuP3fWjuizpDYa8AHFDrpzD4uD1dV/XcWey
b2Ms6iypR9snJAU8PQlEAECeWYMXiNyZhHFmfZGn6I3sMrtoySK2i5ZqteoWrr7I
XOyuAAPGpBouCAKNsLjAnrSvU0fzAnekR8ZPju5DudOlfBV82NfNJ0izr6U7m5BD
IWPL9qojvmJVLOhRVew9fwUh2eeccSDeRs2xRQIDAQABAoIBAFL8SEjEYwj5clU0
v/fimAdRXAX0iHtsJiICa2h3DMUubhKwPAVcSxeh64bo0oKU1L7OcUuInGK/sT2U
PMBH4YQLGMKsVYVvD/7Le6dcIuU1lMTKB4c8UUcV7ZztfmnzDwb1yNaCKQINSiEW
FriEfNyZobgvHCE3jh4KI7H1sYErDgDUk7uMSz2EJFpmJ/WkMurVFolPPn+MEIBs
ltZvuyVxIGSq1hCgejWvDiUceVnLJcgCA65qiMwoYuR+ON4OA/7RYPfRJO7vcp+F
sB3rNsyoTn0NH3rse5Y+tQ8/jyA+EpVF3HU5cJHwPc+Jxfk6k0WIC62BeDnz7tdV
Q7AqszUCgYEA7A2Q6QjcodmwId5WWhGgS0cGTt2KQcb/zxs/EEZsdPHVqVojV0RY
05tLuNPEr/o8zZnP3QSkEpH3kACMDzgH0VJQDu405E7TGFfgR0x1ZHn+yugI2jlv
iVjq6elbpHpkB+bCPpQEYnJA5NPDK6pzkBqNHXxJk6MtamDP88WI7VcCgYEA1cW9
OpO9KGP4RqmeUDSHLF7GFoGavduvrfdhSgKDAOskOFNR9ZuSCK8o1luKOdNz6XBP
CXohEeiRLnmbQ7XCgiFAaymgpA6FHS70MANUXPmljxf1fPpoNUIXaF5d5lHsTmzB
AxEgc3EMgrc5K1shEYHuR4NPqN3d4dS/cwONWMMCgYALrkQsc+bPD4Gau3DUdijT
cMlMH8RWqu0/p16AhKubQdhL0A0NpXErz3R4yenit2RI3EKf8jnYPWbdtlk365Lf
dc5GXt05KvlhLAAKJytr9Gl6Su8dNVhimIbPWl/RjMjkZzPXeuWYpYS2jhALWhzr
1ZSEEAFoD9wQdofzzSOQcwKBgDt/DWuAMuVK7Y69JpKsC/MNbZRV/ftZaUvBzhIL
IOrghvQmPGlfIwXHulXupEnz0A7ocxbwJsQVNlL5BX2S2M/e8U7iBxOh9upoZw31
30UBNlLdGDXwe5BXFKy3lurDYkFxg0aXPbDjhdfbps2qT0nQH8FHiqQ1G8v+qkoY
cv6BAoGBAKqx/+JeOTIOaSsrkhgDEJcGyfIVbX3YwVONgTLrYCYPSoT6wNROCEiQ
zWmEYYfH1zg2UErhq84neXqKloPsK1h3BzgF6QnIXks5TRSqVNGFfRgUC+5JJ2FC
aYgCAsCDf8z+cq2HzPFMRutfWupJyN8mVGCEJQCVl6CRy3e5NeHe
-----END RSA PRIVATE KEY-----`

	// Sample EC certificate
	// openssl ecparam -name prime256v1 -genkey -out ec_key.pem
	// openssl req -new -x509 -key ec_key.pem -out ec_crt.pem -days 3650 \
	//  -subj "/C=US/ST=CA/O=Acme" \
	//  -reqexts SAN -extensions SAN -config <(cat /etc/ssl/openssl.cnf \
	//  <(printf "\n[SAN]\nsubjectAltName=DNS:example.com,DNS:www.example.com"))
	ecPublicCertificate = `-----BEGIN CERTIFICATE-----
MIIBdDCCARqgAwIBAgIJAOx++iJycIx5MAoGCCqGSM49BAMCMCkxCzAJBgNVBAYT
AlVTMQswCQYDVQQIDAJDQTENMAsGA1UECgwEQWNtZTAeFw0yMTExMjUwOTI1NDNa
Fw0zMTExMjMwOTI1NDNaMCkxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJDQTENMAsG
A1UECgwEQWNtZTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABOZbP1VfwAAgb/9Z
5gtdiKrXnmS6cLNN01tB6zFnvWVEhnsDIEgURhlahhD6/zxIWNuUhyesVRz526ac
8TXktNyjKzApMCcGA1UdEQQgMB6CC2V4YW1wbGUuY29tgg93d3cuZXhhbXBsZS5j
b20wCgYIKoZIzj0EAwIDSAAwRQIhALxufYRJTrNGdFpdN+HvPkqZ9agM72uWp5UL
iIKypZoiAiAPhmX9ni61slHE8kpnSiF3A02rpNW8mcKsZwfLR54nEw==
-----END CERTIFICATE-----`
	ecPrivateKey = `-----BEGIN EC PARAMETERS-----
BggqhkjOPQMBBw==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIAOqNCRoigQqPE0hkq6AlzTJ9p3jTeKTmiqOwP1uvvRVoAoGCCqGSM49
AwEHoUQDQgAE5ls/VV/AACBv/1nmC12IqteeZLpws03TW0HrMWe9ZUSGewMgSBRG
GVqGEPr/PEhY25SHJ6xVHPnbppzxNeS03A==
-----END EC PRIVATE KEY-----`

	// Sample CA certificate
	// openssl genrsa -out ca_key.pem 4096
	// openssl req -x509 -new -nodes -key ca_key.pem -sha256 -days 1024 -out ca_crt.pem
	caCertificate = `-----BEGIN CERTIFICATE-----
MIIFKDCCAxACCQDhu/zHZWGpODANBgkqhkiG9w0BAQsFADBWMQswCQYDVQQGEwJD
TjELMAkGA1UECAwCR0QxCzAJBgNVBAcMAlNaMQ0wCwYDVQQKDARTVVNFMQwwCgYD
VQQLDANERVYxEDAOBgNVBAMMB2xhd3ItY2EwHhcNMjExMTI0MDcyMDMwWhcNMjQw
OTEzMDcyMDMwWjBWMQswCQYDVQQGEwJDTjELMAkGA1UECAwCR0QxCzAJBgNVBAcM
AlNaMQ0wCwYDVQQKDARTVVNFMQwwCgYDVQQLDANERVYxEDAOBgNVBAMMB2xhd3It
Y2EwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQC8oaQHuGhSsT3igqMN
ncCGlxW3eBvrKFGV36Bn93LtEEzieiGhEZfGNaR+Wps0MQ8rHG0H4tffz/wdKs/I
PrdVuH49f+OX2WHb+b/vXS6AJIl0XA1ipMBORnl2w9a2gP6Kx4hFC5q6xW38mS/g
lGKNf6x/4sCOul0WAJh8+pGT1brT+9uEkjtSa5cXd7wBSziRKcXfhKg1nAiwOzWa
UjwBQeX8vRc/TcT1fR80bHi2wS1+H89X7Lyl1j+7fG0D7oFqB2c1i9auU4/Nmvpn
YZJAE/FpNeFXab3w8X3wz3mjxlnUhlzMKQT3kT02H05SYOUzfIQKowYv99RxQfuq
Qa8PSGslYO5qHEQOWODTlvxl9SXifgbl2UBvL2fmKIpFs+ZxIjKxBWJOqvIyNDy2
BqAoBy/gPRI5T/MbpBxGzwIS+1Ps2Aqe0xBMK1x/b71y6Oj9ajAcuC02voh9U0hm
iHhgUqTLTo29zESUV2TGMnEqpUy1YWqs+DG2ETcaD3fkRq7INAaNMRssKEPfrsON
n84yKoJ/xB44SBILWb/TDH4OsUhV+QuMQqaAsR6XL+I+DTI2I+qf1Tea4yOYMmKn
Nv9Jf/fvzPa/IVJIEaHWKa+7+mVRNw1Q1udDI2unv097p+/bnKW/5McUzJ9Ninrq
LyctwHom5OjOp2uU3gxTk9sI8wIDAQABMA0GCSqGSIb3DQEBCwUAA4ICAQCxOqgp
ZkBpAwTHwtliXH/liRzsVl3TOeXV3y4A+QLgoSKWMlqOAP/KSXBsg0D21Itnrt7+
KnkICdoPbxwR/E9oxwCFi2bwa8HL4+f5Ay1kWarzLMbM/Ov49OOR5WPZKb+VI2GJ
id5W9ZrFVDZh4N3VebIpkzBU9rXw32HTQyrcN7SUeEQ1Va3JrqdahLVIvW0dUuA5
CBCodA7loDOPlPeLQZjhBMANKnyXQcbRnqtEqqJ0S6zSRtZRIU7u4n27az3HRIgl
HzN8mgZ0TaGG4AZp/5xmRoZIFD6sje9EOZ9eW5vq2W1spKaZbmnnezp2TRnTX3jt
aLHRzs9obcXpqu+8PbonOXmQ/9pvfHyZqgZQuWaOL26r3f2svkAfRxWHjc7dpo8W
CWxSJnPCTcbHP2YrtvRvHH5Nmp/796QS0kKbnNxaFeTd62lk/egyDvnHsZ3m8MCx
vAuzd4qD0vD6Dhoc+HpK8x28dszyeGZ6WB2FxMZE1E0XSIiuUAoMjfgl9UlVSBJK
4okCzbSlL7o2DMt5uHk2YS/sksXJyQ6kw+GZCMuz7OSB9b4oV6D0bGrZCNqqBm3h
81JWqiqry/GGsJaYu7lZfqivCLBLVK0zBoaYCffo+PuVHMkkc7Y5gGAXqdY+8zf4
sigGu6RstlytNkq8d3Ozq/jqbStvCoVvXpHVWw==
-----END CERTIFICATE-----`
)

func TestCAValidation(t *testing.T) {
	tests := map[string]struct {
		pem   string
		valid bool
		err   error
	}{
		"valid CA": {
			pem: caCertificate,
			err: nil,
		},
		"invalid CA": {
			pem: "INVALID TEST CA DATA",
			err: errors.New("failed to locate certificate"),
		},
	}

	for _, test := range tests {
		err := ValidateCABundle([]byte(test.pem))
		assert.Equal(t, err, test.err)
	}
}

func TestPublicCertificateValidation(t *testing.T) {
	tests := map[string]struct {
		pem   string
		valid bool
		err   error
	}{
		"valid rsa cert": {
			pem: rsaPublicCertificate,
			err: nil,
		},
		"valid ec cert": {
			pem: ecPublicCertificate,
			err: nil,
		},
		"invalid cert": {
			pem: "INVALID TEST CERT DATA",
			err: errors.New("failed to locate certificate"),
		},
	}

	for _, test := range tests {
		err := ValidateServingBundle([]byte(test.pem))
		assert.Equal(t, err, test.err)
	}
}

func TestPrivateKeyValidation(t *testing.T) {
	tests := map[string]struct {
		pem   string
		valid bool
		err   error
	}{
		"valid rsa private key": {
			pem: rsaPrivateKey,
			err: nil,
		},
		"valid ec private key": {
			pem: ecPrivateKey,
			err: nil,
		},
		"invalid private key": {
			pem: "INVALID TEST PRIVATE KEY DATA",
			err: errors.New("failed to locate private key"),
		},
	}

	for _, test := range tests {
		err := ValidatePrivateKey([]byte(test.pem))
		assert.Equal(t, err, test.err)
	}
}
