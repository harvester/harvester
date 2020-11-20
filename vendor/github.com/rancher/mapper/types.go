package mapper

type Schema struct {
	ID             string            `json:"id,omitempty"`
	CodeName       string            `json:"-"`
	CodeNamePlural string            `json:"-"`
	PkgName        string            `json:"-"`
	Type           string            `json:"type,omitempty"`
	Links          map[string]string `json:"links"`
	PluralName     string            `json:"pluralName,omitempty"`
	ResourceFields map[string]Field  `json:"resourceFields"`
	NonNamespaced  bool              `json:"-"`
	Object         bool              `json:"-"`

	InternalSchema *Schema `json:"-"`
	Mapper         Mapper  `json:"-"`
}

type Field struct {
	Type         string      `json:"type,omitempty"`
	Default      interface{} `json:"default,omitempty"`
	Nullable     bool        `json:"nullable,omitempty"`
	Create       bool        `json:"create"`
	WriteOnly    bool        `json:"writeOnly,omitempty"`
	Required     bool        `json:"required,omitempty"`
	Update       bool        `json:"update"`
	MinLength    *int64      `json:"minLength,omitempty"`
	MaxLength    *int64      `json:"maxLength,omitempty"`
	Min          *int64      `json:"min,omitempty"`
	Max          *int64      `json:"max,omitempty"`
	Options      []string    `json:"options,omitempty"`
	ValidChars   string      `json:"validChars,omitempty"`
	InvalidChars string      `json:"invalidChars,omitempty"`
	Description  string      `json:"description,omitempty"`
	CodeName     string      `json:"-"`
	DynamicField bool        `json:"dynamicField,omitempty"`
}
