package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kaikozlov/kindle-koplugin/internal/jsonout"
)

// buildHelper builds the kindle-helper binary and returns its path.
// It is cleaned up automatically via t.Cleanup.
func buildHelper(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "kindle-helper")
	pkg := "github.com/kaikozlov/kindle-koplugin/cmd/kindle-helper"
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func TestNoArgsPrintsUsage(t *testing.T) {
	bin := buildHelper(t)
	cmd := exec.Command(bin)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code with no args")
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.ExitCode())
	}
	if len(out) > 0 {
		// Usage goes to stderr, which is in CombinedOutput
		got := string(out)
		if len(got) == 0 {
			t.Fatal("expected usage output on stderr")
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	bin := buildHelper(t)
	cmd := exec.Command(bin, "bogus")
	out, _ := cmd.CombinedOutput()
	got := string(out)
	if len(got) == 0 {
		t.Fatal("expected error output for unknown command")
	}
}

func TestScanFlagParsing(t *testing.T) {
	bin := buildHelper(t)

	tmpDir := t.TempDir()
	cmd := exec.Command(bin, "scan", "--root", tmpDir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan failed: %v\nstderr: %s", err, out)
	}

	var result jsonout.ScanResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput: %s", err, out)
	}

	if result.Version != 1 {
		t.Errorf("expected version 1, got %d", result.Version)
	}
	if result.Root != tmpDir {
		t.Errorf("expected root %q, got %q", tmpDir, result.Root)
	}
	if result.Books == nil {
		t.Error("expected non-nil books slice")
	}
}

func TestScanMissingRoot(t *testing.T) {
	bin := buildHelper(t)
	cmd := exec.Command(bin, "scan")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected error when --root is missing")
	}
}

func TestConvertFlagParsing(t *testing.T) {
	bin := buildHelper(t)

	inputPath := "/nonexistent/file.kfx"
	outputPath := filepath.Join(t.TempDir(), "out.epub")
	cmd := exec.Command(bin, "convert", "--input", inputPath, "--output", outputPath)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}

	var result jsonout.ConvertResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput: %s", err, out)
	}

	if result.Version != 1 {
		t.Errorf("expected version 1, got %d", result.Version)
	}
	if result.OK {
		t.Error("expected ok=false for nonexistent file")
	}
	if result.Code != "error" {
		t.Errorf("expected code 'error', got %q", result.Code)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestConvertMissingInput(t *testing.T) {
	bin := buildHelper(t)
	cmd := exec.Command(bin, "convert", "--output", "/tmp/out.epub")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected error when --input is missing")
	}
}

func TestConvertMissingOutput(t *testing.T) {
	bin := buildHelper(t)
	cmd := exec.Command(bin, "convert", "--input", "/tmp/in.kfx")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected error when --output is missing")
	}
}

func TestScanFindsKFXFile(t *testing.T) {
	bin := buildHelper(t)

	// Create a temp directory with a .kfx file (empty, just for scan detection)
	tmpDir := t.TempDir()
	kfxFile := filepath.Join(tmpDir, "Test Book_ABCDEF1234.kfx")
	if err := os.WriteFile(kfxFile, []byte("not a real kfx"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "scan", "--root", tmpDir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan failed: %v\n%s", err, out)
	}

	var result jsonout.ScanResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput: %s", err, out)
	}

	if len(result.Books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(result.Books))
	}

	book := result.Books[0]
	if book.Format != "kfx" {
		t.Errorf("expected format 'kfx', got %q", book.Format)
	}
	if book.Title != "Test Book" {
		t.Errorf("expected title 'Test Book', got %q", book.Title)
	}
	if book.SourcePath != kfxFile {
		t.Errorf("expected source_path %q, got %q", kfxFile, book.SourcePath)
	}
}

func TestScanSkipsSDRDirs(t *testing.T) {
	bin := buildHelper(t)

	tmpDir := t.TempDir()
	// Create a .sdr directory that should be skipped
	sdrDir := filepath.Join(tmpDir, "book.sdr")
	if err := os.MkdirAll(sdrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Put a .kfx file inside the .sdr directory (should not be found)
	if err := os.WriteFile(filepath.Join(sdrDir, "inner.kfx"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Put a .kfx file at root level (should be found)
	if err := os.WriteFile(filepath.Join(tmpDir, "outer.kfx"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "scan", "--root", tmpDir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	var result jsonout.ScanResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	if len(result.Books) != 1 {
		t.Fatalf("expected 1 book (skipping .sdr), got %d", len(result.Books))
	}
	if result.Books[0].SourcePath != filepath.Join(tmpDir, "outer.kfx") {
		t.Errorf("expected outer.kfx, got %q", result.Books[0].SourcePath)
	}
}
