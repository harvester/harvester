package upgrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	defaultNamespace = "harvester-system"
)

func TestGetUpgradableVersions(t *testing.T) {
	type input struct {
		newVersions    harvesterv1.Version
		respVersion    Version
		currentVersion string
	}
	type output struct {
		canUpgrades bool
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "no iso url specified",
			given: input{
				currentVersion: "v0.1.0",
				respVersion: Version{
					Name:                 "v0.2.0",
					MinUpgradableVersion: "v0.1.0",
					Tags:                 []string{"v0.2.0"},
				},
				newVersions: harvesterv1.Version{
					Spec: harvesterv1.VersionSpec{
						ISOURL:      "",
						ISOChecksum: "xxx",
					},
				},
			},
			expected: output{
				canUpgrades: false,
			},
		},
		{
			name: "no iso checksum specified",
			given: input{
				currentVersion: "v0.1.0",
				respVersion: Version{
					Name:                 "v0.2.0",
					MinUpgradableVersion: "v0.1.0",
					Tags:                 []string{"v0.2.0"},
				},
				newVersions: harvesterv1.Version{
					Spec: harvesterv1.VersionSpec{
						ISOURL:      "http://server/harvester.iso",
						ISOChecksum: "",
					},
				},
			},
			expected: output{
				canUpgrades: false,
			},
		},
		{
			name: "dev tag",
			given: input{
				currentVersion: "v0.1.0",
				respVersion: Version{
					Name:                 "v0.3.0",
					MinUpgradableVersion: "v0.2.0",
					Tags:                 []string{"v0.3.0", "dev"},
				},
				newVersions: harvesterv1.Version{
					Spec: harvesterv1.VersionSpec{
						ISOURL:      "http://server/harvester.iso",
						ISOChecksum: "xxxx",
					},
				},
			},
			expected: output{
				canUpgrades: true,
			},
		},
		{
			name: "current version name more than responder version name",
			given: input{
				currentVersion: "v0.2.0",
				respVersion: Version{
					Name:                 "v0.1.0",
					MinUpgradableVersion: "v0.1.0",
					Tags:                 []string{"v0.1.0"},
				},
				newVersions: harvesterv1.Version{
					Spec: harvesterv1.VersionSpec{
						ISOURL:      "http://server/harvester.iso",
						ISOChecksum: "xxxx",
					},
				},
			},
			expected: output{
				canUpgrades: false,
			},
		},
		{
			name: "current version name less than responder version name and no min upgradable version",
			given: input{
				currentVersion: "v0.2.0",
				respVersion: Version{
					Name:                 "v0.4.0",
					MinUpgradableVersion: "",
					Tags:                 []string{"v0.4.0"},
				},
				newVersions: harvesterv1.Version{
					Spec: harvesterv1.VersionSpec{
						ISOURL:      "http://server/harvester.iso",
						ISOChecksum: "xxxx",
					},
				},
			},
			expected: output{
				canUpgrades: true,
			},
		},
		{
			name: "current version name less than responder version name and min upgradable version not met",
			given: input{
				currentVersion: "v0.2.0",
				respVersion: Version{
					Name:                 "v0.4.0",
					MinUpgradableVersion: "v0.3.0",
					Tags:                 []string{"v0.4.0"},
				},
				newVersions: harvesterv1.Version{
					Spec: harvesterv1.VersionSpec{
						ISOURL:      "http://server/harvester.iso",
						ISOChecksum: "xxxx",
					},
				},
			},
			expected: output{
				canUpgrades: false,
			},
		},
		{
			name: "current version name less than responder version name and min upgradable version requirement met",
			given: input{
				currentVersion: "v0.2.0",
				respVersion: Version{
					Name:                 "v0.3.0",
					MinUpgradableVersion: "v0.2.0",
					Tags:                 []string{"v0.3.0"},
				},
				newVersions: harvesterv1.Version{
					Spec: harvesterv1.VersionSpec{
						ISOURL:      "http://server/harvester.iso",
						ISOChecksum: "xxxx",
					},
				},
			},
			expected: output{
				canUpgrades: true,
			},
		},
	}

	for _, tc := range testCases {
		nv := tc.given.newVersions
		assert.Equal(t, tc.expected.canUpgrades, canUpgrade(tc.given.currentVersion, &nv, tc.given.respVersion), "case %q", tc.name)
	}
}

func Test_formatQuantityToGi(t *testing.T) {
	type args struct {
		qs string
	}
	testCases := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test1",
			args: args{
				qs: "32920204Ki",
			},
			want: "32Gi",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := resource.ParseQuantity(tc.args.qs)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, formatQuantityToGi(&q))
		})
	}
}

func Test_syncVersions(t *testing.T) {
	versionObjs := []runtime.Object{
		&harvesterv1.Version{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "v1.4.0",
				Namespace: defaultNamespace,
			},
			Spec: harvesterv1.VersionSpec{
				ISOURL:               "server/harvester.iso",
				ISOChecksum:          "xxx",
				MinUpgradableVersion: "v1.3.0",
			},
		},
		&harvesterv1.Version{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "v1.3.0",
				Namespace: defaultNamespace,
			},
			Spec: harvesterv1.VersionSpec{
				ISOURL:               "server/harvester.iso",
				ISOChecksum:          "xxx",
				MinUpgradableVersion: "v1.2.2",
			},
		},
		&harvesterv1.Version{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "v1.3.1",
				Namespace: defaultNamespace,
			},
			Spec: harvesterv1.VersionSpec{
				ISOURL:               "",
				ISOChecksum:          "xxx",
				MinUpgradableVersion: "v1.2.2",
			},
		},
	}

	resp := CheckUpgradeResponse{
		Versions: []Version{
			{
				Name:                 "v1.1.0",
				MinUpgradableVersion: "v1.0.0",
				Tags:                 []string{"v1.1.0"},
			},
			{
				Name:                 "v1.1.1",
				MinUpgradableVersion: "v1.1.0",
				Tags:                 []string{"v1.1.1"},
			},
			{
				Name:                 "v1.1.2",
				MinUpgradableVersion: "v1.1.1",
				Tags:                 []string{"v1.1.2"},
			},
			{
				Name:                 "v1.2.1",
				MinUpgradableVersion: "v1.1.2",
				Tags:                 []string{"v1.2.1"},
			},
			{
				Name:                 "v1.3.0",
				MinUpgradableVersion: "v1.2.2",
				Tags:                 []string{"v1.2.2"},
			},
			{
				Name:                 "v1.3.0",
				MinUpgradableVersion: "v1.2.2",
				Tags:                 []string{"v1.2.2"},
			},
			{
				Name:                 "v1.3.1",
				MinUpgradableVersion: "v1.3.0",
				Tags:                 []string{"v1.3.0"},
			},
		},
	}
	server := fakeHTTPEndpoint(resp)
	server.Start()
	defer server.Close()
	err := settings.ReleaseDownloadURL.Set(server.URL)
	assert.Nil(t, err)
	client := fake.NewSimpleClientset(versionObjs...)
	vc := fakeclients.VersionClient(client.HarvesterhciV1beta1().Versions)
	assert := require.New(t)
	vs := versionSyncer{
		ctx:           context.TODO(),
		namespace:     defaultNamespace,
		httpClient:    server.Client(),
		versionClient: vc,
	}
	err = vs.syncVersions(resp, "v1.3.0")
	assert.NoError(err)
	versionList, err := vc.List(defaultNamespace, metav1.ListOptions{})
	assert.NoError(err)
	assert.Len(versionList.Items, 1, "expected to find 1 version object only")

}

type fakeResponder struct {
	resp CheckUpgradeResponse
}

func fakeHTTPEndpoint(resp CheckUpgradeResponse) *httptest.Server {
	r := mux.NewRouter()
	responder := fakeResponder{
		resp: resp,
	}
	r.HandleFunc("/{version}/version.yaml", responder.versionResponder)
	r.HandleFunc("/{version}/version-arm64.yaml", responder.versionResponder)
	s := httptest.NewUnstartedServer(r)
	return s
}

func (r fakeResponder) versionResponder(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	for _, v := range r.resp.Versions {
		if vars["version"] == v.Name {
			generateResponse(rw, v)
			return
		}
	}
	rw.WriteHeader(http.StatusNotFound)
}

func generateResponse(rw http.ResponseWriter, v Version) {
	harvesterVersion := harvesterv1.Version{
		ObjectMeta: metav1.ObjectMeta{
			Name: v.Name,
		},
		Spec: harvesterv1.VersionSpec{
			ISOURL:      "fakeurl/image.iso",
			ISOChecksum: "xxxx",
		},
	}
	err := json.NewEncoder(rw).Encode(&harvesterVersion)
	if err != nil {
		logrus.Errorf("error encoding response: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
}
