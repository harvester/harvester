package image

import (
	"context"
	"sync"
)

var importContexts sync.Map

type ImportContext struct {
	ID     string
	URL    string
	Ctx    context.Context
	Cancel context.CancelFunc
}

func cleanUpImport(imageName, url string) {
	v, ok := importContexts.Load(imageName)
	if ok {
		imp, ok := v.(*ImportContext)
		if ok && imp.URL == url {
			imp.Cancel()
			importContexts.Delete(imageName)
		}
	}
}
