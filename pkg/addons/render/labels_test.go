package render

import "testing"

func TestDeriveLabels(t *testing.T) {
	cases := []struct {
		name string
		meta *Metadata
		want map[string]string
	}{
		{
			name: "ga stage produces no label yet",
			meta: &Metadata{Stage: StageGA},
			want: map[string]string{},
		},
		{
			name: "experimental stage",
			meta: &Metadata{Stage: StageExperimental},
			want: map[string]string{"addon.harvesterhci.io/experimental": "true"},
		},
		{
			name: "deprecated adds a separate label",
			meta: &Metadata{Stage: StageExperimental, Deprecated: true},
			want: map[string]string{
				"addon.harvesterhci.io/experimental": "true",
				"addon.harvesterhci.io/deprecated":   "true",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DeriveLabels(c.meta)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("label %s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestInjectLabelsNoExistingBlock(t *testing.T) {
	fragment := "apiVersion: harvesterhci.io/v1beta1\n" +
		"kind: Addon\n" +
		"metadata:\n" +
		"  name: harvester-seeder\n" +
		"  namespace: harvester-system\n" +
		"spec:\n" +
		"  chart: harvester-seeder\n"

	got, err := injectLabels(fragment, map[string]string{"addon.harvesterhci.io/experimental": "true"})
	if err != nil {
		t.Fatal(err)
	}

	want := "apiVersion: harvesterhci.io/v1beta1\n" +
		"kind: Addon\n" +
		"metadata:\n" +
		"  name: harvester-seeder\n" +
		"  namespace: harvester-system\n" +
		"  labels:\n" +
		"    addon.harvesterhci.io/experimental: \"true\"\n" +
		"spec:\n" +
		"  chart: harvester-seeder\n"

	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestInjectLabelsPreservesHandAuthoredLabel(t *testing.T) {
	fragment := "apiVersion: harvesterhci.io/v1beta1\n" +
		"kind: Addon\n" +
		"metadata:\n" +
		"  name: descheduler\n" +
		"  namespace: kube-system\n" +
		"  labels:\n" +
		"    addon.harvesterhci.io/displayName: \"virtual-machine-auto-balance\"\n" +
		"spec:\n" +
		"  chart: descheduler\n"

	got, err := injectLabels(fragment, map[string]string{"addon.harvesterhci.io/experimental": "true"})
	if err != nil {
		t.Fatal(err)
	}

	want := "apiVersion: harvesterhci.io/v1beta1\n" +
		"kind: Addon\n" +
		"metadata:\n" +
		"  name: descheduler\n" +
		"  namespace: kube-system\n" +
		"  labels:\n" +
		"    addon.harvesterhci.io/displayName: \"virtual-machine-auto-balance\"\n" +
		"    addon.harvesterhci.io/experimental: \"true\"\n" +
		"spec:\n" +
		"  chart: descheduler\n"

	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestInjectLabelsIdempotent(t *testing.T) {
	fragment := "apiVersion: harvesterhci.io/v1beta1\n" +
		"kind: Addon\n" +
		"metadata:\n" +
		"  name: kubeovn-operator\n" +
		"  namespace: kube-system\n" +
		"  labels:\n" +
		"    addon.harvesterhci.io/experimental: \"true\"\n" +
		"spec:\n" +
		"  chart: kubeovn-operator\n"

	got, err := injectLabels(fragment, map[string]string{"addon.harvesterhci.io/experimental": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if got != fragment {
		t.Errorf("expected no change, got:\n%s", got)
	}
}

func TestInjectLabelsNoLabels(t *testing.T) {
	fragment := "apiVersion: harvesterhci.io/v1beta1\nkind: Addon\n"
	got, err := injectLabels(fragment, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != fragment {
		t.Errorf("expected fragment unchanged when no labels given, got:\n%s", got)
	}
}
