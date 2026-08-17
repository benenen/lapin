package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBoundedPDFOutputDiscardsExcessWithoutBlockingProcess(t *testing.T) {
	output := &boundedPDFOutput{}
	input := bytes.Repeat([]byte("x"), maxPDFCommandOutput+1024)
	written, err := output.Write(input)
	if err != nil || written != len(input) || len(output.value) != maxPDFCommandOutput {
		t.Fatalf("Write() = (%d, %v), retained = %d", written, err, len(output.value))
	}
}

func TestPDFTemporaryDirectoryLimitCountsSparseFileSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized-output")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxPDFTemporaryBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ensurePDFTemporaryDirectoryWithinLimits(filepath.Dir(path)); err == nil {
		t.Fatal("oversized temporary output was accepted")
	}
}
