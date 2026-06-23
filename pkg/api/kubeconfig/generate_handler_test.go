package kubeconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_getServerURL(t *testing.T) {
	tests := []struct {
		name    string
		vip     string
		want    string
		wantErr bool
	}{
		{
			name:    "IPv4 VIP produces correct URL",
			vip:     "192.168.1.10",
			want:    "https://192.168.1.10:6443",
			wantErr: false,
		},
		{
			name:    "IPv6 VIP produces RFC-3986-compliant bracketed URL",
			vip:     "2001:db8::1",
			want:    "https://[2001:db8::1]:6443",
			wantErr: false,
		},
		{
			name:    "invalid VIP string returns error",
			vip:     "not-an-ip",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vip",
					Namespace: "harvester-system",
				},
				Data: map[string]string{
					"ip": tt.vip,
				},
			}
			err := clientset.Tracker().Add(cm)
			assert.Nil(t, err)

			h := &GenerateHandler{
				namespace:      "harvester-system",
				configMapCache: fakeclients.ConfigmapCache(clientset.CoreV1().ConfigMaps),
			}

			got, err := h.getServerURL()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
