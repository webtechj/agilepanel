package cmd

import (
	"bytes"
	"testing"
)

func TestSiteUpdateCommandHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"site", "update", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("expected no error from site update --help, got %v", err)
	}

	out := buf.String()
	flags := []string{"--php81", "--php82", "--php83", "--php", "--memory", "--heavy-plugins"}
	for _, f := range flags {
		if !bytes.Contains([]byte(out), []byte(f)) {
			t.Errorf("expected flag %s in site update --help output", f)
		}
	}
}

func TestSiteCreateCommandHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"site", "create", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("expected no error from site create --help, got %v", err)
	}

	out := buf.String()
	flags := []string{"--heavy-plugins", "--memory"}
	for _, f := range flags {
		if !bytes.Contains([]byte(out), []byte(f)) {
			t.Errorf("expected flag %s in site create --help output", f)
		}
	}
}
