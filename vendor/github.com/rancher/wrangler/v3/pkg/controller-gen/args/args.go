package args

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/gengo/v2/types"
)

type CustomArgs struct {
	// Package is the directory path where generated code output will be located
	Package       string
	ImportPackage string
	TypesByGroup  map[schema.GroupVersion][]*types.Name
	Options       Options
	OutputBase    string
	// BoilerplateContent is the actual boilerplate content that has been
	// read. It will be added to every generated files.
	BoilerplateContent []byte
}

type Options struct {
	ImportPackage string
	// OutputPackage is the directory path where generated code output will be located
	OutputPackage string
	Groups        map[string]Group
	// Boilerplate is the filepath to a boilerplate file whose content will
	// be added to every generated files.
	Boilerplate string
}

type Type struct {
	Version string
	Package string
	Name    string
}

type Group struct {
	// Types is a slice of the following types
	// Instance of any struct: used for reflection to describe the type
	// string: a directory that will be listed (non-recursively) for types
	// Type: a description of a type
	Types         []interface{}
	GenerateTypes bool
	// Generate clientsets
	GenerateClients             bool
	OutputControllerPackageName string
	// Generate listers
	GenerateListers bool
	// Generate informers
	GenerateInformers bool
	// Generate openapi
	GenerateOpenAPI bool
	// OpenAPI extra dependencies
	OpenAPIDependencies []string
	// The package name of the API types
	PackageName string
}
