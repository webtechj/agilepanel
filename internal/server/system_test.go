package server

import (
	"path/filepath"
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
	// On Windows, LockDirectory and UnlockDirectory execute mock paths cleanly without errors
	mockDir := "/var/www/missbaha.com/htdocs"
	if err := LockDirectory(mockDir); err != nil {
		t.Fatalf("LockDirectory failed: %v", err)
	}

	if err := UnlockDirectory(mockDir); err != nil {
		t.Fatalf("UnlockDirectory failed: %v", err)
	}
}
