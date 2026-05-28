package server

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"
	"os/exec"
	"runtime"
)

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
	sqlQuery := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s`; "+
			"CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'; "+
			"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'; "+
			"FLUSH PRIVILEGES;",
		dbName, dbUser, dbPassword, dbName, dbUser,
	)

	if runtime.GOOS != "linux" {
		fmt.Printf("DB (Mock): Execute SQL via mysql -u root:\n%s\n", sqlQuery)
		return nil
	}

	cmd := exec.Command("mysql", "-u", "root", "-e", sqlQuery)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create database and user: %w (stderr: %s)", err, stderr.String())
	}

	fmt.Printf("DB: Created database %s and user %s successfully.\n", dbName, dbUser)
	return nil
}

// DeleteDatabase removes the database and user.
func DeleteDatabase(dbName string, dbUser string) error {
	sqlQuery := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`; "+
			"DROP USER IF EXISTS '%s'@'localhost';",
		dbName, dbUser,
	)

	if runtime.GOOS != "linux" {
		fmt.Printf("DB (Mock): Execute SQL via mysql -u root:\n%s\n", sqlQuery)
		return nil
	}

	cmd := exec.Command("mysql", "-u", "root", "-e", sqlQuery)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete database and user: %w (stderr: %s)", err, stderr.String())
	}

	fmt.Printf("DB: Dropped database %s and user %s successfully.\n", dbName, dbUser)
	return nil
}
