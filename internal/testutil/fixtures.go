package testutil

import (
	"os"
	"testing"
)

// SkipIfMissing skips the test if the given file path does not exist.
func SkipIfMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("fixture not found: %s", path)
	}
}
