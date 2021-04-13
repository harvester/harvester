package ref

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/gengo/examples/set-gen/sets"
)

const (
	AnnotationSchemaOwnerKeyName = "harvesterhci.io/owned-by"
)

// AnnotationSchemaReferences represents the reference collection.
type AnnotationSchemaReferences struct {
	sets.String
}

func (s AnnotationSchemaReferences) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.List())
}

func (s *AnnotationSchemaReferences) UnmarshalJSON(bytes []byte) error {
	var arr []string
	if err := json.Unmarshal(bytes, &arr); err != nil {
		return err
	}
	s.String = sets.NewString(arr...)
	return nil
}

func NewAnnotationSchemaOwnerReferences(refs ...string) AnnotationSchemaReferences {
	return AnnotationSchemaReferences{String: sets.NewString(refs...)}
}

// AnnotationSchemaReference represents the owners with same schema ID.
type AnnotationSchemaReference struct {
	// SchemaID represents the ID of steve schema API.
	SchemaID string `json:"schema"`
	// References stores the steve data ID of the owners.
	References AnnotationSchemaReferences `json:"refs,omitempty"`
}

// AnnotationSchemaOwners structures the value recorded in "harvesterhci.io/owned-by" annotation.
type AnnotationSchemaOwners map[string]AnnotationSchemaReference

func (o AnnotationSchemaOwners) String() string {
	var bytes, _ = o.MarshalJSON() // error should never produce
	return string(bytes)
}

func (o AnnotationSchemaOwners) MarshalJSON() ([]byte, error) {
	if o == nil {
		return []byte(`[]`), nil
	}
	var refs = make(sortableSliceOfAnnotationSchemaReference, 0, len(o))
	for _, ref := range o {
		refs = append(refs, ref)
	}
	if len(refs) > 1 {
		sort.Sort(refs)
	}
	return json.Marshal([]AnnotationSchemaReference(refs))
}

func (o *AnnotationSchemaOwners) UnmarshalJSON(bytes []byte) error {
	var refs []AnnotationSchemaReference
	if err := json.Unmarshal(bytes, &refs); err != nil {
		return err
	}

	var owners = make(AnnotationSchemaOwners, len(refs))
	for _, ref := range refs {
		if ref.SchemaID == "" {
			continue
		}
		if _, existed := owners[ref.SchemaID]; !existed {
			// add a new owner
			owners[ref.SchemaID] = ref
			continue
		}
		// add new sortableSliceOfAnnotationSchemaReference
		owners[ref.SchemaID].References.Insert(ref.References.List()...)
	}
	*o = owners
	return nil
}

// List returns the owner's name list by given group kind.
func (o AnnotationSchemaOwners) List(ownerGK schema.GroupKind) []string {
	var schemaID = GroupKindToSchemaID(ownerGK)
	var schemaRef, existed = o[schemaID]
	if !existed || schemaRef.SchemaID != schemaID {
		return []string{}
	}
	return schemaRef.References.UnsortedList()
}

// Has checks if the given owner is owned.
func (o AnnotationSchemaOwners) Has(ownerGK schema.GroupKind, owner metav1.Object) bool {
	var schemaID = GroupKindToSchemaID(ownerGK)
	var ownerRef = Construct(owner.GetNamespace(), owner.GetName())

	var schemaRef, existed = o[schemaID]
	if !existed {
		return false
	}
	return schemaRef.SchemaID == schemaID && schemaRef.References.Has(ownerRef)
}

// Add adds the given owner as an annotation schema owner,
// returns false to indicate that the ownerRef was a reference.
func (o AnnotationSchemaOwners) Add(ownerGK schema.GroupKind, owner metav1.Object) bool {
	if o.Has(ownerGK, owner) {
		return false
	}

	var schemaID = GroupKindToSchemaID(ownerGK)
	var ownerRef = Construct(owner.GetNamespace(), owner.GetName())
	var schemaRef, existed = o[schemaID]
	if !existed {
		schemaRef = AnnotationSchemaReference{SchemaID: schemaID, References: NewAnnotationSchemaOwnerReferences()}
	}
	schemaRef.References.Insert(ownerRef)
	o[schemaID] = schemaRef
	return true
}

// Remove remove the given owner from the annotation schema owners,
// returns false to indicate that the ownerRef isn't a reference.
func (o AnnotationSchemaOwners) Remove(ownerGK schema.GroupKind, owner metav1.Object) bool {
	if !o.Has(ownerGK, owner) {
		return false
	}

	var schemaID = GroupKindToSchemaID(ownerGK)
	var ownerRef = Construct(owner.GetNamespace(), owner.GetName())
	var schemaRef = o[schemaID]
	if schemaRef.References.Delete(ownerRef).Len() == 0 {
		delete(o, schemaID)
	}
	return true
}

// Bind bind the schema owners to given object's annotation.
func (o AnnotationSchemaOwners) Bind(obj metav1.Object) error {
	var annotations = obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	if len(o) == 0 {
		delete(annotations, AnnotationSchemaOwnerKeyName)
	} else {
		var ownersBytes, err = json.Marshal(o)
		if err != nil {
			return fmt.Errorf("failed to marshal annotation schema owners: %w", err)
		}
		annotations[AnnotationSchemaOwnerKeyName] = string(ownersBytes)
	}

	if len(annotations) == 0 {
		obj.SetAnnotations(nil)
	} else {
		obj.SetAnnotations(annotations)
	}
	return nil
}

type sortableSliceOfAnnotationSchemaReference []AnnotationSchemaReference

func (s sortableSliceOfAnnotationSchemaReference) Len() int {
	return len(s)
}

func (s sortableSliceOfAnnotationSchemaReference) Less(i, j int) bool {
	return s[i].SchemaID < s[j].SchemaID
}

func (s sortableSliceOfAnnotationSchemaReference) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// GroupKindToSchemaID translate the GroupKind to steve schema ID.
func GroupKindToSchemaID(kind schema.GroupKind) string {
	return strings.ToLower(fmt.Sprintf("%s.%s", kind.Group, kind.Kind))
}

// GetSchemaOwnersFromAnnotation gets the annotation schema owners from resource.
func GetSchemaOwnersFromAnnotation(obj metav1.Object) (AnnotationSchemaOwners, error) {
	var annotations = obj.GetAnnotations()
	var ownedByAnnotation, ok = annotations[AnnotationSchemaOwnerKeyName]
	if !ok {
		return AnnotationSchemaOwners{}, nil
	}

	var owner AnnotationSchemaOwners
	if err := json.Unmarshal([]byte(ownedByAnnotation), &owner); err != nil {
		return owner, fmt.Errorf("failed to unmarshal annotation schema owners: %w", err)
	}
	return owner, nil
}
