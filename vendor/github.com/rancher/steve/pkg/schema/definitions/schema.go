package definitions

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/debounce"
	apiextcontrollerv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apiextensions.k8s.io/v1"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apiregistration.k8s.io/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/discovery"
)

const (
	handlerKey     = "schema-definitions"
	delayEnvVar    = "CATTLE_CRD_REFRESH_DELAY_SECONDS"
	defaultDelay   = 2
	delayUnit      = time.Second
	refreshEnvVar  = "CATTLE_BACKGROUND_REFRESH_MINUTES"
	defaultRefresh = 10
	refreshUnit    = time.Minute
)

type schemaDefinition struct {
	DefinitionType string                `json:"definitionType"`
	Definitions    map[string]definition `json:"definitions"`
}

type definition struct {
	ResourceFields map[string]definitionField `json:"resourceFields"`
	Type           string                     `json:"type"`
	Description    string                     `json:"description"`
}

type definitionField struct {
	Type        string `json:"type"`
	SubType     string `json:"subtype,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Merge merges the provided schema into s. All conflicting values (i.e. that are in both schema and s)
// are replaced with the values from s.
func (s *schemaDefinition) Merge(schema schemaDefinition) error {
	if s.DefinitionType != schema.DefinitionType {
		return fmt.Errorf("invalid definition type: %s != %s", s.DefinitionType, schema.DefinitionType)
	}

	for key, value := range schema.Definitions {
		mergedDef := s.Definitions[key]
		if mergedDef.ResourceFields == nil {
			mergedDef.ResourceFields = make(map[string]definitionField)
		}

		mergedDef.Type = value.Type
		mergedDef.Description = value.Description
		for fieldKey, fieldValue := range value.ResourceFields {
			mergedDef.ResourceFields[fieldKey] = fieldValue
		}
		s.Definitions[key] = mergedDef
	}
	return nil
}

// Register registers the schemaDefinition schema.
func Register(ctx context.Context,
	baseSchema *types.APISchemas,
	client discovery.DiscoveryInterface,
	crd apiextcontrollerv1.CustomResourceDefinitionController,
	apiService v1.APIServiceController) {
	handler := NewSchemaDefinitionHandler(baseSchema, crd.Cache(), client)
	baseSchema.MustAddSchema(types.APISchema{
		Schema: &schemas.Schema{
			ID:              "schemaDefinition",
			PluralName:      "schemaDefinitions",
			ResourceMethods: []string{"GET"},
		},
		ByIDHandler: handler.byIDHandler,
	})

	debounce := debounce.DebounceableRefresher{
		Refreshable: handler,
	}
	crdDebounce := getDurationEnvVarOrDefault(delayEnvVar, defaultDelay, delayUnit)
	refHandler := refreshHandler{
		debounceRef:      &debounce,
		debounceDuration: crdDebounce,
	}
	crd.OnChange(ctx, handlerKey, refHandler.onChangeCRD)
	apiService.OnChange(ctx, handlerKey, refHandler.onChangeAPIService)
	refreshFrequency := getDurationEnvVarOrDefault(refreshEnvVar, defaultRefresh, refreshUnit)
	// there's a delay between when a CRD is created and when it is available in the openapi/v2 endpoint
	// the crd/apiservice controllers use a delay of 2 seconds to account for this, but it's possible that this isn't
	// enough in certain environments, so we also use an infrequent background refresh to eventually correct any misses
	refHandler.startBackgroundRefresh(ctx, refreshFrequency)
}

// getDurationEnvVarOrDefault gets the duration value for a given envVar. If not found, it returns the provided default.
// unit is the unit of time (time.Second/time.Minute/etc.) that the returned duration should be in
func getDurationEnvVarOrDefault(envVar string, defaultVal int, unit time.Duration) time.Duration {
	defaultDuration := time.Duration(defaultVal) * unit
	envValue, ok := os.LookupEnv(envVar)
	if !ok {
		return defaultDuration
	}
	parsed, err := strconv.Atoi(envValue)
	if err != nil {
		logrus.Errorf("Env var %s was specified, but could not be converted to an int, default of %d seconds will be used",
			envVar, int64(defaultDuration.Seconds()))
		return defaultDuration
	}
	return time.Duration(parsed) * unit
}
