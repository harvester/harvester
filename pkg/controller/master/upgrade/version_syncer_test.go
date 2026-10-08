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
	"github.com/harvester/harvester/pkg/util"
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
	err = vs.syncVersions(resp, "v1.3.0", settings.ServerFlavorCommunity)
	assert.NoError(err)
	versionList, err := vc.List(defaultNamespace, metav1.ListOptions{})
	assert.NoError(err)

	// v1.3.0 can't be upgraded to and v1.3.1 has no ISO URL, so both are removed.
	// v1.3.1 is then recreated from the remote version. v1.4.0 has no remote version
	// and is kept.
	names := map[string]map[string]string{}
	for _, v := range versionList.Items {
		names[v.Name] = v.Labels
	}
	assert.Equal(map[string]map[string]string{
		"v1.3.1": {versionSourceLabel: versionSourceUpgradeChecker},
		"v1.4.0": nil,
	}, names)
}

func Test_cleanupVersions(t *testing.T) {
	newVersion := func(name, releaseDate string, labels map[string]string) *harvesterv1.Version {
		return &harvesterv1.Version{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: defaultNamespace,
				Labels:    labels,
			},
			Spec: harvesterv1.VersionSpec{
				ISOURL:      "server/harvester.iso",
				ISOChecksum: "xxx",
				ReleaseDate: releaseDate,
			},
		}
	}
	synced := map[string]string{versionSourceLabel: versionSourceUpgradeChecker}
	manual := map[string]string{versionSourceLabel: "manual"}
	communityRelease := map[string]string{util.LabelServerFlavor: settings.ServerFlavorCommunity}
	primeRelease := map[string]string{util.LabelServerFlavor: settings.ServerFlavorPrime}
	const releaseDate = "2026-09-01T00:00:00Z"

	tests := []struct {
		name           string
		existing       *harvesterv1.Version
		remote         []Version
		currentVersion string
		flavor         string
		kept           bool
		expectedLabels map[string]string
	}{
		{
			name:     "synced version that can't be upgraded to is removed",
			existing: newVersion("v1.9.0", "", synced),
			remote:   []Version{{Name: "v1.9.0"}},
		},
		{
			name:     "synced version that can be upgraded to is kept",
			existing: newVersion("v1.9.2", "", synced),
			remote:   []Version{{Name: "v1.9.0"}, {Name: "v1.9.2"}},
			kept:     true,
		},
		{
			name:     "version without a remote version is kept",
			existing: newVersion("v1.9.0", "", synced),
			remote:   []Version{{Name: "v1.8.0"}},
			kept:     true,
		},
		{
			name:     "unlabeled version created by an older release is removed",
			existing: newVersion("v1.9.1", "2026-09-01T00:00:00Z", nil),
			remote:   []Version{{Name: "v1.9.1", ReleaseDate: "2026-09-01T00:00:00Z"}},
		},
		{
			name:     "unlabeled version created by a user is kept",
			existing: newVersion("v1.9.1", "20260905", nil),
			remote:   []Version{{Name: "v1.9.1", ReleaseDate: "2026-09-01T00:00:00Z"}},
			kept:     true,
		},
		{
			name:     "version labeled by a user is kept",
			existing: newVersion("v1.9.1", "2026-09-01T00:00:00Z", manual),
			remote:   []Version{{Name: "v1.9.1", ReleaseDate: "2026-09-01T00:00:00Z"}},
			kept:     true,
		},
		{
			name:     "flavor-labeled version created by an older release is removed",
			existing: newVersion("v1.9.1", releaseDate, communityRelease),
			remote:   []Version{{Name: "v1.9.1", ReleaseDate: releaseDate}},
		},
		{
			name:     "Prime version imported on a community cluster is kept",
			existing: newVersion("v1.9.1", releaseDate, primeRelease),
			remote:   []Version{{Name: "v1.9.1", ReleaseDate: releaseDate}},
			kept:     true,
		},
		{
			name:     "imported Prime version is removed once the cluster is Prime",
			existing: newVersion("v1.9.1", releaseDate, primeRelease),
			remote:   []Version{{Name: "v1.9.1", ReleaseDate: releaseDate}},
			flavor:   settings.ServerFlavorPrime,
		},
		{
			name:     "unlabeled version on a Prime cluster is kept",
			existing: newVersion("v1.9.1", releaseDate, nil),
			remote:   []Version{{Name: "v1.9.1", ReleaseDate: releaseDate}},
			flavor:   settings.ServerFlavorPrime,
			kept:     true,
		},
		{
			name:           "upgradable version created by an older release is adopted",
			existing:       newVersion("v1.10.1", releaseDate, communityRelease),
			remote:         []Version{{Name: "v1.10.1", ReleaseDate: releaseDate}},
			kept:           true,
			expectedLabels: map[string]string{util.LabelServerFlavor: settings.ServerFlavorCommunity, versionSourceLabel: versionSourceUpgradeChecker},
		},
		{
			name:           "upgradable unlabeled version created by an older release is adopted",
			existing:       newVersion("v1.10.1", releaseDate, nil),
			remote:         []Version{{Name: "v1.10.1", ReleaseDate: releaseDate}},
			kept:           true,
			expectedLabels: synced,
		},
		{
			name:     "synced community version without a remote is removed on a Prime cluster",
			existing: newVersion("v1.10.0", releaseDate, map[string]string{util.LabelServerFlavor: settings.ServerFlavorCommunity, versionSourceLabel: versionSourceUpgradeChecker}),
			remote:   []Version{{Name: "v1.10.1", ReleaseDate: releaseDate}},
			flavor:   settings.ServerFlavorPrime,
		},
		{
			name:     "synced version without a flavor label is removed on a Prime cluster",
			existing: newVersion("v1.10.0", releaseDate, synced),
			remote:   []Version{{Name: "v1.10.0", ReleaseDate: releaseDate}},
			flavor:   settings.ServerFlavorPrime,
		},
		{
			name:     "synced Prime version is kept on a Prime cluster",
			existing: newVersion("v1.10.1", releaseDate, map[string]string{util.LabelServerFlavor: settings.ServerFlavorPrime, versionSourceLabel: versionSourceUpgradeChecker}),
			remote:   []Version{{Name: "v1.10.1", ReleaseDate: releaseDate}},
			flavor:   settings.ServerFlavorPrime,
			kept:     true,
		},
		{
			name:     "user-imported community version is kept on a Prime cluster",
			existing: newVersion("v1.10.0", "20260905", communityRelease),
			remote:   []Version{{Name: "v1.10.1", ReleaseDate: releaseDate}},
			flavor:   settings.ServerFlavorPrime,
			kept:     true,
		},
		{
			name:           "upgradable user-created version is not adopted",
			existing:       newVersion("v1.10.1", "20260905", nil),
			remote:         []Version{{Name: "v1.10.1", ReleaseDate: releaseDate}},
			kept:           true,
			expectedLabels: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(tc.existing)
			vc := fakeclients.VersionClient(client.HarvesterhciV1beta1().Versions)
			vs := versionSyncer{
				ctx:           context.TODO(),
				namespace:     defaultNamespace,
				versionClient: vc,
			}

			flavor := tc.flavor
			if flavor == "" {
				flavor = settings.ServerFlavorCommunity
			}
			require.NoError(t, vs.cleanupVersions("v1.9.1", flavor, tc.remote))

			versionList, err := vc.List(defaultNamespace, metav1.ListOptions{})
			require.NoError(t, err)
			require.Equal(t, tc.kept, len(versionList.Items) == 1)
			if tc.expectedLabels != nil {
				labels := versionList.Items[0].Labels
				if labels == nil {
					labels = map[string]string{}
				}
				assert.Equal(t, tc.expectedLabels, labels)
			}
		})
	}
}

// Test_syncVersionsAfterFlavorSwitch covers a cluster that switched from community
// to Prime with a synced community Version sharing its name with a Prime remote
// version: the community Version is replaced by the Prime one.
func Test_syncVersionsAfterFlavorSwitch(t *testing.T) {
	resp := CheckUpgradeResponse{
		Versions: []Version{
			{Name: "v1.10.1", MinUpgradableVersion: "v1.9.0", Tags: []string{"v1.10.1", "community", "prime"}},
		},
	}
	server := fakeHTTPEndpoint(resp)
	server.Start()
	defer server.Close()
	require.NoError(t, settings.ReleaseDownloadURL.Set(server.URL))

	client := fake.NewSimpleClientset(&harvesterv1.Version{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "v1.10.1",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				util.LabelServerFlavor: settings.ServerFlavorCommunity,
				versionSourceLabel:     versionSourceUpgradeChecker,
			},
		},
		Spec: harvesterv1.VersionSpec{
			ISOURL:      "community/harvester.iso",
			ISOChecksum: "xxx",
		},
	})
	vc := fakeclients.VersionClient(client.HarvesterhciV1beta1().Versions)
	vs := versionSyncer{
		ctx:           context.TODO(),
		namespace:     defaultNamespace,
		httpClient:    server.Client(),
		versionClient: vc,
	}

	require.NoError(t, vs.syncVersions(resp, "v1.9.1", settings.ServerFlavorPrime))

	version, err := vc.Get(defaultNamespace, "v1.10.1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "fakeurl/image.iso", version.Spec.ISOURL)
}

func Test_filterVersionsByFlavor(t *testing.T) {
	versions := []Version{
		{Name: "v1.6.0", Tags: []string{"v1.6.0"}},
		{Name: "v1.6.1", Tags: []string{"v1.6.1", "community"}},
		{Name: "v1.6.2", Tags: []string{"v1.6.2", "prime"}},
		{Name: "v1.7.0", Tags: []string{"v1.7.0", "prime", "community"}},
		{Name: "v1.7.1", Tags: []string{"prime", "dev"}},
		{Name: "v1.7.2", Tags: []string{"dev"}},
		{Name: "v1.7.3"},
	}

	tests := []struct {
		name     string
		flavor   string
		expected []string
	}{
		{
			name:     "community keeps only community versions",
			flavor:   settings.ServerFlavorCommunity,
			expected: []string{"v1.6.1", "v1.7.0"},
		},
		{
			name:     "empty flavor is treated as community",
			flavor:   "",
			expected: []string{"v1.6.1", "v1.7.0"},
		},
		{
			name:     "prime keeps only prime versions",
			flavor:   settings.ServerFlavorPrime,
			expected: []string{"v1.6.2", "v1.7.0", "v1.7.1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var names []string
			for _, v := range filterVersionsByFlavor(versions, tc.flavor) {
				names = append(names, v.Name)
			}
			assert.Equal(t, tc.expected, names)
		})
	}
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
