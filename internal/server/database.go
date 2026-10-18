package server

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var sqlIdentifierRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)

// ValidateSQLIdentifier checks that an identifier consists strictly of alphanumeric and underscore characters.
func ValidateSQLIdentifier(ident string) error {
	if !sqlIdentifierRegex.MatchString(ident) {
		return fmt.Errorf("invalid SQL identifier %q: must be 1-64 alphanumeric or underscore characters", ident)
	}
	return nil
}

// EscapeSQLString escapes single quotes and backslashes for safe embedding in SQL statements.
func EscapeSQLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// GenerateSecurePassword generates a random 24-character alphanumeric password.
func GenerateSecurePassword() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 24)
	for i := 0; i < 24; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

// GenerateSecurePrefix generates a random lowercase alphanumeric prefix of the specified length.
// The first character is guaranteed to be a letter (required for valid SQL identifiers).
func GenerateSecurePrefix(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("length must be greater than 0")
	}
	const firstCharset = "abcdefghijklmnopqrstuvwxyz"
	const generalCharset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)

	// First character must be a letter
	num, err := rand.Int(rand.Reader, big.NewInt(int64(len(firstCharset))))
	if err != nil {
		return "", err
	}
	result[0] = firstCharset[num.Int64()]

	// Remaining characters
	for i := 1; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(generalCharset))))
		if err != nil {
			return "", err
		}
		result[i] = generalCharset[num.Int64()]
	}
	return string(result), nil
}

// CreateDatabase provisions a MariaDB database and user with appropriate privileges.
func CreateDatabase(dbName string, dbUser string, dbPassword string) error {
	if err := ValidateSQLIdentifier(dbName); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	if err := ValidateSQLIdentifier(dbUser); err != nil {
		return fmt.Errorf("invalid database user: %w", err)
	}

	escapedPassword := EscapeSQLString(dbPassword)

	sqlQuery := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s`; "+
			"CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'; "+
			"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'; "+
			"FLUSH PRIVILEGES;",
		dbName, dbUser, escapedPassword, dbName, dbUser,
	)

	if runtime.GOOS != "linux" {
		fmt.Printf("DB (Mock): Execute SQL via mysql -u root:\n%s\n", sqlQuery)
		return nil
	}

	opts := CommandOptions{
		Timeout:       30 * time.Second,
		AllowAutoHeal: true,
		RetryCount:    1,
	}
	_, err := SmartExecute("mysql", []string{"-u", "root", "-e", sqlQuery}, opts)
	if err != nil {
		return fmt.Errorf("failed to create database and user: %w", err)
	}

	fmt.Printf("DB: Created database %s and user %s successfully.\n", dbName, dbUser)
	return nil
}

// DeleteDatabase removes the database and user.
func DeleteDatabase(dbName string, dbUser string) error {
	if err := ValidateSQLIdentifier(dbName); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	if err := ValidateSQLIdentifier(dbUser); err != nil {
		return fmt.Errorf("invalid database user: %w", err)
	}

	sqlQuery := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`; "+
			"DROP USER IF EXISTS '%s'@'localhost';",
		dbName, dbUser,
	)

	if runtime.GOOS != "linux" {
		fmt.Printf("DB (Mock): Execute SQL via mysql -u root:\n%s\n", sqlQuery)
		return nil
	}

	opts := CommandOptions{
		Timeout:       30 * time.Second,
		AllowAutoHeal: true,
		RetryCount:    1,
	}
	_, err := SmartExecute("mysql", []string{"-u", "root", "-e", sqlQuery}, opts)
	if err != nil {
		return fmt.Errorf("failed to delete database and user: %w", err)
	}

	fmt.Printf("DB: Dropped database %s and user %s successfully.\n", dbName, dbUser)
	return nil
}
