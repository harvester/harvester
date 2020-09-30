package auth

import (
	"context"
	"fmt"
	"testing"

	"github.com/rancher/harvester/pkg/auth/jwe"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/settings"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	corev1type "k8s.io/client-go/kubernetes/typed/core/v1"
)

func TestRefreshKeyInTokenManager(t *testing.T) {
	type input struct {
		secret *corev1.Secret
	}
	type output struct {
		errorMessage string
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "update secret to valid private/public key",
			given: input{
				secret: validUpdatedSecret,
			},
			expected: output{
				errorMessage: "",
			},
		},
		{
			name: "update secret to invalid private/public key",
			given: input{
				secret: invalidUpdatedSecret,
			},
			expected: output{
				errorMessage: fmt.Sprintf("Failed to parse rsa key from secret %s/%s: Failed to parse PEM block containing the key", namespace, settings.AuthSecretName.Get()),
			},
		},
	}

	clientSet := fake.NewSimpleClientset()
	err := clientSet.Tracker().Add(iniSecret)
	assert.Nil(t, err, "Mock resource should add into fake controller tracker")

	secretClient := fakeSecretClient(clientSet.CoreV1().Secrets)
	tokenManager, err := jwe.NewJWETokenManager(secretClient, namespace)
	assert.Nil(t, err, "NewJWETokenManager should return no error")
	scaled := &config.Scaled{TokenManager: tokenManager}

	for _, tc := range testCases {
		_, err := secretClient.Update(tc.given.secret)
		assert.Nil(t, err, "case %q", tc.name)

		err = refreshKeyInTokenManager(tc.given.secret, scaled)
		if tc.expected.errorMessage == "" {
			assert.Nil(t, err, "case %q", tc.name)
		} else {
			assert.EqualError(t, err, tc.expected.errorMessage, "case %q", tc.name)
		}
	}

}

type fakeSecretClient func(string) corev1type.SecretInterface

func (c fakeSecretClient) Create(secret *corev1.Secret) (*corev1.Secret, error) {
	return c(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
}

func (c fakeSecretClient) Update(secret *corev1.Secret) (*corev1.Secret, error) {
	return c(secret.Namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
}

func (c fakeSecretClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c fakeSecretClient) Get(namespace, name string, options metav1.GetOptions) (*corev1.Secret, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c fakeSecretClient) List(namespace string, opts metav1.ListOptions) (*corev1.SecretList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeSecretClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeSecretClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*corev1.Secret, error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

var (
	namespace = "test"
	iniSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      settings.AuthSecretName.Get(),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"priv": []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAvz2eBXUUD+G+8oqxyeYPGSIEq10B/7P91HNvCa3bm73s5WuN
4Tdb/+w1TZtGwlW6xAyp+4qL3oz1ncGCciam87EeT/YzIxUok3T8Fg/AjdtZCXLU
z0YbO1h6p4ERgfTSh/k6tJfo/sk8R2TmXFoylvyUG+4q/uuyFsH8L/7VS7gtqZgk
6dvyo6pAkkaSnmOQ9fovKnb7zYYVfIVqxCWiBeczo9C3XekIxSBReX1K5MoD/Jsm
RYy1vN6IBhXvXpC6h6vYifLIuFQxUUDSdy6QVpZvxVwqjeXxt11QRkfL5gZicOU2
poRbVGAtUvL4bYbUuiIHdW6pTHPbQ4VXdR8gxQIDAQABAoIBAQChRDOaRIV7cxFT
dFPCfudieUZYv2CtITo+Sm1rSxnytnErccw+DDHfmW/FlthPjn2lT0yUWtvh+xow
QvteiWul+kkmguXSgsgpuK/PZs8okLz/c78zXtod7FnaIMQDw9E5apjvq16GZGoZ
hmOfo0wf+LRo7SmCuH0AJeslCg8R/rCO765IKF5BVtwu82tJAulaOI4SRCQPrNir
LrbU8kK6Qe+Ly02jskCaeeYsfSbw1afupzuJqOs/j3T62N0qys8VtEAbGtkNRr20
gvAsLWZnvVb+IRakG6OsW79HC2KIUp5EiL7oYKrICrwihhev0o28TgY3GxExTEPB
Y+q2P4bNAoGBAMD4JQQZSZKQw4aWwz1KjwhUr0fyFvlNrBqkj+9kMy+BMhOAVQhN
4CusNRVWGXtry9S9fBJXpXUVpD7iYmVvdkSl1OkgqOGj/nDfO0TNuBXlfykGZMW5
UtKpvSDd034SQntCQNKl9H3HHJic+LZ+ejqTH5GWVk0P0hOsROp+dhVvAoGBAP20
7Wr+9xP2jAB9j1uDnYdqcVz8c5pBs4pAp5wIl5QV8qpjfx+ij8yNYncleQ51AtTu
PMFxmZy6/EkVHEvqKl9sOv3/G+dnMntn0Fzrz+okgxtHlGK+tZOqN1h/MNsqxFvh
lJL8fgHpWHsObvq7cyJHwFU+/90bu1StGBjVDpsLAoGAcXg5A/z+p0Gax+SVL9BM
5SAu5cZ0PeqvfgcwYBtygceds5vt5HEulV+w4zf6yflsJU+6ymphb8TnDNc/9teh
GuLMnL1IsU4miyapCl9RlQabTHtm/GFqU1feT5pBB8wi7anaxkMxzlgr942uLlmW
9CSZFpnpa20XIdxVtfHg698CgYBH8ECN4UQAFh22mePHaDeHyUfhvPeumsilABZG
qS0J4XtQkyvdtYOe1cxAypBb6BPoerEhjOuoxGB6/JBsejaPninQEcFAyUNIOLSd
VIQ8+SNv3ckWgssL1u0gm9gnnSXWg81ULGIyeo8LPZl8YSCRbNT9lwKIGK/yn65A
hFFC5wKBgEl6SOjTyllOYtEkQrv90P8pgg/8kMnlzFY6I+Eo8QVrxgNcRQECn99s
37O6qgVagVCYcPFpIxiI18BbEkDu0ZCd9c9nHC2bM/nzwpzDrQoxsNMJ51PLfs4V
y8ObfLnp+dEPSI0KLS9IIqFqfVRiwvRdhq6GuetVDQqfZEF1rnGz
-----END RSA PRIVATE KEY-----`),
			"pub": []byte(`-----BEGIN RSA PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAvz2eBXUUD+G+8oqxyeYP
GSIEq10B/7P91HNvCa3bm73s5WuN4Tdb/+w1TZtGwlW6xAyp+4qL3oz1ncGCciam
87EeT/YzIxUok3T8Fg/AjdtZCXLUz0YbO1h6p4ERgfTSh/k6tJfo/sk8R2TmXFoy
lvyUG+4q/uuyFsH8L/7VS7gtqZgk6dvyo6pAkkaSnmOQ9fovKnb7zYYVfIVqxCWi
Beczo9C3XekIxSBReX1K5MoD/JsmRYy1vN6IBhXvXpC6h6vYifLIuFQxUUDSdy6Q
VpZvxVwqjeXxt11QRkfL5gZicOU2poRbVGAtUvL4bYbUuiIHdW6pTHPbQ4VXdR8g
xQIDAQAB
-----END RSA PUBLIC KEY-----`),
		},
	}

	validUpdatedSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      settings.AuthSecretName.Get(),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"priv": []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEAvGln3e2HeHYpZ3GlhsqZNJJK1QXC0z+x7JvEJ5no1NH4V7sm
JiBcv5JxFiBJCM6n0drPgQvexg/EVbhwsNT387UlaPD/be8HWBpCOHTo4Xet3I2b
suh8vbnQR/iv7XRr0DZBTLuQI/bC+tPBnPw6ecyTmo+wv9N97efOUGPbmvwdzoQE
TedjJCeuGligmzog0xaCQpl0tR6Ol/ZmWtSB9hlrBXoSkAg6hUQN6/cd2ToAaMdV
qqNYDgwY5vYosjiXSIAmVcCnW1lklN2sU+BM/yxx1CzVeMa+VqAc8v2lUCNZjjCY
5si7yb2RxwxNfZnBNicJb2MFJsESbIW5y30mZwIDAQABAoIBAQCPxH6VQaKVsNR3
Mqjz6bRuARNI6VR4janeuh07ep3Hh1DQ4OWDQj6Dj+Lq10fjiI1V/HlKJxyeVXmj
T1HuHRP2ysr5AKxn3nTkLWVKXys0oHXyTbv5EJ3ex+K+iGz17Fg4UK4TNywNxUWS
z/J1L6IPPqOC1RIxzdfRqYgsn4X7BAcIK5mhsy2G4GayPMzDhZ8psFLbewy+vp8/
Hulh1+zKYbDfQYSK4MnJcr2ZqIdk4WJPu2DYDLl8mgOLxxr62CoUdTUDes27VcdA
XSynjTODcqOb9f38g8JEL3jAoUPqQRIj3rKr9D5ZiRGtkDsEUZhyN0u5xdTbRYQI
Jcsb/ydBAoGBANUV6KSuajgC3l0/oi6CZFZlasRkSw2GtUQ5LyDhVQsU1uqfyWl+
LEceksqg8fC68EnF7fsIDJCA6LL/IBsroM76wCxizojacxLQEXDUX5PVVL2Wjq2L
AFjm8/h7oQrZSegRw+Sn1OptBUaF+GoXVTCHbIYa+6ZFYhMPFsqGtSpzAoGBAOJb
Yib/ioX0SgZDvYkm86xKSirTdt0os5cg6DyAfoeJQjK+oPSwMul6nRPjK/6mP9kI
BBDHS7Q26dEXkwS19b7R3o+hXhD1G4uvuZ0l2KJ4z2+xGSuPkrgfG6yizZIXe/cN
Bua6Wce1YxgmvB9ACN8hPFiRnwiLesR8YoJSxpM9AoGBAJLTY9iFrf8mSt5qCHCP
vF+jxivJB8YsOh7mYEkBuz3FgElvDLO6Evx2XqNsvwknZocO8Wp2I2I20SD1lsPi
Dg5QzbZH5xR5oa0m3b2nOKx+5MM2SN3f179qdFWVqmP1UW2tQBQAaT+XG3l6uq8v
oK2twuOtGBV73ZZQYV3v8EltAoGBANV7HEHthka508q+vpYIn44hbnuffp4sUdw5
0+2jvjGz2TQkp4a+WvXqhxSHjymWv+a/cZ4laBeqJrDly+mIdyGlq4LIzP+vO3Bt
peA5Hmx1Biav3y4/NT/jTuVtkfWzol2o8pZOsHfycWgIuCm86eEO5mwdwuB7M6j2
Kq4AxXl9AoGAcTNCmTZIm6vUC4v2TotvT23s5A0Ej8OkUbSjSMGpB5f+M72dm8G/
oGdCxqAulka2lgnefbGYrW+mx0B5ywxfHIRkhc/fZZ1FQA07i460zaawUPYbsK/g
yY/4Vz3W8xxWby2vl3oMMU1cXjT3LNS7JpWFv0aiNvNJDWH+mgzKmeg=
-----END RSA PRIVATE KEY-----`),
			"pub": []byte(`-----BEGIN RSA PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAvGln3e2HeHYpZ3GlhsqZ
NJJK1QXC0z+x7JvEJ5no1NH4V7smJiBcv5JxFiBJCM6n0drPgQvexg/EVbhwsNT3
87UlaPD/be8HWBpCOHTo4Xet3I2bsuh8vbnQR/iv7XRr0DZBTLuQI/bC+tPBnPw6
ecyTmo+wv9N97efOUGPbmvwdzoQETedjJCeuGligmzog0xaCQpl0tR6Ol/ZmWtSB
9hlrBXoSkAg6hUQN6/cd2ToAaMdVqqNYDgwY5vYosjiXSIAmVcCnW1lklN2sU+BM
/yxx1CzVeMa+VqAc8v2lUCNZjjCY5si7yb2RxwxNfZnBNicJb2MFJsESbIW5y30m
ZwIDAQAB
-----END RSA PUBLIC KEY-----`),
		},
	}

	invalidUpdatedSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      settings.AuthSecretName.Get(),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"priv": []byte("invalid"),
			"pub":  []byte("invalid"),
		},
	}
)
