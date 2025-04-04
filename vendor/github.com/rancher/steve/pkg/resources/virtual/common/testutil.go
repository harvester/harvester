package common

import (
	"github.com/rancher/steve/pkg/summarycache"
	"github.com/rancher/wrangler/v3/pkg/summary"
	"k8s.io/apimachinery/pkg/runtime"
)

type FakeSummaryCache struct {
	SummarizedObject *summary.SummarizedObject
	Relationships    []summarycache.Relationship
}

func (f *FakeSummaryCache) SummaryAndRelationship(runtime.Object) (*summary.SummarizedObject, []summarycache.Relationship) {
	return f.SummarizedObject, f.Relationships
}
