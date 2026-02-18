package schematracker

import (
	"errors"
	"slices"

	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/steve/pkg/schema"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
)

type Resetter interface {
	Reset(k8sschema.GroupVersionKind) error
}

type SchemaTracker struct {
	knownSchemas map[k8sschema.GroupVersionKind][]common.ColumnDefinition
	resetter     Resetter
}

func NewSchemaTracker(resetter Resetter) *SchemaTracker {
	return &SchemaTracker{
		knownSchemas: make(map[k8sschema.GroupVersionKind][]common.ColumnDefinition),
		resetter:     resetter,
	}
}

func (s *SchemaTracker) OnSchemas(schemas *schema.Collection) error {
	knownSchemas := make(map[k8sschema.GroupVersionKind][]common.ColumnDefinition)

	needsReset := make(map[k8sschema.GroupVersionKind]struct{})

	deletedSchemas := make(map[k8sschema.GroupVersionKind]struct{})
	for gvk := range s.knownSchemas {
		deletedSchemas[gvk] = struct{}{}
	}

	for _, id := range schemas.IDs() {
		theSchema := schemas.Schema(id)
		if theSchema == nil {
			continue
		}

		gvk := attributes.GVK(theSchema)

		cols := common.GetColumnDefinitions(theSchema)

		knownSchemas[gvk] = cols

		oldCols, exists := s.knownSchemas[gvk]
		if exists {
			if !slices.Equal(cols, oldCols) {
				needsReset[gvk] = struct{}{}
			}
		} else {
			needsReset[gvk] = struct{}{}
		}

		// Schema is still there so it hasn't been deleted
		delete(deletedSchemas, gvk)
	}

	// All deleted schemas must be resetted as well
	for gvk := range deletedSchemas {
		needsReset[gvk] = struct{}{}
	}

	// Reset known schemas
	var retErr error
	for gvk := range needsReset {
		err := s.resetter.Reset(gvk)
		retErr = errors.Join(retErr, err)
	}

	s.knownSchemas = knownSchemas
	return retErr
}
