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
