package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"golang.org/x/term"
)

func promptString(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	fd := int(os.Stdin.Fd())
	bytePassword, err := term.ReadPassword(fd)
	if err != nil {
		return "", err
	}
	fmt.Println() // print newline since ReadPassword doesn't echo it
	return strings.TrimSpace(string(bytePassword)), nil
}

func getDomainArg(args []string) (string, error) {
	if len(args) >= 1 {
		return args[0], nil
	}
	domain, err := promptString("Enter domain name: ")
	if err != nil {
		return "", err
	}
	if domain == "" {
		return "", fmt.Errorf("domain name cannot be empty")
	}
	return domain, nil
}

func getServiceArg(args []string) (string, error) {
	if len(args) >= 1 {
		return args[0], nil
	}
	svc, err := promptString("Enter service name (caddy, mariadb, redis, php-fpm, or all) [all]: ")
	if err != nil {
		return "", err
	}
	if svc == "" {
		return "all", nil
	}
	return svc, nil
}
