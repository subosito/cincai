package testfixture

import (
	"path/filepath"
	"runtime"
)

// ProvidersYAML is the shared catalog fixture for unit tests.
func ProvidersYAML() string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "testdata", "fixtures", "providers.yaml")
}