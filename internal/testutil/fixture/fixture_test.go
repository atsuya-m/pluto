package fixture

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureMatchesTestdata(t *testing.T) {
	for _, f := range Files() {
		embedded, err := Source(f)
		if err != nil {
			t.Fatal(err)
		}
		onDisk, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "proto", f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if !bytes.Equal(embedded, onDisk) {
			t.Errorf("%s differs from testdata/proto; copy testdata/proto into internal/testutil/fixture/proto", f)
		}
	}
}
