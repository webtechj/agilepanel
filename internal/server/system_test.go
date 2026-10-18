package server

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetSiteRootDir(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "/var/www/example.com/htdocs", expected: filepath.Clean("/var/www/example.com")},
		{input: "/var/www/example.com/htdocs/public", expected: filepath.Clean("/var/www/example.com")},
		{input: "/var/www/my-site.org", expected: filepath.Clean("/var/www/my-site.org")},
	}

	for _, tt := range tests {
		actual := GetSiteRootDir(tt.input)
		if actual != tt.expected {
			t.Errorf("GetSiteRootDir(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestLockAndUnlockDirectoryMock(t *testing.T) {
	// On Windows/non-Linux, LockDirectory, UnlockDirectory, and IsDirectoryLocked execute mock paths cleanly without errors
	mockDir := "/var/www/missbaha.com/htdocs"

	// Verify idempotency: calling LockDirectory consecutively must succeed
	if err := LockDirectory(mockDir); err != nil {
		t.Fatalf("First LockDirectory failed: %v", err)
	}
	if err := LockDirectory(mockDir); err != nil {
		t.Fatalf("Second (consecutive) LockDirectory failed: %v", err)
	}

	if err := UnlockDirectory(mockDir); err != nil {
		t.Fatalf("UnlockDirectory failed: %v", err)
	}
	if err := UnlockDirectory(mockDir); err != nil {
		t.Fatalf("Second (consecutive) UnlockDirectory failed: %v", err)
	}

	_ = IsDirectoryLocked(mockDir)
}

func TestSafeExtractZip(t *testing.T) {
	tempDir := t.TempDir()
	validZipPath := filepath.Join(tempDir, "valid.zip")
	destDir := filepath.Join(tempDir, "extracted")

	// 1. Test Valid ZIP extraction
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	f, err := w.Create("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("world"))

	subF, err := w.Create("subdir/sub.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = subF.Write([]byte("sub content"))

	_ = w.Close()
	_ = os.WriteFile(validZipPath, buf.Bytes(), 0644)

	if err := SafeExtractZip(validZipPath, destDir); err != nil {
		t.Fatalf("SafeExtractZip failed on valid zip: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
	if err != nil || string(data) != "world" {
		t.Fatalf("expected hello.txt to contain 'world', got %s", string(data))
	}

	// 2. Test Zip Slip malicious archive
	evilBuf := new(bytes.Buffer)
	evilW := zip.NewWriter(evilBuf)
	evilF, err := evilW.Create("../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = evilF.Write([]byte("malicious content"))
	_ = evilW.Close()

	evilZipPath := filepath.Join(tempDir, "evil.zip")
	_ = os.WriteFile(evilZipPath, evilBuf.Bytes(), 0644)

	err = SafeExtractZip(evilZipPath, destDir)
	if err == nil {
		t.Fatal("expected error on Zip Slip attempt, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Fatalf("expected security violation error, got: %v", err)
	}
}
