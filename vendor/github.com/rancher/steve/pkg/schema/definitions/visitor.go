package definitions

import (
	"k8s.io/kube-openapi/pkg/util/proto"
)

// schemaFieldVisitor implements proto.SchemaVisitor and turns a given schema into a definitionField.
type schemaFieldVisitor struct {
	field       definitionField
	definitions map[string]definition
}

// VisitArray turns an array into a definitionField (stored on the receiver). For arrays of complex types, will also
// visit the subtype.
func (s *schemaFieldVisitor) VisitArray(array *proto.Array) {
	field := definitionField{
		Description: array.GetDescription(),
	}
	// this currently is not recursive and provides little information for nested types- while this isn't optimal,
	// it was kept this way to provide backwards compat with previous endpoints.
	array.SubType.Accept(s)
	subField := s.field
	field.Type = "array"
	field.SubType = subField.Type
	s.field = field
}

// VisitMap turns a map into a definitionField (stored on the receiver). For maps of complex types, will also visit the
// subtype.
func (s *schemaFieldVisitor) VisitMap(protoMap *proto.Map) {
	field := definitionField{
		Description: protoMap.GetDescription(),
	}
	// this currently is not recursive and provides little information for nested types- while this isn't optimal,
	// it was kept this way to provide backwards compat with previous endpoints.
	protoMap.SubType.Accept(s)
	subField := s.field
	field.Type = "map"
	field.SubType = subField.Type
	s.field = field
}

// VisitPrimitive turns a primitive into a definitionField (stored on the receiver).
func (s *schemaFieldVisitor) VisitPrimitive(primitive *proto.Primitive) {
	field := definitionField{
		Description: primitive.GetDescription(),
	}
	field.Type = getPrimitiveType(primitive.Type)
	s.field = field
}

// VisitKind turns a kind into a definitionField and a definition. Both are stored on the receiver.
func (s *schemaFieldVisitor) VisitKind(kind *proto.Kind) {
	path := kind.Path.String()
	field := definitionField{
		Description: kind.GetDescription(),
		Type:        path,
	}
	if _, ok := s.definitions[path]; ok {
		// if we have already seen this kind, we don't want to re-evaluate the definition. Some kinds can be
		// recursive through use of references, so this circuit-break is necessary to avoid infinite loops
		s.field = field
		return
	}
	schemaDefinition := definition{
		ResourceFields: map[string]definitionField{},
		Type:           path,
		Description:    kind.GetDescription(),
	}
	// this definition may refer to itself, so we mark this as seen to not infinitely recurse
	s.definitions[path] = definition{}
	for fieldName, schemaField := range kind.Fields {
		schemaField.Accept(s)
		schemaDefinition.ResourceFields[fieldName] = s.field
	}
	for _, field := range kind.RequiredFields {
		current, ok := schemaDefinition.ResourceFields[field]
		if !ok {
			// this does silently ignore inconsistent kinds that list
			continue
		}
		current.Required = true
		schemaDefinition.ResourceFields[field] = current
	}
	s.definitions[path] = schemaDefinition
	// the visitor may have set the field multiple times while evaluating kind fields, so we only set the final
	// kind-based field at the end
	s.field = field
}

// VisitReference turns a reference into a definitionField. Will also visit the referred type.
func (s *schemaFieldVisitor) VisitReference(ref proto.Reference) {
	sub := ref.SubSchema()
	if sub == nil {
		// if we don't have a sub-schema defined, we can't extract much meaningful information
		field := definitionField{
			Description: ref.GetDescription(),
			Type:        ref.Reference(),
		}
		s.field = field
		return
	}
	sub.Accept(s)
	field := s.field
	field.Description = ref.GetDescription()
	s.field = field
}

// VisitArbitrary turns an abitrary (item with no type) into a definitionField (stored on the receiver).
func (s *schemaFieldVisitor) VisitArbitrary(arb *proto.Arbitrary) {
	// In certain cases k8s seems to not provide a type for certain fields. We assume for the
	// purposes of this visitor that all of these have a type of string.
	s.field = definitionField{
		Description: arb.GetDescription(),
		Type:        "string",
	}
}
