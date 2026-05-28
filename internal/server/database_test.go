package server

import (
	"testing"
)

func TestGenerateSecurePassword(t *testing.T) {
	pw1, err := GenerateSecurePassword()
	if err != nil {
		t.Fatalf("failed to generate password: %v", err)
	}

	if len(pw1) != 24 {
		t.Errorf("expected password length 24, got %d", len(pw1))
	}

	pw2, err := GenerateSecurePassword()
	if err != nil {
		t.Fatalf("failed to generate password: %v", err)
	}

	if pw1 == pw2 {
		t.Error("expected randomly generated passwords to be unique, but got identical values")
	}
}

func TestGenerateSecurePrefix(t *testing.T) {
	prefix, err := GenerateSecurePrefix(6)
	if err != nil {
		t.Fatalf("failed to generate prefix: %v", err)
	}

	if len(prefix) != 6 {
		t.Errorf("expected prefix length 6, got %d", len(prefix))
	}

	firstChar := prefix[0]
	if firstChar < 'a' || firstChar > 'z' {
		t.Errorf("expected first character of prefix to be a letter, got '%c'", firstChar)
	}

	prefix2, _ := GenerateSecurePrefix(6)
	if prefix == prefix2 {
		t.Error("expected generated prefixes to be unique, got identical values")
	}
}

func TestCreateAndDeleteDatabase(t *testing.T) {
	err := CreateDatabase("wp_test_db", "wp_test_user", "some_secure_pass")
	if err != nil {
		t.Fatalf("expected no error from CreateDatabase, got %v", err)
	}

	err = DeleteDatabase("wp_test_db", "wp_test_user")
	if err != nil {
		t.Fatalf("expected no error from DeleteDatabase, got %v", err)
	}
}
