package supportbundle

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	supportBundleUtil "github.com/harvester/harvester/pkg/util/supportbundle"
)

func intPtr(i int) *int {
	return &i
}

func TestDetermineNodeTimeout(t *testing.T) {
	tests := []struct {
		name           string
		specTimeout    int
		settingValue   string
		expectedResult time.Duration
		description    string
	}{
		{
			name:           "spec.NodeTimeout has value, should use it directly",
			specTimeout:    45,
			settingValue:   "60", // setting has different value
			expectedResult: 45 * time.Minute,
			description:    "When spec.NodeTimeout is > 0, it should be used regardless of settings",
		},
		{
			name:           "spec.NodeTimeout is 0, use settings value",
			specTimeout:    0,
			settingValue:   "90",
			expectedResult: 90 * time.Minute,
			description:    "When spec.NodeTimeout is 0, settings value should be used",
		},
		{
			name:           "spec.NodeTimeout is 0, settings is 0, use default",
			specTimeout:    0,
			settingValue:   "0",
			expectedResult: time.Duration(supportBundleUtil.SupportBundleNodeCollectionTimeoutDefault) * time.Minute,
			description:    "When both spec.NodeTimeout and settings are 0, default value should be used",
		},
		{
			name:           "spec.NodeTimeout is 0, settings is empty, use default",
			specTimeout:    0,
			settingValue:   "",
			expectedResult: time.Duration(supportBundleUtil.SupportBundleNodeCollectionTimeoutDefault) * time.Minute,
			description:    "When spec.NodeTimeout is 0 and settings is empty, default value should be used",
		},
		{
			name:           "spec.NodeTimeout has large value",
			specTimeout:    1440, // 24 hours
			settingValue:   "30",
			expectedResult: 1440 * time.Minute,
			description:    "Large spec.NodeTimeout values should be handled correctly",
		},
		{
			name:           "spec.NodeTimeout is 1, minimum positive value",
			specTimeout:    1,
			settingValue:   "30",
			expectedResult: 1 * time.Minute,
			description:    "Minimum positive spec.NodeTimeout should be used",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleNodeCollectionTimeout.Set(tt.settingValue)

			// Create test SupportBundle
			sb := &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sb",
					Namespace: "test-namespace",
				},
				Spec: harvesterv1.SupportBundleSpec{
					NodeTimeout: tt.specTimeout,
					Description: "test support bundle",
				},
			}

			// Execute
			result := determineNodeTimeout(sb)

			// Assert
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

func TestDetermineNodeTimeoutEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func() *harvesterv1.SupportBundle
		settingValue   string
		expectedResult time.Duration
		description    string
	}{
		{
			name: "nil support bundle spec",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					// Spec is not explicitly set, so NodeTimeout should be 0
				}
			},
			settingValue:   "45",
			expectedResult: 45 * time.Minute,
			description:    "When spec is empty, NodeTimeout should be 0 and settings should be used",
		},
		{
			name: "negative setting value treated as 0",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						NodeTimeout: 0,
						Description: "test support bundle",
					},
				}
			},
			settingValue:   "-10", // This would be handled by GetInt() method
			expectedResult: time.Duration(supportBundleUtil.SupportBundleNodeCollectionTimeoutDefault) * time.Minute,
			description:    "Negative or invalid setting values should fall back to default",
		},
		{
			name: "invalid setting value falls back to default",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						NodeTimeout: 0,
						Description: "test support bundle",
					},
				}
			},
			settingValue:   "invalid-number",
			expectedResult: time.Duration(supportBundleUtil.SupportBundleNodeCollectionTimeoutDefault) * time.Minute,
			description:    "Invalid setting values should fall back to default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleNodeCollectionTimeout.Set(tt.settingValue)

			// Create test SupportBundle
			sb := tt.setupFunc()

			// Execute
			result := determineNodeTimeout(sb)

			// Assert
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

func TestDetermineExpiration(t *testing.T) {
	tests := []struct {
		name           string
		specExpiration int
		settingValue   string
		expectedResult time.Duration
		description    string
	}{
		{
			name:           "spec.Expiration has value, should use it directly",
			specExpiration: 120,
			settingValue:   "60", // setting has different value
			expectedResult: 120 * time.Minute,
			description:    "When spec.Expiration is > 0, it should be used regardless of settings",
		},
		{
			name:           "spec.Expiration is 0, use settings value",
			specExpiration: 0,
			settingValue:   "90",
			expectedResult: 90 * time.Minute,
			description:    "When spec.Expiration is 0, settings value should be used",
		},
		{
			name:           "spec.Expiration is 0, settings is 0, use default",
			specExpiration: 0,
			settingValue:   "0",
			expectedResult: time.Duration(supportBundleUtil.SupportBundleExpirationDefault) * time.Minute,
			description:    "When both spec.Expiration and settings are 0, default value should be used",
		},
		{
			name:           "spec.Expiration is 0, settings is empty, use default",
			specExpiration: 0,
			settingValue:   "",
			expectedResult: time.Duration(supportBundleUtil.SupportBundleExpirationDefault) * time.Minute,
			description:    "When spec.Expiration is 0 and settings is empty, default value should be used",
		},
		{
			name:           "spec.Expiration has large value",
			specExpiration: 2880, // 48 hours
			settingValue:   "30",
			expectedResult: 2880 * time.Minute,
			description:    "Large spec.Expiration values should be handled correctly",
		},
		{
			name:           "spec.Expiration is 1, minimum positive value",
			specExpiration: 1,
			settingValue:   "30",
			expectedResult: 1 * time.Minute,
			description:    "Minimum positive spec.Expiration should be used",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleExpiration.Set(tt.settingValue)

			// Create test SupportBundle
			sb := &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sb",
					Namespace: "test-namespace",
				},
				Spec: harvesterv1.SupportBundleSpec{
					Expiration:  tt.specExpiration,
					Description: "test support bundle",
				},
			}

			// Execute
			result := determineExpiration(sb)

			// Assert
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

func TestDetermineExpirationEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func() *harvesterv1.SupportBundle
		settingValue   string
		expectedResult time.Duration
		description    string
	}{
		{
			name: "nil support bundle spec",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					// Spec is not explicitly set, so Expiration should be 0
				}
			},
			settingValue:   "45",
			expectedResult: 45 * time.Minute,
			description:    "When spec is empty, Expiration should be 0 and settings should be used",
		},
		{
			name: "negative setting value treated as 0",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						Expiration:  0,
						Description: "test support bundle",
					},
				}
			},
			settingValue:   "-10", // This would be handled by GetInt() method
			expectedResult: time.Duration(supportBundleUtil.SupportBundleExpirationDefault) * time.Minute,
			description:    "Negative or invalid setting values should fall back to default",
		},
		{
			name: "invalid setting value falls back to default",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						Expiration:  0,
						Description: "test support bundle",
					},
				}
			},
			settingValue:   "invalid-number",
			expectedResult: time.Duration(supportBundleUtil.SupportBundleExpirationDefault) * time.Minute,
			description:    "Invalid setting values should fall back to default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleExpiration.Set(tt.settingValue)

			// Create test SupportBundle
			sb := tt.setupFunc()

			// Execute
			result := determineExpiration(sb)

			// Assert
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

func TestDetermineTimeout(t *testing.T) {
	tests := []struct {
		name           string
		specTimeout    *int
		settingValue   string
		expectedResult time.Duration
		expectedError  bool
		description    string
	}{
		{
			name:           "spec.Timeout has value, should use it directly",
			specTimeout:    intPtr(60),
			settingValue:   "30", // setting has different value
			expectedResult: 60 * time.Minute,
			expectedError:  false,
			description:    "When spec.Timeout is > 0, it should be used regardless of settings",
		},
		{
			name:           "spec.Timeout is 0, should use it directly",
			specTimeout:    intPtr(0),
			settingValue:   "45",
			expectedResult: 0,
			expectedError:  false,
			description:    "When spec.Timeout is 0, settings value should be used",
		},
		{
			name:           "spec.Timeout is not set, settings is 0, use settings 0",
			specTimeout:    nil,
			settingValue:   "20",
			expectedResult: 20 * time.Minute,
			expectedError:  false,
			description:    "When spec.Timeout is 0 and settings is '0', should use 0 timeout",
		},
		{
			name:           "spec.Timeout is 1, minimum positive value",
			specTimeout:    intPtr(1),
			settingValue:   "30",
			expectedResult: 1 * time.Minute,
			expectedError:  false,
			description:    "Minimum positive spec.Timeout should be used",
		},
		{
			name:           "spec.Timeout is not set, invalid setting value should return error",
			specTimeout:    nil,
			settingValue:   "invalid-number",
			expectedResult: 0,
			expectedError:  true,
			description:    "Invalid setting values should return an error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleTimeout.Set(tt.settingValue)

			// Create test SupportBundle
			sb := &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sb",
					Namespace: "test-namespace",
				},
				Spec: harvesterv1.SupportBundleSpec{
					Timeout:     tt.specTimeout,
					Description: "test support bundle",
				},
			}

			// Execute
			result, err := determineTimeout(sb)

			// Assert
			if tt.expectedError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.expectedResult, result, tt.description)
			}
		})
	}
}

func TestDetermineTimeoutEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func() *harvesterv1.SupportBundle
		settingValue   string
		expectedResult time.Duration
		expectedError  bool
		description    string
	}{
		{
			name: "nil support bundle spec",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					// Spec is not explicitly set, so Timeout should be 0
				}
			},
			settingValue:   "45",
			expectedResult: 45 * time.Minute,
			expectedError:  false,
			description:    "When spec is empty, Timeout should be 0 and settings should be used",
		},
		{
			name: "empty spec with empty settings",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						Timeout:     nil,
						Description: "test support bundle",
					},
				}
			},
			settingValue:   "",
			expectedResult: 50 * time.Minute, // Default value
			expectedError:  false,
			description:    "When both spec.Timeout and settings are empty, no timeout should be applied",
		},
		{
			name: "float-like setting value should return error",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						Timeout:     nil,
						Description: "test support bundle",
					},
				}
			},
			settingValue:   "45.5",
			expectedResult: 0,
			expectedError:  true,
			description:    "Float-like setting values should return an error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleTimeout.Set(tt.settingValue)

			// Create test SupportBundle
			sb := tt.setupFunc()

			// Execute
			result, err := determineTimeout(sb)

			// Assert
			if tt.expectedError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.expectedResult, result, tt.description)
			}
		})
	}
}

func TestGetCollectNamespaces(t *testing.T) {
	// Create a manager instance for testing
	manager := &Manager{}

	tests := []struct {
		name                    string
		specExtraNamespaces     []string
		settingValue            string
		expectedExtraNamespaces []string
		description             string
	}{
		{
			name:                    "spec.ExtraCollectionNamespaces has values, should use them",
			specExtraNamespaces:     []string{"custom-ns1", "custom-ns2"},
			settingValue:            "setting-ns1,setting-ns2", // setting has different namespaces
			expectedExtraNamespaces: []string{"custom-ns1", "custom-ns2"},
			description:             "When spec.ExtraCollectionNamespaces has values, they should be used regardless of settings",
		},
		{
			name:                    "spec.ExtraCollectionNamespaces is empty, use settings value",
			specExtraNamespaces:     []string{}, // empty spec
			settingValue:            "setting-ns1,setting-ns2",
			expectedExtraNamespaces: []string{"setting-ns1", "setting-ns2"}, // settings should be split into separate namespaces
			description:             "When spec.ExtraCollectionNamespaces is empty, settings value should be used",
		},
		{
			name:                    "spec.ExtraCollectionNamespaces is nil, use settings value",
			specExtraNamespaces:     nil, // nil spec
			settingValue:            "setting-ns3",
			expectedExtraNamespaces: []string{"setting-ns3"},
			description:             "When spec.ExtraCollectionNamespaces is nil, settings value should be used",
		},
		{
			name:                    "both spec and settings are empty, no extra namespaces",
			specExtraNamespaces:     []string{},
			settingValue:            "",
			expectedExtraNamespaces: []string{},
			description:             "When both spec.ExtraCollectionNamespaces and settings are empty, no extra namespaces should be added",
		},
		{
			name:                    "spec has single namespace",
			specExtraNamespaces:     []string{"single-custom-ns"},
			settingValue:            "setting-ns",
			expectedExtraNamespaces: []string{"single-custom-ns"},
			description:             "Single namespace in spec should work correctly",
		},
		{
			name:                    "spec is nil, settings is empty, no extra namespaces",
			specExtraNamespaces:     nil,
			settingValue:            "",
			expectedExtraNamespaces: []string{},
			description:             "When spec.ExtraCollectionNamespaces is nil and settings is empty, no extra namespaces should be added",
		},
	}

	// Define expected base namespaces for verification
	expectedBaseNamespaces := []string{
		"cattle-dashboards",
		"cattle-fleet-local-system",
		"cattle-fleet-system",
		"cattle-fleet-clusters-system",
		"cattle-monitoring-system",
		"fleet-local",
		"harvester-system",
		"local",
		"longhorn-system",
		"cattle-logging-system",
		"cattle-provisioning-capi-system",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleNamespaces.Set(tt.settingValue)

			// Create test SupportBundle
			sb := &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sb",
					Namespace: "test-namespace",
				},
				Spec: harvesterv1.SupportBundleSpec{
					ExtraCollectionNamespaces: tt.specExtraNamespaces,
					Description:               "test support bundle",
				},
			}

			// Execute
			result := manager.getCollectNamespaces(sb)

			// Parse result
			resultNamespaces := strings.Split(result, ",")

			// Verify base namespaces are always present
			for _, expectedNS := range expectedBaseNamespaces {
				assert.Contains(t, resultNamespaces, expectedNS, "Base namespace %s should always be included", expectedNS)
			}

			// Verify extra namespaces
			if len(tt.expectedExtraNamespaces) == 0 {
				// Should only contain base namespaces
				assert.Len(t, resultNamespaces, len(expectedBaseNamespaces), tt.description)
			} else {
				// Should contain base + extra namespaces
				expectedTotal := len(expectedBaseNamespaces) + len(tt.expectedExtraNamespaces)
				assert.Len(t, resultNamespaces, expectedTotal, tt.description)

				// Verify each expected extra namespace is present
				for _, expectedExtra := range tt.expectedExtraNamespaces {
					assert.Contains(t, resultNamespaces, expectedExtra, "Extra namespace %s should be included: %s", expectedExtra, tt.description)
				}
			}
		})
	}
}

func TestGetCollectNamespacesEdgeCases(t *testing.T) {
	manager := &Manager{}

	tests := []struct {
		name             string
		setupFunc        func() *harvesterv1.SupportBundle
		settingValue     string
		shouldContain    []string
		shouldNotContain []string
		description      string
	}{
		{
			name: "spec with duplicate namespaces",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						ExtraCollectionNamespaces: []string{"duplicate-ns", "duplicate-ns", "unique-ns"},
						Description:               "test support bundle",
					},
				}
			},
			settingValue:     "setting-ns",
			shouldContain:    []string{"duplicate-ns", "unique-ns"},
			shouldNotContain: []string{"setting-ns"}, // should not use settings when spec has values
			description:      "Duplicate namespaces in spec should be de-duplicated",
		},
		{
			name: "spec with empty string namespace",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						ExtraCollectionNamespaces: []string{"", "valid-ns"},
						Description:               "test support bundle",
					},
				}
			},
			settingValue:     "setting-ns",
			shouldContain:    []string{"valid-ns", ""}, // empty string should be included as-is
			shouldNotContain: []string{"setting-ns"},
			description:      "Empty string in spec namespaces should be handled",
		},
		{
			name: "spec with duplicate of base namespace",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						ExtraCollectionNamespaces: []string{"harvester-system", "custom-ns"}, // harvester-system is already in base
						Description:               "test support bundle",
					},
				}
			},
			settingValue:     "setting-ns",
			shouldContain:    []string{"harvester-system", "custom-ns"}, // both should be present
			shouldNotContain: []string{"setting-ns"},
			description:      "Duplicate of base namespace should be de-duplicated",
		},
		{
			name: "settings with duplicate namespaces",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						ExtraCollectionNamespaces: []string{}, // empty to use settings
						Description:               "test support bundle",
					},
				}
			},
			settingValue:     "setting-ns1,setting-ns1,setting-ns2", // duplicate setting-ns1
			shouldContain:    []string{"setting-ns1", "setting-ns2"},
			shouldNotContain: []string{},
			description:      "Duplicate namespaces in settings should be de-duplicated",
		},
		{
			name: "settings with whitespace and duplicates",
			setupFunc: func() *harvesterv1.SupportBundle {
				return &harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sb",
						Namespace: "test-namespace",
					},
					Spec: harvesterv1.SupportBundleSpec{
						ExtraCollectionNamespaces: []string{}, // empty to use settings
						Description:               "test support bundle",
					},
				}
			},
			settingValue:     " setting-ns1 , setting-ns1, setting-ns2 ", // whitespace and duplicates
			shouldContain:    []string{"setting-ns1", "setting-ns2"},
			shouldNotContain: []string{" setting-ns1 ", " setting-ns2 "}, // should not contain whitespace versions
			description:      "Settings with whitespace and duplicates should be handled correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleNamespaces.Set(tt.settingValue)

			// Create test SupportBundle
			sb := tt.setupFunc()

			// Execute
			result := manager.getCollectNamespaces(sb)
			resultNamespaces := strings.Split(result, ",")

			// Verify should contain
			for _, expected := range tt.shouldContain {
				assert.Contains(t, resultNamespaces, expected, "Should contain namespace %s: %s", expected, tt.description)
			}

			// Verify should not contain
			for _, notExpected := range tt.shouldNotContain {
				assert.NotContains(t, resultNamespaces, notExpected, "Should not contain namespace %s: %s", notExpected, tt.description)
			}
		})
	}
}

func TestGetCollectNamespacesDuplication(t *testing.T) {
	manager := &Manager{}

	tests := []struct {
		name                string
		specExtraNamespaces []string
		settingValue        string
		expectedCount       map[string]int // expected count of each namespace in final result
		description         string
	}{
		{
			name:                "spec duplicates should be de-duplicated",
			specExtraNamespaces: []string{"ns1", "ns1", "ns2", "ns1", "ns3"},
			settingValue:        "setting-ns",
			expectedCount: map[string]int{
				"ns1": 1, // should appear only once despite 3 occurrences
				"ns2": 1,
				"ns3": 1,
			},
			description: "Multiple duplicates in spec should result in single occurrence",
		},
		{
			name:                "spec duplicate of base namespace",
			specExtraNamespaces: []string{"harvester-system", "longhorn-system", "custom-ns"},
			settingValue:        "setting-ns",
			expectedCount: map[string]int{
				"harvester-system": 1, // already in base, should not duplicate
				"longhorn-system":  1, // already in base, should not duplicate
				"custom-ns":        1, // new namespace
			},
			description: "Spec namespaces that duplicate base namespaces should not create duplicates",
		},
		{
			name:                "settings duplicates should be de-duplicated",
			specExtraNamespaces: []string{}, // empty to use settings
			settingValue:        "ns1,ns1,ns2,ns1,ns3,ns2",
			expectedCount: map[string]int{
				"ns1": 1, // should appear only once despite 3 occurrences
				"ns2": 1, // should appear only once despite 2 occurrences
				"ns3": 1,
			},
			description: "Multiple duplicates in settings should result in single occurrence",
		},
		{
			name:                "settings duplicate of base namespace",
			specExtraNamespaces: []string{}, // empty to use settings
			settingValue:        "harvester-system,custom-ns,harvester-system,local",
			expectedCount: map[string]int{
				"harvester-system": 1, // already in base, should not duplicate
				"local":            1, // already in base, should not duplicate
				"custom-ns":        1, // new namespace
			},
			description: "Settings namespaces that duplicate base namespaces should not create duplicates",
		},
		{
			name:                "empty and whitespace handling",
			specExtraNamespaces: []string{}, // empty to use settings
			settingValue:        "ns1, ,ns2, ns1 ,, ns3",
			expectedCount: map[string]int{
				"ns1": 1, // trimmed and de-duplicated
				"ns2": 1,
				"ns3": 1,
				// empty strings should be filtered out
			},
			description: "Empty strings and whitespace should be handled correctly",
		},
	}

	// Get expected base namespaces for reference
	expectedBaseNamespaces := []string{
		"cattle-dashboards",
		"cattle-fleet-local-system",
		"cattle-fleet-system",
		"cattle-fleet-clusters-system",
		"cattle-monitoring-system",
		"fleet-local",
		"harvester-system",
		"local",
		"longhorn-system",
		"cattle-logging-system",
		"cattle-provisioning-capi-system",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the test setting value
			_ = settings.SupportBundleNamespaces.Set(tt.settingValue)

			// Create test SupportBundle
			sb := &harvesterv1.SupportBundle{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sb",
					Namespace: "test-namespace",
				},
				Spec: harvesterv1.SupportBundleSpec{
					ExtraCollectionNamespaces: tt.specExtraNamespaces,
					Description:               "test support bundle",
				},
			}

			// Execute
			result := manager.getCollectNamespaces(sb)
			resultNamespaces := strings.Split(result, ",")

			// Count occurrences of each namespace
			actualCount := make(map[string]int)
			for _, ns := range resultNamespaces {
				actualCount[ns]++
			}

			// Verify base namespaces appear exactly once
			for _, baseNS := range expectedBaseNamespaces {
				assert.Equal(t, 1, actualCount[baseNS], "Base namespace %s should appear exactly once", baseNS)
			}

			// Verify expected namespaces have correct counts
			for expectedNS, expectedCountVal := range tt.expectedCount {
				assert.Equal(t, expectedCountVal, actualCount[expectedNS],
					"Namespace %s should appear %d time(s), but appeared %d time(s): %s",
					expectedNS, expectedCountVal, actualCount[expectedNS], tt.description)
			}

			// Verify no namespace appears more than once
			for ns, count := range actualCount {
				assert.LessOrEqual(t, count, 1, "Namespace %s appears %d times, should not appear more than once", ns, count)
			}

			// Verify no empty strings (unless explicitly expected)
			for _, ns := range resultNamespaces {
				if _, expected := tt.expectedCount[""]; !expected {
					assert.NotEmpty(t, ns, "Result should not contain empty strings unless explicitly expected")
				}
			}
		})
	}
}
