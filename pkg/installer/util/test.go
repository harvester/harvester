package util

import (
	"os"
	"path"
	"testing"
)

// LoadFixture loads a testing fixture from testdata dir
func LoadFixture(t *testing.T, name string) []byte {
	path := path.Join("testdata", name)
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		t.Fatalf("Fail to load fixture %q", path)
	}
	return data
}
