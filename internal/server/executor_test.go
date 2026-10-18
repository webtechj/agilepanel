package server

import (
	"errors"
	"strings"
	"testing"
)

func TestDiagnoseCommandFailure(t *testing.T) {
	tests := []struct {
		name          string
		cmd           string
		args          []string
		output        string
		expectedCause string
	}{
	{
		name:          "PHP Memory Limit",
		cmd:           "wp",
		args:          []string{"core", "update"},
		output:        "Fatal error: Allowed memory size of 134217728 bytes exhausted",
		expectedCause: "PHP runtime exhausted allocated memory_limit",
	},
	{
		name:          "Database Error",
		cmd:           "wp",
		args:          []string{"post", "list"},
		output:        "Error establishing a database connection",
		expectedCause: "Database authentication failure or MariaDB is stopped",
	},
	{
		name:          "Too Many Connections",
		cmd:           "mysql",
		args:          []string{"-u", "root"},
		output:        "ERROR 1040 (08004): Too many connections",
		expectedCause: "MariaDB max_connections limit exhausted by concurrent workers",
	},
	{
		name:          "Filesystem Immutable Lock",
		cmd:           "wp",
		args:          []string{"plugin", "install", "woocommerce"},
		output:        "touch: cannot touch 'index.php': Operation not permitted",
		expectedCause: "Filesystem is protected by Linux immutable attribute (chattr +i)",
	},
	{
		name:          "Disk Full",
		cmd:           "unzip",
		args:          []string{"archive.zip"},
		output:        "write error: No space left on device",
		expectedCause: "Server storage volume is 100% full",
	},
	{
		name:          "Table Corrupted",
		cmd:           "wp",
		args:          []string{"db", "check"},
		output:        "wp_options is marked as crashed and should be repaired",
		expectedCause: "Corrupted MariaDB table index detected",
	},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := &ExecutionResult{
				Combined: tc.output,
				ExitCode: 1,
			}
			err := DiagnoseCommandFailure(tc.cmd, tc.args, res, errors.New("exit status 1"))
			if err == nil {
				t.Fatalf("expected diagnostic error, got nil")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.expectedCause) {
				t.Errorf("expected cause %q in error string, got:\n%s", tc.expectedCause, errStr)
			}
		})
	}
}

func TestSmartExecuteMock(t *testing.T) {
	opts := CommandOptions{
		Timeout:       5,
		RunAsUser:     "wp_test",
		AllowAutoHeal: true,
	}
	res, err := SmartExecute("echo", []string{"hello"}, opts)
	if err != nil {
		t.Fatalf("expected successful mock execution, got err: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
}
