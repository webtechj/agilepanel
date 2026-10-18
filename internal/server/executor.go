package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ExecutionResult captures complete output and diagnostic metadata from a command run.
type ExecutionResult struct {
	Stdout      string
	Stderr      string
	Combined    string
	ExitCode    int
	Duration    time.Duration
	AutoHealed  bool
	HealActions []string
}

// CommandOptions configures behavior for SmartExecute.
type CommandOptions struct {
	Timeout       time.Duration
	RunAsUser     string
	HomeDir       string
	SitePublicDir string
	AllowAutoHeal bool
	RetryCount    int
	Stdin         io.Reader
	Env           []string
}

// SmartExecute runs system commands with unified stdout/stderr interception,
// root cause error diagnosis, and automated non-destructive self-healing.
func SmartExecute(cmdName string, args []string, opts CommandOptions) (*ExecutionResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.RetryCount <= 0 {
		opts.RetryCount = 1
	}

	// Mock behavior for non-Linux OS (Windows dev / CI) unless forced via test env
	if runtime.GOOS != "linux" && os.Getenv("AGILEPANEL_TEST_REAL_EXEC") != "true" {
		fullArgs := append([]string{cmdName}, args...)
		if opts.RunAsUser != "" {
			fmt.Printf("OS (Mock SmartExec as %s): %v\n", opts.RunAsUser, fullArgs)
		} else {
			fmt.Printf("OS (Mock SmartExec): %v\n", fullArgs)
		}
		return &ExecutionResult{
			Stdout:   "",
			Stderr:   "",
			Combined: "",
			ExitCode: 0,
			Duration: 1 * time.Millisecond,
		}, nil
	}

	var healActions []string
	autoHealed := false

	maxAttempts := opts.RetryCount + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)

		var targetBinary string
		var targetArgs []string

		if opts.RunAsUser != "" {
			targetBinary = "sudo"
			if opts.HomeDir != "" {
				targetArgs = append([]string{"-u", opts.RunAsUser, "env", "HOME=" + opts.HomeDir, cmdName}, args...)
			} else {
				targetArgs = append([]string{"-u", opts.RunAsUser, cmdName}, args...)
			}
		} else {
			targetBinary = cmdName
			targetArgs = args
		}

		cmd := exec.CommandContext(ctx, targetBinary, targetArgs...)
		if opts.Stdin != nil {
			cmd.Stdin = opts.Stdin
		}
		if len(opts.Env) > 0 {
			cmd.Env = append(os.Environ(), opts.Env...)
		}

		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		start := time.Now()
		err := cmd.Run()
		duration := time.Since(start)
		cancel()

		result := &ExecutionResult{
			Stdout:      strings.TrimSpace(stdoutBuf.String()),
			Stderr:      strings.TrimSpace(stderrBuf.String()),
			Combined:    strings.TrimSpace(stdoutBuf.String() + "\n" + stderrBuf.String()),
			Duration:    duration,
			AutoHealed:  autoHealed,
			HealActions: healActions,
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}

		// Success case
		if err == nil {
			return result, nil
		}

		// Attempt intelligent self-healing if enabled and attempts remain
		if opts.AllowAutoHeal && attempt < maxAttempts {
			healed, action := AttemptAutoHealing(result, opts)
			if healed {
				autoHealed = true
				healActions = append(healActions, action)
				time.Sleep(300 * time.Millisecond)
				continue // Retry original command after healing
			}
		}

		// If no healing was possible or retry still failed, return formatted diagnosis
		return result, DiagnoseCommandFailure(cmdName, args, result, err)
	}

	return nil, fmt.Errorf("command execution exhausted all retry attempts")
}

// AttemptAutoHealing evaluates error signatures and triggers automated remedies.
func AttemptAutoHealing(res *ExecutionResult, opts CommandOptions) (bool, string) {
	combined := res.Combined

	// 1. Filesystem immutable lock (chattr +i) preventing file operations
	if (strings.Contains(combined, "Operation not permitted") ||
		strings.Contains(combined, "Permission denied") ||
		strings.Contains(combined, "Could not create directory") ||
		strings.Contains(combined, "Cannot write to")) && opts.SitePublicDir != "" {
		parentDir := GetSiteRootDir(opts.SitePublicDir)
		if err := UnlockDirectory(parentDir); err == nil {
			return true, fmt.Sprintf("Temporarily unlocked immutable attributes on %s", parentDir)
		}
	}

	// 2. Stale WP-CLI core updater lock transient
	if strings.Contains(combined, "Another update is currently in progress") && opts.SitePublicDir != "" {
		delCmd := exec.Command("sudo", "-u", opts.RunAsUser, "wp", "transient", "delete", "core_updater.lock", "--path="+opts.SitePublicDir)
		_ = delCmd.Run()
		return true, "Purged stale WordPress core_updater.lock transient"
	}

	// 3. Database connection failure due to inactive/crashed MariaDB daemon
	if strings.Contains(combined, "Can't connect to local MySQL server through socket") ||
		strings.Contains(combined, "Connection refused") && strings.Contains(combined, "3306") {
		restartCmd := exec.Command("systemctl", "restart", "mariadb")
		if err := restartCmd.Run(); err != nil {
			restartFallback := exec.Command("systemctl", "restart", "mysql")
			_ = restartFallback.Run()
		}
		time.Sleep(1 * time.Second)
		return true, "Detected inactive MariaDB daemon and initiated service restart"
	}

	// 4. Stale package manager lock file (/var/lib/dpkg/lock-frontend)
	if strings.Contains(combined, "Could not get lock /var/lib/dpkg/lock") {
		time.Sleep(3 * time.Second)
		return true, "Waited for background apt/dpkg lock release"
	}

	// 5. Caddy reload failure due to inactive process
	if strings.Contains(combined, "caddy reload") && strings.Contains(combined, "connection refused") {
		_ = exec.Command("systemctl", "restart", "caddy").Run()
		time.Sleep(500 * time.Millisecond)
		return true, "Restarted Caddy service after reload endpoint refusal"
	}

	return false, ""
}

// DiagnoseCommandFailure parses command output to provide human-readable root-cause and remediation.
func DiagnoseCommandFailure(cmd string, args []string, res *ExecutionResult, originalErr error) error {
	combined := res.Combined
	var cause, remedy string

	switch {
	case strings.Contains(combined, "Allowed memory size of"):
		cause = "PHP runtime exhausted allocated memory_limit."
		remedy = "Run 'ap site update-pool <domain> --memory=256M' (or 512M) to expand memory limits."

	case strings.Contains(combined, "Error establishing a database connection"):
		cause = "Database authentication failure or MariaDB is stopped."
		remedy = "Check database credentials in wp-config.php and verify 'systemctl status mariadb'."

	case strings.Contains(combined, "Too many connections") || strings.Contains(combined, "Error 1040"):
		cause = "MariaDB max_connections limit exhausted by concurrent workers."
		remedy = "Run 'ap server tune' to recalculate optimal database connection allocations."

	case strings.Contains(combined, "Operation not permitted") || strings.Contains(combined, "chattr"):
		cause = "Filesystem is protected by Linux immutable attribute (chattr +i) or permission restrictions."
		remedy = "Run 'ap site unlock <domain>' before performing maintenance or file edits."

	case strings.Contains(combined, "Access denied for user"):
		cause = "Database user credentials mismatch or insufficient SQL privileges."
		remedy = "Verify database user password and grant privileges via MariaDB."

	case strings.Contains(combined, "No space left on device"):
		cause = "Server storage volume is 100% full."
		remedy = "Run 'ap server clean' to purge journal logs, old backups, and package caches."

	case strings.Contains(strings.ToLower(combined), "marked as crashed"):
		cause = "Corrupted MariaDB table index detected."
		remedy = "Run 'ap site optimize <domain>' to defragment and repair database tables."

	case strings.Contains(combined, "cURL error 28") || strings.Contains(combined, "Could not resolve host"):
		cause = "Outbound network connectivity or DNS resolution failure."
		remedy = "Check server /etc/resolv.conf and verify outbound internet connectivity."

	default:
		cause = originalErr.Error()
		remedy = "Inspect command stdout/stderr output for detailed diagnostic traces."
	}

	displayOut := combined
	if len(displayOut) > 400 {
		displayOut = displayOut[:397] + "..."
	}
	if displayOut == "" {
		displayOut = "<no output produced>"
	}

	return fmt.Errorf("Command Execution Failed: %s %s (Exit Code: %d)\n  ├─ Root Cause : %s\n  ├─ Output     : %s\n  └─ Remediation: %s",
		cmd, strings.Join(args, " "), res.ExitCode, cause, displayOut, remedy)
}

// SmartRunAsUser executes a command on Linux using sudo with full stdout/stderr capture and self-healing.
func SmartRunAsUser(username string, homeDir string, publicDir string, command string, args ...string) (*ExecutionResult, error) {
	opts := CommandOptions{
		RunAsUser:     username,
		HomeDir:       homeDir,
		SitePublicDir: publicDir,
		AllowAutoHeal: true,
		RetryCount:    1,
		Timeout:       120 * time.Second,
	}
	return SmartExecute(command, args, opts)
}
